//go:build gui
// +build gui

package main

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/unimap-icp-hunter/project/internal/adapter"
	"github.com/unimap-icp-hunter/project/internal/config"
	"github.com/unimap-icp-hunter/project/internal/exporter"
	"github.com/unimap-icp-hunter/project/internal/model"
	"github.com/unimap-icp-hunter/project/internal/screenshot"
	"github.com/unimap-icp-hunter/project/internal/service"
	"github.com/unimap-icp-hunter/project/internal/tamper"
)

const configPath = "configs/config.yaml"

// AppState 应用状态
type AppState struct {
	ConfigManager *config.Manager
	Config        *config.Config
	QueryResults  []model.UnifiedAsset
	Service       *service.UnifiedService
	Detector      *tamper.Detector
	TamperStorage *tamper.HashStorage
	ScreenshotMgr *screenshot.Manager
}

func main() {
	myApp := app.New()
	if t := newCJKTheme(); t != nil {
		myApp.Settings().SetTheme(t)
	}
	myWindow := myApp.NewWindow("UniMap - 网络空间资产查询工具")
	myWindow.Resize(fyne.NewSize(1200, 800))

	// 初始化配置
	cfgManager := config.NewManager(configPath)
	if err := cfgManager.Load(); err != nil {
		fmt.Printf("Warning: Failed to load config: %v\n", err)
	}
	cfg := cfgManager.GetConfig()
	if cfg == nil {
		// Should not happen if Load defaults handles it, but safety check
		dialog.ShowError(fmt.Errorf("无法加载配置"), myWindow)
	}

	svc := service.NewUnifiedService()

	// 初始化应用状态
	state := &AppState{
		ConfigManager: cfgManager,
		Config:        cfg,
		QueryResults:  []model.UnifiedAsset{},
		Service:       svc,
		Detector:      tamper.NewDetector(tamper.DetectorConfig{BaseDir: "./hash_store"}),
		TamperStorage: tamper.NewHashStorage("./hash_store"),
		ScreenshotMgr: buildScreenshotManager(cfg),
	}

	// 创建UI组件
	content := createMainUI(myWindow, state)
	myWindow.SetContent(content)
	myWindow.ShowAndRun()
}

func createMainUI(window fyne.Window, state *AppState) fyne.CanvasObject {
	tabs := container.NewAppTabs(
		container.NewTabItem("资产查询", createQueryTab(window, state)),
		container.NewTabItem("URL监控", createMonitorTab(window, state)),
		container.NewTabItem("历史记录", createHistoryTab(window, state)),
		container.NewTabItem("截图管理", createScreenshotTab(window, state)),
	)
	tabs.SetTabLocation(container.TabLocationTop)
	return tabs
}

