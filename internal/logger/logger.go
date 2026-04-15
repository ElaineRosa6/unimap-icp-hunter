package logger

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/natefinch/lumberjack"
	unierror "github.com/unimap-icp-hunter/project/internal/error"
	"github.com/unimap-icp-hunter/project/internal/requestid"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	// Log 全局日志实例
	Log *zap.Logger
	// Sugar 全局SugarLogger实例（用于更方便的日志记录）
	Sugar *zap.SugaredLogger
	// fileHandle 日志文件句柄，用于关闭
	fileHandle *os.File
	// levelManager 动态日志级别管理器
	levelManager *LevelManager
	// logChan 异步日志通道
	logChan chan *logEntry
	// wg 等待组，用于异步日志写入
	wg sync.WaitGroup
	// asyncRunning 异步日志运行状态
	asyncRunning atomic.Bool
)

// Level 日志级别
type Level string

const (
	// LevelDebug 调试级别
	LevelDebug Level = "debug"
	// LevelInfo 信息级别
	LevelInfo Level = "info"
	// LevelWarn 警告级别
	LevelWarn Level = "warn"
	// LevelError 错误级别
	LevelError Level = "error"
	// LevelFatal 致命级别
	LevelFatal Level = "fatal"
)

// logEntry 异步日志条目
type logEntry struct {
	level   zapcore.Level
	msg     string
	fields  []zap.Field
	callers []zapcore.EntryCaller
}

// Config 日志配置
type Config struct {
	Level    Level
	Encoding string // console 或 json
	File     string // 可选的日志文件路径
	// 日志轮转配置
	Rotate     bool // 是否启用日志轮转
	MaxSize    int  // 单个日志文件最大大小（MB）
	MaxBackups int  // 保留的最大文件数
	MaxAge     int  // 保留的最大天数
	Compress   bool // 是否压缩归档
	// 结构化字段配置
	AppName     string // 应用名称
	Environment string // 环境（development, test, production）
	Version     string // 版本号
	Hostname    string // 主机名
	// 异步日志配置
	Async      bool // 是否启用异步日志
	BufferSize int  // 异步日志缓冲区大小
}

// LevelManager 动态日志级别管理器
type LevelManager struct {
	globalLevel  zap.AtomicLevel
	moduleLevels map[string]zap.AtomicLevel
	mutex        sync.RWMutex
}

// NewLevelManager 创建新的级别管理器
func NewLevelManager(initialLevel zapcore.Level) *LevelManager {
	return &LevelManager{
		globalLevel:  zap.NewAtomicLevelAt(initialLevel),
		moduleLevels: make(map[string]zap.AtomicLevel),
	}
}

// GetGlobalLevel 获取全局日志级别
func (lm *LevelManager) GetGlobalLevel() zapcore.Level {
	return lm.globalLevel.Level()
}

// SetGlobalLevel 设置全局日志级别
func (lm *LevelManager) SetGlobalLevel(level Level) {
	zapLevel := parseLevel(level)
	lm.globalLevel.SetLevel(zapLevel)
}

// GetModuleLevel 获取特定模块的日志级别
func (lm *LevelManager) GetModuleLevel(module string) zapcore.Level {
	lm.mutex.RLock()
	defer lm.mutex.RUnlock()

	if level, exists := lm.moduleLevels[module]; exists {
		return level.Level()
	}
	return lm.globalLevel.Level()
}

// SetModuleLevel 设置特定模块的日志级别
func (lm *LevelManager) SetModuleLevel(module string, level Level) {
	zapLevel := parseLevel(level)

	lm.mutex.Lock()
	defer lm.mutex.Unlock()

	if _, exists := lm.moduleLevels[module]; !exists {
		lm.moduleLevels[module] = zap.NewAtomicLevelAt(zapLevel)
	} else {
		lm.moduleLevels[module].SetLevel(zapLevel)
	}
}

// DeleteModuleLevel 删除模块级别配置，回退到全局级别
func (lm *LevelManager) DeleteModuleLevel(module string) {
	lm.mutex.Lock()
	defer lm.mutex.Unlock()

	delete(lm.moduleLevels, module)
}

