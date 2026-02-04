package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/unimap-icp-hunter/project/internal/adapter"
	"github.com/unimap-icp-hunter/project/internal/core/unimap"
	"github.com/unimap-icp-hunter/project/internal/exporter"
	"github.com/unimap-icp-hunter/project/internal/model"
)

// EngineConfig 引擎配置
type EngineConfig struct {
	Name    string
	Enabled bool
	APIKey  string
	Email   string // 仅FOFA需要
	BaseURL string
}

// AppState 应用状态
type AppState struct {
	Engines      map[string]*EngineConfig
	QueryResults []model.UnifiedAsset
	Orchestrator *adapter.EngineOrchestrator
}

func main() {
	myApp := app.New()
	myWindow := myApp.NewWindow("UniMap - 网络空间资产查询工具")
	myWindow.Resize(fyne.NewSize(1200, 800))

	// 初始化应用状态
	state := &AppState{
		Engines: map[string]*EngineConfig{
			"fofa": {
				Name:    "FOFA",
				Enabled: false,
				BaseURL: "https://fofa.info",
			},
			"hunter": {
				Name:    "Hunter",
				Enabled: false,
				BaseURL: "https://hunter.qianxin.com",
			},
			"zoomeye": {
				Name:    "ZoomEye",
				Enabled: false,
				BaseURL: "https://api.zoomeye.org",
			},
			"quake": {
				Name:    "Quake",
				Enabled: false,
				BaseURL: "https://quake.360.cn",
			},
		},
		QueryResults: []model.UnifiedAsset{},
		Orchestrator: adapter.NewEngineOrchestrator(),
	}

	// 创建UI组件
	content := createMainUI(myWindow, state)
	myWindow.SetContent(content)
	myWindow.ShowAndRun()
}

