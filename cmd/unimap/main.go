package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/unimap-icp-hunter/project/internal/adapter"
	"github.com/unimap-icp-hunter/project/internal/core/unimap"
	"github.com/unimap-icp-hunter/project/internal/service"
)

var (
	cfgFile string
	logger  *zap.Logger
)

// rootCmd represents the base command
var rootCmd = &cobra.Command{
	Use:   "unimap",
	Short: "UniMap - 多引擎统一查询平台",
	Long: `UniMap 是一个统一网络空间测绘引擎查询的工具。

支持多引擎聚合查询，提供统一的查询语法，
屏蔽各引擎字段与语法差异。`,

	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("UniMap v1.0 - 使用 --help 查看可用命令")
	},
}

// queryCmd 统一查询命令
var queryCmd = &cobra.Command{
	Use:   "query [UQL查询语句]",
	Short: "执行统一查询",
	Long: `执行UniMap统一查询。

示例:
  unimap query 'country="CN" && port="80"'
  unimap query 'title="管理后台" && protocol="https"' --engines fofa,hunter
  unimap query 'port IN ["80", "443"]' --max-pages 2 --page-size 100

UQL语法:
  - 基本比较: field="value"
  - 逻辑与: field1="value1" && field2="value2"
  - 范围查询: field IN ["value1", "value2"]
  - 支持字段: body, title, header, port, protocol, ip, country, region, city, asn, org, isp, domain, status_code`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		uql := args[0]
		engines, _ := cmd.Flags().GetStringSlice("engines")
		pageSize, _ := cmd.Flags().GetInt("page-size")
		maxPages, _ := cmd.Flags().GetInt("max-pages")
		output, _ := cmd.Flags().GetString("output")

		// 初始化
		initLogger()
		defer logger.Sync()

		// 创建引擎适配器
		orchestrator := adapter.NewEngineOrchestrator()

		// 注册FOFA适配器
		fofaKey := viper.GetString("unimap.engines.fofa.api_key")
		fofaEmail := viper.GetString("unimap.engines.fofa.email")
		if fofaKey != "" && fofaEmail != "" {
			fofaAdapter := adapter.NewFofaAdapter(
				viper.GetString("unimap.engines.fofa.base_url"),
				fofaKey,
				fofaEmail,
				viper.GetInt("unimap.engines.fofa.qps"),
				viper.GetDuration("unimap.engines.fofa.timeout"),
			)
			orchestrator.RegisterAdapter(fofaAdapter)
		}

		// 注册Hunter适配器
		hunterKey := viper.GetString("unimap.engines.hunter.api_key")
		if hunterKey != "" {
			hunterAdapter := adapter.NewHunterAdapter(
				viper.GetString("unimap.engines.hunter.base_url"),
				hunterKey,
				viper.GetInt("unimap.engines.hunter.qps"),
				viper.GetDuration("unimap.engines.hunter.timeout"),
			)
			orchestrator.RegisterAdapter(hunterAdapter)
		}

		// 注册ZoomEye适配器
		zoomeyeKey := viper.GetString("unimap.engines.zoomeye.api_key")
		if zoomeyeKey != "" {
			zoomeyeAdapter := adapter.NewZoomEyeAdapter(
				viper.GetString("unimap.engines.zoomeye.base_url"),
				zoomeyeKey,
				viper.GetInt("unimap.engines.zoomeye.qps"),
				viper.GetDuration("unimap.engines.zoomeye.timeout"),
			)
			orchestrator.RegisterAdapter(zoomeyeAdapter)
		}

		// 注册Quake适配器
		quakeKey := viper.GetString("unimap.engines.quake.api_key")
		if quakeKey != "" {
			quakeAdapter := adapter.NewQuakeAdapter(
				viper.GetString("unimap.engines.quake.base_url"),
				quakeKey,
				viper.GetInt("unimap.engines.quake.qps"),
				viper.GetDuration("unimap.engines.quake.timeout"),
			)
			orchestrator.RegisterAdapter(quakeAdapter)
		}

		// 如果没有指定引擎，使用所有已注册的
		if len(engines) == 0 {
			engines = orchestrator.ListAdapters()
		}

		if len(engines) == 0 {
			fmt.Println("错误: 未配置任何引擎或未指定引擎")
			os.Exit(1)
		}

		// 执行查询
		fmt.Printf("正在使用引擎 %v 执行查询...\n", engines)
		fmt.Printf("UQL: %s\n\n", uql)

		// 创建服务
		svc := service.NewUniMapService(orchestrator, logger)

		// 执行统一查询
		assets, err := svc.QueryUnified(uql, engines, pageSize, maxPages)
		if err != nil {
			fmt.Printf("查询失败: %v\n", err)
			os.Exit(1)
		}

		// 输出结果
		if output == "json" {
			// JSON输出
			svc.PrintAssetsJSON(assets)
		} else {
			// 表格输出
			svc.PrintAssetsTable(assets)
		}

		fmt.Printf("\n共找到 %d 条结果\n", len(assets))
	},
}