// Init 初始化日志
func Init(cfg Config) {
	// 解析日志级别
	level := parseLevel(cfg.Level)

	// 设置默认值
	if cfg.MaxSize <= 0 {
		cfg.MaxSize = 100 // 默认 100MB
	}
	if cfg.MaxBackups <= 0 {
		cfg.MaxBackups = 10 // 默认保留 10 个文件
	}
	if cfg.MaxAge <= 0 {
		cfg.MaxAge = 30 // 默认保留 30 天
	}

	// 设置默认结构化字段
	if cfg.AppName == "" {
		cfg.AppName = "unimap-project"
	}
	if cfg.Environment == "" {
		cfg.Environment = "development"
	}
	if cfg.Version == "" {
		cfg.Version = "1.0.0"
	}
	if cfg.Hostname == "" {
		hostname, err := os.Hostname()
		if err != nil {
			hostname = "unknown"
		}
		cfg.Hostname = hostname
	}

	// 设置默认异步日志配置
	if cfg.BufferSize <= 0 {
		cfg.BufferSize = 1024 // 默认缓冲区大小
	}

	// 创建级别管理器
	levelManager = NewLevelManager(level)

	// 创建Core
	core := newCore(levelManager.globalLevel, cfg.Encoding, cfg.File, cfg.Rotate, cfg.MaxSize, cfg.MaxBackups, cfg.MaxAge, cfg.Compress)

	// 创建基础字段
	baseFields := []zap.Field{
		zap.String("app", cfg.AppName),
		zap.String("env", cfg.Environment),
		zap.String("version", cfg.Version),
		zap.String("hostname", cfg.Hostname),
	}

	// 创建Logger
	Log = zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1)).With(baseFields...)
	Sugar = Log.Sugar()

	// 如果启用异步日志，初始化异步通道和goroutine
	if cfg.Async {
		logChan = make(chan *logEntry, cfg.BufferSize)
		asyncRunning.Store(true)
		wg.Add(1)
		go asyncLogWriter()
	}
}