func createQueryTab(window fyne.Window, state *AppState) fyne.CanvasObject {
	// --- 1. 顶部：标题与配置 ---
	configBtn := widget.NewButtonWithIcon("配置", theme.SettingsIcon(), func() {
		showEngineConfigDialog(window, state)
	})
	helpBtn := widget.NewButtonWithIcon("帮助", theme.HelpIcon(), func() {
		showHelpDialog(window)
	})

	topHeader := container.NewHBox(
		widget.NewIcon(theme.SearchIcon()),
		widget.NewLabelWithStyle("UniMap 资产查询", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		layout.NewSpacer(),
		helpBtn,
		configBtn,
	)

	// --- 2. 查询输入区 ---
	queryEntry := widget.NewMultiLineEntry()
	queryEntry.SetPlaceHolder("输入查询语句 (UQL)... \n例如: app=\"nginx\" && country=\"CN\"")
	queryEntry.SetMinRowsVisible(3)

	limitEntry := widget.NewEntry()
	limitEntry.SetPlaceHolder("100")
	limitEntry.SetText("100")
	limitContainer := container.NewHBox(widget.NewLabel("数量限制:"), limitEntry)

	// --- 3. 引擎选择区 ---
	fofaCheck := widget.NewCheck("FOFA", func(b bool) { state.Config.Engines.Fofa.Enabled = b })
	fofaCheck.SetChecked(state.Config.Engines.Fofa.Enabled)

	hunterCheck := widget.NewCheck("Hunter", func(b bool) { state.Config.Engines.Hunter.Enabled = b })
	hunterCheck.SetChecked(state.Config.Engines.Hunter.Enabled)

	quakeCheck := widget.NewCheck("Quake", func(b bool) { state.Config.Engines.Quake.Enabled = b })
	quakeCheck.SetChecked(state.Config.Engines.Quake.Enabled)

	zoomeyeCheck := widget.NewCheck("ZoomEye", func(b bool) { state.Config.Engines.Zoomeye.Enabled = b })
	zoomeyeCheck.SetChecked(state.Config.Engines.Zoomeye.Enabled)

	engineBox := container.NewHBox(
		widget.NewLabel("检索引擎:"),
		fofaCheck, hunterCheck, quakeCheck, zoomeyeCheck,
	)

	// --- 4. 状态栏 (预定义以供闭包使用) ---
	statusLabel := widget.NewLabel("就绪")
	statusLabel.TextStyle = fyne.TextStyle{Italic: true}
	resultCountLabel := widget.NewLabel("")
	progressBar := widget.NewProgressBarInfinite()
	progressBar.Hide()

	// --- 5. 结果表格 ---
	headers := []string{"IP", "Port", "Proto", "Host", "URL", "Title", "Server", "Country", "Source"}
	resultTable := widget.NewTable(
		func() (int, int) {
			rows := 1 // header
			if len(state.QueryResults) > 0 {
				rows = len(state.QueryResults) + 1
			}
			return rows, 9
		},
		func() fyne.CanvasObject {
			l := widget.NewLabel("sample text")
			l.Truncation = fyne.TextTruncateEllipsis
			return l
		},
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			label := cell.(*widget.Label)
			label.Alignment = fyne.TextAlignLeading
			label.TextStyle = fyne.TextStyle{}

			if id.Row == 0 {
				label.TextStyle = fyne.TextStyle{Bold: true}
				if id.Col >= 0 && id.Col < len(headers) {
					label.SetText(headers[id.Col])
				}
				return
			}

			if len(state.QueryResults) == 0 {
				if id.Col == 0 {
					label.SetText("暂无结果")
				} else {
					label.SetText("")
				}
				return
			}

			rowIdx := id.Row - 1
			if rowIdx < 0 || rowIdx >= len(state.QueryResults) {
				label.SetText("")
				return
			}

			asset := state.QueryResults[rowIdx]
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

	resultTable.SetColumnWidth(0, 130)
	resultTable.SetColumnWidth(1, 70)
	resultTable.SetColumnWidth(2, 70)
	resultTable.SetColumnWidth(3, 160)
	resultTable.SetColumnWidth(4, 220)
	resultTable.SetColumnWidth(5, 200)
	resultTable.SetColumnWidth(6, 120)
	resultTable.SetColumnWidth(7, 70)
	resultTable.SetColumnWidth(8, 80)

	resultTable.OnSelected = func(id widget.TableCellID) {
		if id.Row == 0 {
			resultTable.Unselect(id)
			return
		}
		rowIdx := id.Row - 1
		if rowIdx >= 0 && rowIdx < len(state.QueryResults) {
			asset := state.QueryResults[rowIdx]
			showAssetDetails(window, asset)
			resultTable.Unselect(id)
		}
	}

	// --- 6. 按钮与逻辑 ---
	var startBtn *widget.Button
	var exportJSONBtn *widget.Button
	var exportExcelBtn *widget.Button
	var clearBtn *widget.Button

	runOnUI := func(fn func()) {
		if fn != nil {
			fn()
		}
	}

	setBusy := func(busy bool) {
		runOnUI(func() {
			if busy {
				statusLabel.SetText("正在查询...")
				progressBar.Show()
				startBtn.Disable()
				if clearBtn != nil {
					clearBtn.Disable()
				}
				if exportJSONBtn != nil {
					exportJSONBtn.Disable()
				}
				if exportExcelBtn != nil {
					exportExcelBtn.Disable()
				}
				return
			}
			progressBar.Hide()
			startBtn.Enable()
			if clearBtn != nil {
				clearBtn.Enable()
			}
			if len(state.QueryResults) > 0 {
				if exportJSONBtn != nil {
					exportJSONBtn.Enable()
				}
				if exportExcelBtn != nil {
					exportExcelBtn.Enable()
				}
			}
		})
	}

	startBtn = widget.NewButtonWithIcon("立即查询", theme.MediaPlayIcon(), func() {
		query := strings.TrimSpace(queryEntry.Text)
		if query == "" {
			dialog.ShowError(fmt.Errorf("请输入查询语句"), window)
			return
		}

		limit := 100
		if v := strings.TrimSpace(limitEntry.Text); v != "" {
			parsed, err := strconv.Atoi(v)
			if err == nil && parsed > 0 {
				if parsed > 2000 {
					parsed = 2000
				}
				limit = parsed
			}
		}

		var engines []string
		if state.Config.Engines.Fofa.Enabled {
			engines = append(engines, "fofa")
		}
		if state.Config.Engines.Hunter.Enabled {
			engines = append(engines, "hunter")
		}
		if state.Config.Engines.Quake.Enabled {
			engines = append(engines, "quake")
		}
		if state.Config.Engines.Zoomeye.Enabled {
			engines = append(engines, "zoomeye")
		}

		if len(engines) == 0 {
			dialog.ShowError(fmt.Errorf("请至少选择一个引擎"), window)
			return
		}

		setBusy(true)

		go func() {
			defer setBusy(false)
			querySvc := service.NewUnifiedService()
			registerEngines(querySvc, state.Config)

			req := service.QueryRequest{
				Query:       query,
				Engines:     engines,
				PageSize:    limit,
				ProcessData: true,
			}

			resp, err := querySvc.Query(context.Background(), req)
			if err != nil {
				runOnUI(func() {
					statusLabel.SetText(fmt.Sprintf("出错: %v", err))
					dialog.ShowError(fmt.Errorf("查询失败: %v", err), window)
				})
				return
			}

			runOnUI(func() {
				state.QueryResults = resp.Assets
				resultCountLabel.SetText(fmt.Sprintf("%d 条结果", len(state.QueryResults)))
				statusLabel.SetText("查询完成")
				resultTable.Refresh()
			})
		}()
	})
	startBtn.Importance = widget.HighImportance

	clearBtn = widget.NewButtonWithIcon("清空结果", theme.ContentClearIcon(), func() {
		state.QueryResults = nil
		resultCountLabel.SetText("")
		statusLabel.SetText("就绪")
		exportJSONBtn.Disable()
		exportExcelBtn.Disable()
		resultTable.Refresh()
	})

	exportJSONBtn = widget.NewButtonWithIcon("JSON", theme.DocumentSaveIcon(), func() {
		dialog.ShowFileSave(func(writer fyne.URIWriteCloser, err error) {
			if err != nil || writer == nil {
				return
			}
			defer writer.Close()
			exporter := exporter.NewJSONExporter()
			if err := exporter.Export(state.QueryResults, writer.URI().Path()); err != nil {
				dialog.ShowError(err, window)
			} else {
				dialog.ShowInformation("成功", "导出完成", window)
			}
		}, window)
	})
	exportJSONBtn.Disable()

	exportExcelBtn = widget.NewButtonWithIcon("Excel", theme.DocumentCreateIcon(), func() {
		dialog.ShowFileSave(func(writer fyne.URIWriteCloser, err error) {
			if err != nil || writer == nil {
				return
			}
			defer writer.Close()
			ex := exporter.NewExcelExporter()
			if err := ex.Export(state.QueryResults, writer.URI().Path()); err != nil {
				dialog.ShowError(err, window)
			} else {
				dialog.ShowInformation("成功", "导出完成", window)
			}
		}, window)
	})
	exportExcelBtn.Disable()

	// --- 7. 布局组装 ---

	// 控制区第二行：引擎+数量
	controlRow1 := container.NewHBox(
		engineBox,
		layout.NewSpacer(),
		limitContainer,
	)

	// 控制区第三行：按钮
	controlRow2 := container.NewHBox(
		startBtn, clearBtn,
		layout.NewSpacer(),
		widget.NewLabel("导出:"), exportJSONBtn, exportExcelBtn,
	)

	// 顶部面板总成
	topPanel := container.NewVBox(
		container.NewPadded(topHeader),
		widget.NewSeparator(),
		container.NewPadded(container.NewVBox(
			widget.NewLabelWithStyle("查询语句 (UQL):", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			queryEntry,
			controlRow1,
			controlRow2,
		)),
		widget.NewSeparator(),
	)

	// 底部状态栏
	statusBar := container.NewHBox(
		widget.NewIcon(theme.InfoIcon()),
		statusLabel,
		layout.NewSpacer(),
		resultCountLabel,
		container.NewPadded(progressBar),
	)

	// 主布局： 上(TopPanel) - 下(StatusBar) - 中(Table)
	return container.NewBorder(
		topPanel,
		container.NewVBox(widget.NewSeparator(), container.NewPadded(statusBar)),
		nil, nil,
		resultTable, // Table 放在 Center 可自适应并支持内置滚动
	)
}

func buildScreenshotManager(cfg *config.Config) *screenshot.Manager {
	if cfg == nil || !cfg.Screenshot.Enabled {
		return nil
	}

	headless := true
	if cfg.Screenshot.Headless != nil {
		headless = *cfg.Screenshot.Headless
	}

	mgr := screenshot.NewManager(screenshot.Config{
		BaseDir:        cfg.Screenshot.BaseDir,
		ChromePath:     cfg.Screenshot.ChromePath,
		UserDataDir:    cfg.Screenshot.ChromeUserDataDir,
		ProfileDir:     cfg.Screenshot.ChromeProfileDir,
		RemoteDebugURL: strings.TrimSpace(cfg.Screenshot.ChromeRemoteDebugURL),
		Headless:       headless,
		Timeout:        time.Duration(cfg.Screenshot.Timeout) * time.Second,
		WindowWidth:    cfg.Screenshot.WindowWidth,
		WindowHeight:   cfg.Screenshot.WindowHeight,
		WaitTime:       time.Duration(cfg.Screenshot.WaitTime) * time.Millisecond,
	})

	if cfg.Engines.Fofa.Enabled && len(cfg.Engines.Fofa.Cookies) > 0 {
		mgr.SetCookies("fofa", convertConfigCookies(cfg.Engines.Fofa.Cookies))
	}
	if cfg.Engines.Hunter.Enabled && len(cfg.Engines.Hunter.Cookies) > 0 {
		mgr.SetCookies("hunter", convertConfigCookies(cfg.Engines.Hunter.Cookies))
	}
	if cfg.Engines.Quake.Enabled && len(cfg.Engines.Quake.Cookies) > 0 {
		mgr.SetCookies("quake", convertConfigCookies(cfg.Engines.Quake.Cookies))
	}
	if cfg.Engines.Zoomeye.Enabled && len(cfg.Engines.Zoomeye.Cookies) > 0 {
		mgr.SetCookies("zoomeye", convertConfigCookies(cfg.Engines.Zoomeye.Cookies))
	}

	return mgr
}

func convertConfigCookies(cfgCookies []config.Cookie) []screenshot.Cookie {
	cookies := make([]screenshot.Cookie, len(cfgCookies))
	for i, c := range cfgCookies {
		cookies[i] = screenshot.Cookie{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   c.Domain,
			Path:     c.Path,
			HTTPOnly: c.HTTPOnly,
			Secure:   c.Secure,
		}
	}
	return cookies
}

func openWebPage(window fyne.Window, route string) {
	baseURL := "http://127.0.0.1:8448"
	target, err := url.Parse(baseURL + route)
	if err != nil {
		dialog.ShowError(fmt.Errorf("无效的Web地址: %v", err), window)
		return
	}

	if err := fyne.CurrentApp().OpenURL(target); err != nil {
		dialog.ShowError(fmt.Errorf("打开失败，请先运行 Web 服务 (go run ./cmd/unimap-web): %v", err), window)
	}
}

func showEngineConfigDialog(window fyne.Window, state *AppState) {
	// Fofa
	fofaKey := widget.NewPasswordEntry()
	fofaKey.SetText(state.Config.Engines.Fofa.APIKey)
	fofaEmail := widget.NewEntry()
	fofaEmail.SetText(state.Config.Engines.Fofa.Email)
	fofaCookie := widget.NewMultiLineEntry()
	fofaCookie.SetText(cookiesToHeader(state.Config.Engines.Fofa.Cookies))
	fofaCookie.SetMinRowsVisible(2)
	fofaCookie.SetPlaceHolder("session=xxx; token=yyy")
	fofaCookieStatus := widget.NewLabel(cookieStatusText(fofaCookie.Text))

	// Hunter
	hunterKey := widget.NewPasswordEntry()
	hunterKey.SetText(state.Config.Engines.Hunter.APIKey)
	hunterCookie := widget.NewMultiLineEntry()
	hunterCookie.SetText(cookiesToHeader(state.Config.Engines.Hunter.Cookies))
	hunterCookie.SetMinRowsVisible(2)
	hunterCookie.SetPlaceHolder("session=xxx; token=yyy")
	hunterCookieStatus := widget.NewLabel(cookieStatusText(hunterCookie.Text))

	// Quake
	quakeKey := widget.NewPasswordEntry()
	quakeKey.SetText(state.Config.Engines.Quake.APIKey)
	quakeCookie := widget.NewMultiLineEntry()
	quakeCookie.SetText(cookiesToHeader(state.Config.Engines.Quake.Cookies))
	quakeCookie.SetMinRowsVisible(2)
	quakeCookie.SetPlaceHolder("session=xxx; token=yyy")
	quakeCookieStatus := widget.NewLabel(cookieStatusText(quakeCookie.Text))

	// Zoomeye
	zoomeyeKey := widget.NewPasswordEntry()
	zoomeyeKey.SetText(state.Config.Engines.Zoomeye.APIKey)
	zoomeyeCookie := widget.NewMultiLineEntry()
	zoomeyeCookie.SetText(cookiesToHeader(state.Config.Engines.Zoomeye.Cookies))
	zoomeyeCookie.SetMinRowsVisible(2)
	zoomeyeCookie.SetPlaceHolder("session=xxx; token=yyy")
	zoomeyeCookieStatus := widget.NewLabel(cookieStatusText(zoomeyeCookie.Text))

	fofaCookie.OnChanged = func(value string) {
		fofaCookieStatus.SetText(cookieStatusText(value))
	}
	hunterCookie.OnChanged = func(value string) {
		hunterCookieStatus.SetText(cookieStatusText(value))
	}
	quakeCookie.OnChanged = func(value string) {
		quakeCookieStatus.SetText(cookieStatusText(value))
	}
	zoomeyeCookie.OnChanged = func(value string) {
		zoomeyeCookieStatus.SetText(cookieStatusText(value))
	}

	form := widget.NewForm(
		widget.NewFormItem("FOFA Email", fofaEmail),
		widget.NewFormItem("FOFA API Key", fofaKey),
		widget.NewFormItem("FOFA Cookie (截图)", container.NewVBox(fofaCookie, fofaCookieStatus)),
		widget.NewFormItem("Hunter API Key", hunterKey),
		widget.NewFormItem("Hunter Cookie (截图)", container.NewVBox(hunterCookie, hunterCookieStatus)),
		widget.NewFormItem("Quake API Key", quakeKey),
		widget.NewFormItem("Quake Cookie (截图)", container.NewVBox(quakeCookie, quakeCookieStatus)),
		widget.NewFormItem("ZoomEye API Key", zoomeyeKey),
		widget.NewFormItem("ZoomEye Cookie (截图)", container.NewVBox(zoomeyeCookie, zoomeyeCookieStatus)),
	)

	dialog.ShowCustomConfirm("引擎配置", "保存", "取消", form, func(save bool) {
		if save {
			state.Config.Engines.Fofa.APIKey = fofaKey.Text
			state.Config.Engines.Fofa.Email = fofaEmail.Text
			state.Config.Engines.Fofa.Cookies = config.ParseCookieHeader(fofaCookie.Text, config.DefaultCookieDomain("fofa"))
			state.Config.Engines.Hunter.APIKey = hunterKey.Text
			state.Config.Engines.Hunter.Cookies = config.ParseCookieHeader(hunterCookie.Text, config.DefaultCookieDomain("hunter"))
			state.Config.Engines.Quake.APIKey = quakeKey.Text
			state.Config.Engines.Quake.Cookies = config.ParseCookieHeader(quakeCookie.Text, config.DefaultCookieDomain("quake"))
			state.Config.Engines.Zoomeye.APIKey = zoomeyeKey.Text
			state.Config.Engines.Zoomeye.Cookies = config.ParseCookieHeader(zoomeyeCookie.Text, config.DefaultCookieDomain("zoomeye"))

			if err := state.ConfigManager.Save(); err != nil {
				dialog.ShowError(fmt.Errorf("保存配置失败: %v", err), window)
			}
		}
	}, window)
}

func cookiesToHeader(cookies []config.Cookie) string {
	if len(cookies) == 0 {
		return ""
	}
	parts := make([]string, 0, len(cookies))
	for _, c := range cookies {
		if strings.TrimSpace(c.Name) == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%s", c.Name, c.Value))
	}
	return strings.Join(parts, "; ")
}

func cookieStatusText(value string) string {
	if strings.TrimSpace(value) == "" {
		return "状态: 未配置"
	}
	return "状态: 已配置"
}

func showAssetDetails(window fyne.Window, asset model.UnifiedAsset) {
	// 创建只读的详情表单
	form := widget.NewForm(
		widget.NewFormItem("IP", newReadonlyEntry(asset.IP)),
		widget.NewFormItem("Port", newReadonlyEntry(fmt.Sprintf("%d", asset.Port))),
		widget.NewFormItem("Protocol", newReadonlyEntry(asset.Protocol)),
		widget.NewFormItem("Host", newReadonlyEntry(asset.Host)),
		widget.NewFormItem("URL", newReadonlyEntry(asset.URL)),
		widget.NewFormItem("Title", newReadonlyEntry(asset.Title)),
		widget.NewFormItem("Server", newReadonlyEntry(asset.Server)),
		widget.NewFormItem("Status", newReadonlyEntry(fmt.Sprintf("%d", asset.StatusCode))),
		widget.NewFormItem("Country", newReadonlyEntry(asset.CountryCode)),
		widget.NewFormItem("City", newReadonlyEntry(asset.City)),
		widget.NewFormItem("ISP", newReadonlyEntry(asset.ISP)),
		widget.NewFormItem("ASN", newReadonlyEntry(asset.ASN)),
		widget.NewFormItem("Org", newReadonlyEntry(asset.Org)),
		widget.NewFormItem("Source", newReadonlyEntry(asset.Source)),
	)

	// Create a scrollable container for the details
	content := container.NewScroll(container.NewPadded(form))
	content.SetMinSize(fyne.NewSize(600, 500))

	d := dialog.NewCustom("资产详情", "关闭", content, window)
	d.Resize(fyne.NewSize(620, 550))
	d.Show()
}

func newReadonlyEntry(text string) *widget.Entry {
	e := widget.NewEntry()
	e.SetText(text)
	e.Disable() // 只读, 但允许复制
	return e
}

func showHelpDialog(window fyne.Window) {
	helpText := `UniMap UQL 查询语法帮助

1. 基础语法
   key="value"

2. 示例
   - 查询使用了 nginx 的服务: app="nginx"
   - 查询位于中国的服务: country="CN"
   - 组合查询: app="nginx" && country="CN"
   - 排除: port!="80"

3. 支持的字段
   ip, port, protocol, app, title, body, header, server, 
   status_code, domain, country, city, org, isp

4. 操作符
   =, !=, IN, && (AND), || (OR), ()

更多详情请参考项目目录下的 UQL_GUIDE.md`

	entry := widget.NewMultiLineEntry()
	entry.SetText(helpText)
	entry.Wrapping = fyne.TextWrapWord
	entry.Disable() // 只读

	content := container.NewScroll(entry)
	content.SetMinSize(fyne.NewSize(500, 300))

	dialog.ShowCustom("UQL 语法帮助", "关闭", content, window)
}

// 帮助函数：从配置注册引擎
func registerEngines(svc *service.UnifiedService, cfg *config.Config) {
	if cfg.Engines.Fofa.Enabled && cfg.Engines.Fofa.APIKey != "" {
		svc.RegisterAdapter(adapter.NewFofaAdapter(
			cfg.Engines.Fofa.BaseURL,
			cfg.Engines.Fofa.APIKey,
			cfg.Engines.Fofa.Email,
			cfg.Engines.Fofa.QPS,
			time.Duration(cfg.Engines.Fofa.Timeout)*time.Second,
		))
	}
	if cfg.Engines.Hunter.Enabled && cfg.Engines.Hunter.APIKey != "" {
		svc.RegisterAdapter(adapter.NewHunterAdapter(
			cfg.Engines.Hunter.BaseURL,
			cfg.Engines.Hunter.APIKey,
			cfg.Engines.Hunter.QPS,
			time.Duration(cfg.Engines.Hunter.Timeout)*time.Second,
		))
	}
	if cfg.Engines.Zoomeye.Enabled && cfg.Engines.Zoomeye.APIKey != "" {
		svc.RegisterAdapter(adapter.NewZoomEyeAdapter(
			cfg.Engines.Zoomeye.BaseURL,
			cfg.Engines.Zoomeye.APIKey,
			cfg.Engines.Zoomeye.QPS,
			time.Duration(cfg.Engines.Zoomeye.Timeout)*time.Second,
		))
	}
	if cfg.Engines.Quake.Enabled && cfg.Engines.Quake.APIKey != "" {
		svc.RegisterAdapter(adapter.NewQuakeAdapter(
			cfg.Engines.Quake.BaseURL,
			cfg.Engines.Quake.APIKey,
			cfg.Engines.Quake.QPS,
			time.Duration(cfg.Engines.Quake.Timeout)*time.Second,
		))
	}
}