func createMainUI(window fyne.Window, state *AppState) fyne.CanvasObject {
	// 顶部：引擎配置按钮
	configBtn := widget.NewButton("配置引擎 API Key", func() {
		showEngineConfigDialog(window, state)
	})

	// 查询输入区
	queryEntry := widget.NewMultiLineEntry()
	queryEntry.SetPlaceHolder("输入查询语句 (支持 UQL 或原生语法)\n例如: country=\"CN\" && port=\"80\"")
	queryEntry.SetMinRowsVisible(3)

	// 引擎选择区
	engineChecks := make(map[string]*widget.Check)
	var engineCheckboxes []fyne.CanvasObject
	for key, cfg := range state.Engines {
		key := key
		check := widget.NewCheck(cfg.Name, func(checked bool) {
			state.Engines[key].Enabled = checked
		})
		engineChecks[key] = check
		engineCheckboxes = append(engineCheckboxes, check)
	}

	engineBox := container.NewVBox(
		widget.NewLabel("选择引擎:"),
		container.NewGridWithColumns(4, engineCheckboxes...),
	)

	// 操作按钮
	statusLabel := widget.NewLabel("就绪")
	progressBar := widget.NewProgressBarInfinite()
	progressBar.Hide()

	// 结果表格
	resultData := binding.NewUntypedList()
	resultTable := widget.NewTable(
		func() (int, int) {
			if len(state.QueryResults) == 0 {
				return 0, 0
			}
			return len(state.QueryResults), 9 // IP, Port, Protocol, Host, URL, Title, Server, Country, Source
		},
		func() fyne.CanvasObject {
			return widget.NewLabel("")
		},
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			label := cell.(*widget.Label)
			if id.Row >= len(state.QueryResults) {
				label.SetText("")
				return
			}
			asset := state.QueryResults[id.Row]
			switch id.Col {
			case 0:
				label.SetText(asset.IP)
			case 1:
				label.SetText(fmt.Sprintf("%d", asset.Port))
			case 2:
				label.SetText(asset.Protocol)
			case 3:
				label.SetText(asset.Host)
			case 4:
				label.SetText(asset.URL)
			case 5:
				label.SetText(asset.Title)
			case 6:
				label.SetText(asset.Server)
			case 7:
				label.SetText(asset.CountryCode)
			case 8:
				label.SetText(asset.Source)
			}
		},
	)

	// 设置列宽
	resultTable.SetColumnWidth(0, 120) // IP
	resultTable.SetColumnWidth(1, 60)  // Port
	resultTable.SetColumnWidth(2, 80)  // Protocol
	resultTable.SetColumnWidth(3, 150) // Host
	resultTable.SetColumnWidth(4, 200) // URL
	resultTable.SetColumnWidth(5, 200) // Title
	resultTable.SetColumnWidth(6, 100) // Server
	resultTable.SetColumnWidth(7, 60)  // Country
	resultTable.SetColumnWidth(8, 80)  // Source

	// 开始查询按钮
	startBtn := widget.NewButton("开始查询", func() {
		query := strings.TrimSpace(queryEntry.Text)
		if query == "" {
			dialog.ShowError(fmt.Errorf("请输入查询语句"), window)
			return
		}

		// 检查是否选择了引擎
		selectedEngines := []string{}
		for key, cfg := range state.Engines {
			if cfg.Enabled && cfg.APIKey != "" {
				selectedEngines = append(selectedEngines, key)
			}
		}

		if len(selectedEngines) == 0 {
			dialog.ShowError(fmt.Errorf("请至少选择并配置一个引擎"), window)
			return
		}

		// 显示进度
		statusLabel.SetText("正在查询...")
		progressBar.Show()
		startBtn.Disable()

		// 异步执行查询
		go func() {
			defer func() {
				progressBar.Hide()
				startBtn.Enable()
			}()

			// 重新初始化编排器和注册引擎
			state.Orchestrator = adapter.NewEngineOrchestrator()
			registerEngines(state)

			// 解析UQL
			parser := unimap.NewUQLParser()
			ast, err := parser.Parse(query)
			if err != nil {
				statusLabel.SetText(fmt.Sprintf("解析错误: %v", err))
				dialog.ShowError(fmt.Errorf("查询语法错误: %v", err), window)
				return
			}

			// 执行查询
			results := []model.UnifiedAsset{}
			for _, engineName := range selectedEngines {
				engineAdapter := state.Orchestrator.GetAdapter(engineName)
				if engineAdapter == nil {
					statusLabel.SetText(fmt.Sprintf("%s 引擎未正确初始化，跳过", engineName))
					continue
				}

				// 转换查询
				engineQuery, err := engineAdapter.Translate(ast)
				if err != nil {
					statusLabel.SetText(fmt.Sprintf("%s 转换失败: %v", engineName, err))
					continue
				}

				// 执行搜索
				rawResult, err := engineAdapter.Search(engineQuery, 1, 100)
				if err != nil {
					statusLabel.SetText(fmt.Sprintf("%s 查询失败: %v", engineName, err))
					continue
				}

				// 标准化结果
				assets, err := engineAdapter.Normalize(rawResult)
				if err != nil {
					statusLabel.SetText(fmt.Sprintf("%s 结果解析失败: %v", engineName, err))
					continue
				}

				results = append(results, assets...)
			}

			// 去重合并
			merger := unimap.NewResultMerger()
			merged := merger.Merge(results)

			// 更新结果
			state.QueryResults = make([]model.UnifiedAsset, 0, len(merged.Assets))
			for _, asset := range merged.Assets {
				state.QueryResults = append(state.QueryResults, *asset)
			}

			statusLabel.SetText(fmt.Sprintf("查询完成: 找到 %d 条结果 (去重后)", len(state.QueryResults)))
			resultTable.Refresh()
			resultData.Reload()
		}()
	})

	// 导出按钮
	exportJSONBtn := widget.NewButton("导出 JSON", func() {
		if len(state.QueryResults) == 0 {
			dialog.ShowInformation("提示", "没有可导出的结果", window)
			return
		}

		dialog.ShowFileSave(func(writer fyne.URIWriteCloser, err error) {
			if err != nil || writer == nil {
				return
			}
			defer writer.Close()

			filepath := writer.URI().Path()
			exporter := exporter.NewJSONExporter()
			if err := exporter.Export(state.QueryResults, filepath); err != nil {
				dialog.ShowError(fmt.Errorf("导出失败: %v", err), window)
				return
			}

			dialog.ShowInformation("成功", fmt.Sprintf("已导出 %d 条结果到 %s", len(state.QueryResults), filepath), window)
		}, window)
	})

	exportExcelBtn := widget.NewButton("导出 Excel", func() {
		if len(state.QueryResults) == 0 {
			dialog.ShowInformation("提示", "没有可导出的结果", window)
			return
		}

		dialog.ShowFileSave(func(writer fyne.URIWriteCloser, err error) {
			if err != nil || writer == nil {
				return
			}
			defer writer.Close()

			filepath := writer.URI().Path()
			exporter := exporter.NewExcelExporter()
			if err := exporter.Export(state.QueryResults, filepath); err != nil {
				dialog.ShowError(fmt.Errorf("导出失败: %v", err), window)
				return
			}

			dialog.ShowInformation("成功", fmt.Sprintf("已导出 %d 条结果到 %s", len(state.QueryResults), filepath), window)
		}, window)
	})

	// 布局
	topBar := container.NewBorder(nil, nil, nil, configBtn, widget.NewLabel("UniMap 查询工具"))

	queryArea := container.NewBorder(
		widget.NewLabel("查询输入:"),
		nil, nil, nil,
		queryEntry,
	)

	operationBar := container.NewHBox(
		startBtn,
		exportJSONBtn,
		exportExcelBtn,
	)

	statusBar := container.NewBorder(nil, nil, statusLabel, nil, progressBar)

	resultsArea := container.NewBorder(
		widget.NewLabel("查询结果:"),
		nil, nil, nil,
		container.NewScroll(resultTable),
	)

	mainContent := container.NewBorder(
		container.NewVBox(
			topBar,
			widget.NewSeparator(),
			queryArea,
			engineBox,
			operationBar,
			statusBar,
			widget.NewSeparator(),
		),
		nil, nil, nil,
		resultsArea,
	)

	return mainContent
}