// validateCmd 验证UQL语法
var validateCmd = &cobra.Command{
	Use:   "validate [UQL查询语句]",
	Short: "验证UQL语法",
	Long:  `验证UQL查询语法是否正确，不执行实际查询。`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		uql := args[0]
		parser := unimap.NewUQLParser()

		if err := parser.Validate(uql); err != nil {
			fmt.Printf("❌ 语法错误: %v\n", err)
			os.Exit(1)
		}

		ast, err := parser.Parse(uql)
		if err != nil {
			fmt.Printf("❌ 解析失败: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("✅ UQL语法验证通过")
		fmt.Printf("简化后: %s\n", parser.Simplify(uql))

		// 显示提取的条件
		conditions := parser.ExtractConditions(ast)
		if len(conditions) > 0 {
			fmt.Println("\n提取的条件:")
			for field, cond := range conditions {
				fmt.Printf("  %s: %v\n", field, cond)
			}
		}
	},
}

// listEnginesCmd 列出可用引擎
var listEnginesCmd = &cobra.Command{
	Use:   "list-engines",
	Short: "列出可用的测绘引擎",
	Long:  `显示所有已配置的引擎及其状态。`,
	Run: func(cmd *cobra.Command, args []string) {
		initLogger()
		defer logger.Sync()

		fmt.Println("可用的测绘引擎:")
		fmt.Println("================================")

		engines := []struct {
			name    string
			enabled bool
			key     string
		}{
			{"FOFA", viper.GetBool("unimap.engines.fofa.enabled"), viper.GetString("unimap.engines.fofa.api_key")},
			{"Hunter", viper.GetBool("unimap.engines.hunter.enabled"), viper.GetString("unimap.engines.hunter.api_key")},
			{"ZoomEye", viper.GetBool("unimap.engines.zoomeye.enabled"), viper.GetString("unimap.engines.zoomeye.api_key")},
			{"Quake", viper.GetBool("unimap.engines.quake.enabled"), viper.GetString("unimap.engines.quake.api_key")},
		}

		for _, e := range engines {
			status := "❌ 未配置"
			if e.enabled {
				if e.key != "" {
					status = "✅ 已配置"
				} else {
					status = "⚠️  已启用但未配置API密钥"
				}
			}
			fmt.Printf("%-10s %s\n", e.name, status)
		}
	},
}

func init() {
	cobra.OnInitialize(initConfig)

	// 主命令标志
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "配置文件路径 (默认: ./configs/config.yaml)")

	// Query命令标志
	queryCmd.Flags().StringSliceP("engines", "e", []string{}, "指定使用的引擎 (fofa, hunter, zoomeye, quake)")
	queryCmd.Flags().IntP("page-size", "s", 100, "每页结果数量")
	queryCmd.Flags().IntP("max-pages", "p", 1, "最大页数")
	queryCmd.Flags().StringP("output", "o", "table", "输出格式 (table, json)")

	// 添加命令到根命令
	rootCmd.AddCommand(queryCmd)
	rootCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(listEnginesCmd)
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
		viper.AddConfigPath("./configs")
		viper.AddConfigPath(".")
	}

	viper.AutomaticEnv()
	viper.SetEnvPrefix("UNIMAP")

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			fmt.Printf("警告: 无法读取配置文件: %v\n", err)
		}
	}
}

func initLogger() {
	if logger != nil {
		return
	}

	logLevel := viper.GetString("app.log_level")
	if logLevel == "" {
		logLevel = "info"
	}

	var level zapcore.Level
	switch logLevel {
	case "debug":
		level = zapcore.DebugLevel
	case "info":
		level = zapcore.InfoLevel
	case "warn":
		level = zapcore.WarnLevel
	case "error":
		level = zapcore.ErrorLevel
	default:
		level = zapcore.InfoLevel
	}

	config := zap.Config{
		Level:            zap.NewAtomicLevelAt(level),
		Development:      false,
		Encoding:         "console",
		EncoderConfig:    zap.NewProductionEncoderConfig(),
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
	}

	config.EncoderConfig.TimeKey = "timestamp"
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	var err error
	logger, err = config.Build()
	if err != nil {
		panic(fmt.Sprintf("Failed to initialize logger: %v", err))
	}
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