// Sync 同步日志缓冲，应在应用退出前调用
func Sync() error {
	var errs []error

	// 如果启用了异步日志，关闭通道并等待所有日志写入完成
	if asyncRunning.Load() && logChan != nil {
		close(logChan)
		wg.Wait()
		asyncRunning.Store(false)
	}

	if Log != nil {
		if err := Log.Sync(); err != nil {
			errs = append(errs, err)
		}
	}
	if Sugar != nil {
		if err := Sugar.Sync(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// Close 关闭日志文件句柄，应在应用退出前调用
func Close() error {
	if fileHandle != nil {
		return fileHandle.Close()
	}
	return nil
}

// newCore 创建zap.Core
func newCore(level zap.AtomicLevel, encoding string, file string, rotate bool, maxSize, maxBackups, maxAge int, compress bool) zapcore.Core {
	// 编码器配置
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	// 创建编码器
	var encoder zapcore.Encoder
	if encoding == "json" {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	} else {
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	}

	// 创建输出
	var writeSyncer zapcore.WriteSyncer
	if file != "" {
		// 确保目录存在（使用 filepath.Dir 兼容 Windows）
		fileDir := filepath.Dir(file)
		if fileDir != "" && fileDir != "." {
			os.MkdirAll(fileDir, 0755)
		}

		var writer zapcore.WriteSyncer
		if rotate {
			// 使用 lumberjack 实现日志轮转
			lumberjackLogger := &lumberjack.Logger{
				Filename:   file,
				MaxSize:    maxSize,    // MB
				MaxBackups: maxBackups, // 保留的文件数
				MaxAge:     maxAge,     // 保留的天数
				Compress:   compress,   // 是否压缩
			}
			writer = zapcore.AddSync(lumberjackLogger)
		} else {
			// 创建文件（权限 0600，包含敏感信息）
			f, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
			if err != nil {
				// 文件创建失败，使用标准输出
				writer = zapcore.AddSync(os.Stdout)
			} else {
				// 保存文件句柄以便关闭
				fileHandle = f
				writer = zapcore.AddSync(f)
			}
		}

		// 同时输出到文件和标准输出
		writeSyncer = zapcore.NewMultiWriteSyncer(
			writer,
			zapcore.AddSync(os.Stdout),
		)
	} else {
		// 只输出到标准输出
		writeSyncer = zapcore.AddSync(os.Stdout)
	}

	return zapcore.NewCore(encoder, writeSyncer, level)
}

// parseLevel 解析日志级别
func parseLevel(level Level) zapcore.Level {
	switch strings.ToLower(string(level)) {
	case string(LevelDebug):
		return zapcore.DebugLevel
	case string(LevelInfo):
		return zapcore.InfoLevel
	case string(LevelWarn):
		return zapcore.WarnLevel
	case string(LevelError):
		return zapcore.ErrorLevel
	case string(LevelFatal):
		return zapcore.FatalLevel
	default:
		return zapcore.InfoLevel
	}
}

// Debug 记录调试级别日志
func Debug(msg string, fields ...zap.Field) {
	if Log != nil {
		Log.Debug(msg, fields...)
	}
}

// Debugf 使用SugarLogger记录调试级别日志（格式化）
func Debugf(template string, args ...interface{}) {
	if Sugar != nil {
		Sugar.Debugf(template, args...)
	}
}

// Info 记录信息级别日志
func Info(msg string, fields ...zap.Field) {
	if Log != nil {
		Log.Info(msg, fields...)
	}
}

// Infof 使用SugarLogger记录信息级别日志（格式化）
func Infof(template string, args ...interface{}) {
	if Sugar != nil {
		Sugar.Infof(template, args...)
	}
}

// Warn 记录警告级别日志
func Warn(msg string, fields ...zap.Field) {
	if Log != nil {
		Log.Warn(msg, fields...)
	}
}

// Warnf 使用SugarLogger记录警告级别日志（格式化）
func Warnf(template string, args ...interface{}) {
	if Sugar != nil {
		Sugar.Warnf(template, args...)
	}
}

// Error 记录错误级别日志
func Error(msg string, fields ...zap.Field) {
	if Log != nil {
		Log.Error(msg, fields...)
	}
}

// Errorf 使用SugarLogger记录错误级别日志（格式化）
func Errorf(template string, args ...interface{}) {
	if Sugar != nil {
		Sugar.Errorf(template, args...)
	}
}

// Fatal 记录致命级别日志并退出
func Fatal(msg string, fields ...zap.Field) {
	if Log != nil {
		Log.Fatal(msg, fields...)
	}
}

// Fatalf 使用SugarLogger记录致命级别日志并退出（格式化）
func Fatalf(template string, args ...interface{}) {
	if Sugar != nil {
		Sugar.Fatalf(template, args...)
	}
}

// With 创建一个带有额外字段的Logger
func With(fields ...zap.Field) *zap.Logger {
	if Log != nil {
		return Log.With(fields...)
	}
	return nil
}

// WithSugar 创建一个带有额外字段的SugarLogger
func WithSugar(args ...interface{}) *zap.SugaredLogger {
	if Sugar != nil {
		return Sugar.With(args...)
	}
	return nil
}

func withRIDPrefix(ctx context.Context, template string) string {
	rid := requestid.FromContext(ctx)
	if rid == "" {
		return template
	}
	return "[rid=" + rid + "] " + template
}

func CtxDebugf(ctx context.Context, template string, args ...interface{}) {
	Debugf(withRIDPrefix(ctx, template), args...)
}

func CtxInfof(ctx context.Context, template string, args ...interface{}) {
	Infof(withRIDPrefix(ctx, template), args...)
}

func CtxWarnf(ctx context.Context, template string, args ...interface{}) {
	Warnf(withRIDPrefix(ctx, template), args...)
}

func CtxErrorf(ctx context.Context, template string, args ...interface{}) {
	Errorf(withRIDPrefix(ctx, template), args...)
}

// ErrorWithDetails 记录带有详细信息的错误日志
func ErrorWithDetails(err error, msg string, fields ...zap.Field) {
	if Log == nil {
		return
	}

	if ue, ok := err.(*unierror.UnimapError); ok {
		// 使用结构化日志记录统一错误
		Log.Error(msg,
			zap.String("error_type", string(ue.Type)),
			zap.Int("error_code", ue.Code),
			zap.String("error_message", ue.Message),
			zap.String("error_details", ue.Details),
			zap.String("stack_trace", ue.StackTrace),
			zap.Error(ue.OriginalErr),
		)
		return
	}

	// 记录普通错误
	Log.Error(msg, zap.Error(err))
}

// CtxErrorWithDetails 记录带有请求ID和详细信息的错误日志
func CtxErrorWithDetails(ctx context.Context, err error, msg string, fields ...zap.Field) {
	if Log == nil {
		return
	}

	rid := requestid.FromContext(ctx)
	fields = append(fields, zap.String("request_id", rid))

	if ue, ok := err.(*unierror.UnimapError); ok {
		// 使用结构化日志记录统一错误
		Log.Error(msg,
			zap.String("request_id", rid),
			zap.String("error_type", string(ue.Type)),
			zap.Int("error_code", ue.Code),
			zap.String("error_message", ue.Message),
			zap.String("error_details", ue.Details),
			zap.String("stack_trace", ue.StackTrace),
			zap.Error(ue.OriginalErr),
		)
		return
	}

	// 记录普通错误
	Log.Error(msg, zap.String("request_id", rid), zap.Error(err))
}

// SetGlobalLevel 动态设置全局日志级别
func SetGlobalLevel(level Level) {
	if levelManager != nil {
		levelManager.SetGlobalLevel(level)
	}
}

// GetGlobalLevel 获取当前全局日志级别
func GetGlobalLevel() Level {
	if levelManager != nil {
		zapLevel := levelManager.GetGlobalLevel()
		return Level(zapLevel.String())
	}
	return LevelInfo
}

// SetModuleLevel 动态设置特定模块的日志级别
func SetModuleLevel(module string, level Level) {
	if levelManager != nil {
		levelManager.SetModuleLevel(module, level)
	}
}

// GetModuleLevel 获取特定模块的日志级别
func GetModuleLevel(module string) Level {
	if levelManager != nil {
		zapLevel := levelManager.GetModuleLevel(module)
		return Level(zapLevel.String())
	}
	return LevelInfo
}

// DeleteModuleLevel 删除模块级别配置，回退到全局级别
func DeleteModuleLevel(module string) {
	if levelManager != nil {
		levelManager.DeleteModuleLevel(module)
	}
}

// PerfInfof 记录性能指标日志
func PerfInfof(ctx context.Context, operation string, duration time.Duration, fields ...zap.Field) {
	if Log == nil {
		return
	}

	rid := requestid.FromContext(ctx)
	allFields := append(fields,
		zap.String("operation", operation),
		zap.Duration("duration", duration),
		zap.String("request_id", rid),
	)

	Log.Info("performance_metric", allFields...)
}

// PerfDebugf 记录调试级别的性能指标日志
func PerfDebugf(ctx context.Context, operation string, duration time.Duration, fields ...zap.Field) {
	if Log == nil {
		return
	}

	rid := requestid.FromContext(ctx)
	allFields := append(fields,
		zap.String("operation", operation),
		zap.Duration("duration", duration),
		zap.String("request_id", rid),
	)

	Log.Debug("performance_metric", allFields...)
}

// asyncLogWriter 异步日志写入器
func asyncLogWriter() {
	defer wg.Done()

	for entry := range logChan {
		if Log == nil {
			continue
		}

		switch entry.level {
		case zapcore.DebugLevel:
			Log.Debug(entry.msg, entry.fields...)
		case zapcore.InfoLevel:
			Log.Info(entry.msg, entry.fields...)
		case zapcore.WarnLevel:
			Log.Warn(entry.msg, entry.fields...)
		case zapcore.ErrorLevel:
			Log.Error(entry.msg, entry.fields...)
		case zapcore.FatalLevel:
			Log.Fatal(entry.msg, entry.fields...)
		}
	}
}