func showEngineConfigDialog(window fyne.Window, state *AppState) {
	entries := make(map[string]*widget.Entry)
	emailEntry := widget.NewEntry()
	emailEntry.SetPlaceHolder("your_email@example.com")

	var items []fyne.CanvasObject
	items = append(items, widget.NewLabel("配置引擎 API Key"))
	items = append(items, widget.NewSeparator())

	// FOFA配置 (需要email)
	items = append(items, widget.NewLabel("FOFA:"))
	fofaKey := widget.NewEntry()
	fofaKey.SetPlaceHolder("FOFA API Key")
	if state.Engines["fofa"].APIKey != "" {
		fofaKey.SetText(state.Engines["fofa"].APIKey)
	}
	items = append(items, fofaKey)
	entries["fofa"] = fofaKey

	items = append(items, widget.NewLabel("FOFA Email:"))
	if state.Engines["fofa"].Email != "" {
		emailEntry.SetText(state.Engines["fofa"].Email)
	}
	items = append(items, emailEntry)
	items = append(items, widget.NewSeparator())

	// 其他引擎配置
	for key, cfg := range state.Engines {
		if key == "fofa" {
			continue // 已处理
		}
		items = append(items, widget.NewLabel(cfg.Name+":"))
		entry := widget.NewEntry()
		entry.SetPlaceHolder(cfg.Name + " API Key")
		if cfg.APIKey != "" {
			entry.SetText(cfg.APIKey)
		}
		items = append(items, entry)
		entries[key] = entry
		items = append(items, widget.NewSeparator())
	}

	content := container.NewVBox(items...)

	d := dialog.NewCustom("引擎配置", "保存", content, window)
	d.SetOnClosed(func() {
		// 保存配置
		for key, entry := range entries {
			state.Engines[key].APIKey = strings.TrimSpace(entry.Text)
		}
		state.Engines["fofa"].Email = strings.TrimSpace(emailEntry.Text)

		// 重新注册引擎
		registerEngines(state)
	})
	d.Resize(fyne.NewSize(500, 600))
	d.Show()
}

func registerEngines(state *AppState) {
	// 清空现有引擎
	state.Orchestrator = adapter.NewEngineOrchestrator()

	// 注册FOFA
	if cfg := state.Engines["fofa"]; cfg.APIKey != "" && cfg.Email != "" {
		fofaAdapter := adapter.NewFofaAdapter(
			cfg.BaseURL,
			cfg.APIKey,
			cfg.Email,
			10,
			30*time.Second,
		)
		state.Orchestrator.RegisterAdapter(fofaAdapter)
	}

	// 注册Hunter
	if cfg := state.Engines["hunter"]; cfg.APIKey != "" {
		hunterAdapter := adapter.NewHunterAdapter(
			cfg.BaseURL,
			cfg.APIKey,
			10,
			30*time.Second,
		)
		state.Orchestrator.RegisterAdapter(hunterAdapter)
	}

	// 注册ZoomEye
	if cfg := state.Engines["zoomeye"]; cfg.APIKey != "" {
		zoomeyeAdapter := adapter.NewZoomEyeAdapter(
			cfg.BaseURL,
			cfg.APIKey,
			10,
			30*time.Second,
		)
		state.Orchestrator.RegisterAdapter(zoomeyeAdapter)
	}

	// 注册Quake
	if cfg := state.Engines["quake"]; cfg.APIKey != "" {
		quakeAdapter := adapter.NewQuakeAdapter(
			cfg.BaseURL,
			cfg.APIKey,
			10,
			30*time.Second,
		)
		state.Orchestrator.RegisterAdapter(quakeAdapter)
	}
}
