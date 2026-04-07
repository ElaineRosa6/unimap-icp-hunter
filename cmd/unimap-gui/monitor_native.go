//go:build gui
// +build gui

package main

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/unimap-icp-hunter/project/internal/screenshot"
	"github.com/unimap-icp-hunter/project/internal/tamper"
)

type monitorTarget struct {
	InputURL         string
	NormalizedURL    string
	FormatValid      bool
	Reachable        bool
	StatusCode       int
	ReasonType       string
	Reason           string
	BaselineExists   bool
	BaselineStatus   string
	TamperStatus     string
	Tampered         bool
	TamperedSegments []string
	Changes          []tamper.SegmentChange
	LastCheckedAt    int64
	ScreenshotPath   string
	ScreenshotError  string
}

type historyURLItem struct {
	URL         string
	HasBaseline bool
	RecordCount int
	LastCheckAt int64
}

type screenshotBatchItem struct {
	Name      string
	Path      string
	FileCount int
	UpdatedAt time.Time
}

type screenshotFileItem struct {
	Name       string
	Path       string
	PreviewURL string
	Size       int64
	UpdatedAt  time.Time
}

func createMonitorTab(window fyne.Window, state *AppState) fyne.CanvasObject {
	var (
		targets         []monitorTarget
		baselines       []string
		selectedTarget  = -1
		selectedBaseURL = -1
	)

	urlEntry := widget.NewMultiLineEntry()
	urlEntry.SetMinRowsVisible(8)
	urlEntry.SetPlaceHolder("每行一个 URL，支持不带协议的域名或主机:端口")

	concurrencyEntry := widget.NewEntry()
	concurrencyEntry.SetText("5")
	concurrencyEntry.SetPlaceHolder("5")

	statusLabel := widget.NewLabel("就绪")
	summaryLabel := widget.NewLabel("总数 0 | 格式合法 0 | 可达 0 | 不可达 0")

	detailEntry := widget.NewMultiLineEntry()
	detailEntry.Disable()
	detailEntry.SetMinRowsVisible(18)
	baselineDetail := widget.NewMultiLineEntry()
	baselineDetail.Disable()
	baselineDetail.SetMinRowsVisible(7)

	refreshDetails := func() {
		if selectedTarget < 0 || selectedTarget >= len(targets) {
			detailEntry.SetText("选择左侧 URL 查看探活、基线、篡改和截图详情。")
		} else {
			detailEntry.SetText(formatMonitorDetail(targets[selectedTarget]))
		}
		if selectedBaseURL < 0 || selectedBaseURL >= len(baselines) {
			baselineDetail.SetText("选择基线 URL 查看管理信息。")
		} else {
			baselineURL := baselines[selectedBaseURL]
			records, _ := state.TamperStorage.LoadCheckRecords(baselineURL, 5)
			baselineDetail.SetText(formatBaselineDetail(state, baselineURL, records))
		}
	}

	targetList := widget.NewList(
		func() int { return len(targets) },
		func() fyne.CanvasObject {
			primary := widget.NewLabel("")
			primary.TextStyle = fyne.TextStyle{Bold: true}
			secondary := widget.NewLabel("")
			secondary.Wrapping = fyne.TextWrapWord
			return container.NewVBox(primary, secondary)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			item := targets[id]
			box := obj.(*fyne.Container)
			box.Objects[0].(*widget.Label).SetText(monitorTargetTitle(item))
			box.Objects[1].(*widget.Label).SetText(monitorTargetSubtitle(item))
		},
	)
	targetList.OnSelected = func(id widget.ListItemID) {
		selectedTarget = id
		refreshDetails()
	}

	baselineList := widget.NewList(
		func() int { return len(baselines) },
		func() fyne.CanvasObject {
			primary := widget.NewLabel("")
			primary.TextStyle = fyne.TextStyle{Bold: true}
			secondary := widget.NewLabel("")
			return container.NewVBox(primary, secondary)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			baselineURL := baselines[id]
			box := obj.(*fyne.Container)
			box.Objects[0].(*widget.Label).SetText(baselineURL)
			box.Objects[1].(*widget.Label).SetText(baselineMetaText(state, baselineURL))
		},
	)
	baselineList.OnSelected = func(id widget.ListItemID) {
		selectedBaseURL = id
		refreshDetails()
	}

	refreshBaselines := func() {
		list, source, err := listBaselinesPreferAPI(context.Background(), state)
		if err != nil {
			statusLabel.SetText("读取基线失败: " + err.Error())
			return
		}
		baselines = list
		syncBaselineFlags(targets, baselines)
		if selectedBaseURL >= len(baselines) {
			selectedBaseURL = -1
		}
		baselineList.Refresh()
		targetList.Refresh()
		refreshDetails()
		statusLabel.SetText(fmt.Sprintf("已加载 %d 条基线（来源: %s）", len(baselines), source))
	}

	updateSummary := func() {
		total := len(targets)
		valid := 0
		reachable := 0
		for _, item := range targets {
			if item.FormatValid {
				valid++
			}
			if item.Reachable {
				reachable++
			}
		}
		summaryLabel.SetText(fmt.Sprintf("总数 %d | 格式合法 %d | 可达 %d | 不可达 %d", total, valid, reachable, total-valid+valid-reachable))
	}

	setTargets := func(items []monitorTarget) {
		targets = items
		syncBaselineFlags(targets, baselines)
		if selectedTarget >= len(targets) {
			selectedTarget = -1
		}
		updateSummary()
		targetList.Refresh()
		refreshDetails()
	}

	parseConcurrency := func() int {
		value := strings.TrimSpace(concurrencyEntry.Text)
		if value == "" {
			return 5
		}
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed <= 0 {
			return 5
		}
		if parsed > 20 {
			return 20
		}
		return parsed
	}

	var actionButtons []*widget.Button
	setBusy := func(busy bool, text string) {
		if busy {
			statusLabel.SetText(text)
		} else if text != "" {
			statusLabel.SetText(text)
		}
		for _, button := range actionButtons {
			if button == nil {
				continue
			}
			if busy {
				button.Disable()
			} else {
				button.Enable()
			}
		}
	}

	runBatchAction := func(name string, handler func(context.Context, []monitorTarget, int) ([]monitorTarget, string, error)) {
		parsed := parseMonitorTargets(urlEntry.Text)
		if len(parsed) == 0 {
			dialog.ShowError(fmt.Errorf("请输入至少一个 URL"), window)
			return
		}
		concurrency := parseConcurrency()
		setBusy(true, name+"中...")
		go func() {
			ctx := context.Background()
			probed := probeMonitorTargets(ctx, parsed, concurrency)
			resultTargets, statusText, err := handler(ctx, probed, concurrency)
			if err != nil {
				setTargets(probed)
				refreshBaselines()
				setBusy(false, statusText)
				dialog.ShowError(err, window)
				return
			}
			setTargets(resultTargets)
			refreshBaselines()
			setBusy(false, statusText)
		}()
	}

	probeBtn := widget.NewButtonWithIcon("探活", theme.ViewRefreshIcon(), func() {
		runBatchAction("探活", func(_ context.Context, probed []monitorTarget, _ int) ([]monitorTarget, string, error) {
			return probed, summarizeProbeStatus(probed), nil
		})
	})

	baselineBtn := widget.NewButtonWithIcon("设置基线", theme.DocumentCreateIcon(), func() {
		runBatchAction("设置基线", func(ctx context.Context, probed []monitorTarget, concurrency int) ([]monitorTarget, string, error) {
			reachable := reachableURLs(probed)
			if len(reachable) == 0 {
				annotateUnreachableBaseline(probed)
				return probed, "无可达 URL 可设置基线", nil
			}
			results, source, err := runSetBaselinePreferAPI(ctx, state, reachable, concurrency)
			if err != nil {
				return probed, "基线设置失败", err
			}
			applyBaselineResults(probed, results)
			return probed, fmt.Sprintf("%s（来源: %s）", summarizeBaselineStatus(probed), source), nil
		})
	})

	tamperBtn := widget.NewButtonWithIcon("篡改检测", theme.WarningIcon(), func() {
		runBatchAction("篡改检测", func(ctx context.Context, probed []monitorTarget, concurrency int) ([]monitorTarget, string, error) {
			reachable := reachableURLs(probed)
			if len(reachable) == 0 {
				annotateUnreachableTamper(probed)
				return probed, "无可达 URL 可检测", nil
			}
			results, source, err := runTamperCheckPreferAPI(ctx, state, reachable, concurrency)
			if err != nil {
				return probed, "篡改检测失败", err
			}
			applyTamperResults(probed, results)
			return probed, fmt.Sprintf("%s（来源: %s）", summarizeTamperStatus(probed), source), nil
		})
	})

	screenshotBtn := widget.NewButtonWithIcon("批量截图", theme.DocumentIcon(), func() {
		if state.ScreenshotMgr == nil {
			dialog.ShowError(fmt.Errorf("截图功能未启用，请在配置中开启 screenshot.enabled 并配置 Chrome"), window)
			return
		}
		runBatchAction("批量截图", func(ctx context.Context, probed []monitorTarget, concurrency int) ([]monitorTarget, string, error) {
			reachable := reachableURLs(probed)
			if len(reachable) == 0 {
				annotateUnreachableScreenshot(probed)
				return probed, "无可达 URL 可截图", nil
			}
			batchID := time.Now().Format("20060102-150405")
			results, source, err := runBatchScreenshotPreferAPI(ctx, state, reachable, batchID, concurrency)
			if err != nil {
				return probed, "批量截图失败", err
			}
			applyScreenshotResults(probed, results)
			return probed, fmt.Sprintf("%s（来源: %s）", summarizeScreenshotStatus(probed), source), nil
		})
	})

	refreshBaselineBtn := widget.NewButtonWithIcon("刷新基线", theme.ViewRefreshIcon(), func() {
		refreshBaselines()
		statusLabel.SetText(fmt.Sprintf("已加载 %d 条基线", len(baselines)))
	})

	retryBtn := widget.NewButton("重试所选探活", func() {
		if selectedTarget < 0 || selectedTarget >= len(targets) {
			dialog.ShowInformation("提示", "请先选择一个 URL", window)
			return
		}
		item := targets[selectedTarget]
		setBusy(true, "重试探活中...")
		go func() {
			refreshed := probeMonitorTargets(context.Background(), []monitorTarget{item}, 1)
			if len(refreshed) == 1 {
				targets[selectedTarget] = refreshed[0]
				syncBaselineFlags(targets, baselines)
			}
			updateSummary()
			targetList.Refresh()
			refreshDetails()
			setBusy(false, "所选 URL 已完成重试探活")
		}()
	})

	openScreenshotBtn := widget.NewButton("打开所选截图", func() {
		if selectedTarget < 0 || selectedTarget >= len(targets) {
			dialog.ShowInformation("提示", "请先选择一个 URL", window)
			return
		}
		path := strings.TrimSpace(targets[selectedTarget].ScreenshotPath)
		if path == "" {
			dialog.ShowInformation("提示", "所选 URL 还没有截图结果", window)
			return
		}
		if err := openPathInSystem(path); err != nil {
			dialog.ShowError(err, window)
		}
	})

	copyURLBtn := widget.NewButton("复制所选 URL", func() {
		if selectedTarget < 0 || selectedTarget >= len(targets) {
			dialog.ShowInformation("提示", "请先选择一个 URL", window)
			return
		}
		window.Clipboard().SetContent(selectedMonitorURL(targets[selectedTarget]))
		statusLabel.SetText("已复制所选 URL")
	})

	appendBaselineBtn := widget.NewButton("导入所选基线", func() {
		if selectedBaseURL < 0 || selectedBaseURL >= len(baselines) {
			dialog.ShowInformation("提示", "请先选择一个基线 URL", window)
			return
		}
		baseURL := baselines[selectedBaseURL]
		text := strings.TrimSpace(urlEntry.Text)
		if text == "" {
			urlEntry.SetText(baseURL)
		} else {
			urlEntry.SetText(text + "\n" + baseURL)
		}
	})

	deleteBaselineBtn := widget.NewButton("删除所选基线", func() {
		if selectedBaseURL < 0 || selectedBaseURL >= len(baselines) {
			dialog.ShowInformation("提示", "请先选择一个基线 URL", window)
			return
		}
		baselineURL := baselines[selectedBaseURL]
		dialog.ShowConfirm("删除基线", "确认删除该 URL 的基线？", func(confirm bool) {
			if !confirm {
				return
			}
			source, err := runDeleteBaselinePreferAPI(context.Background(), state, baselineURL)
			if err != nil {
				dialog.ShowError(err, window)
				return
			}
			refreshBaselines()
			statusLabel.SetText("基线已删除（来源: " + source + "）")
		}, window)
	})

	actionButtons = []*widget.Button{probeBtn, baselineBtn, tamperBtn, screenshotBtn, refreshBaselineBtn, retryBtn, openScreenshotBtn, copyURLBtn, appendBaselineBtn, deleteBaselineBtn}

	controls := container.NewHBox(
		probeBtn,
		baselineBtn,
		tamperBtn,
		screenshotBtn,
		refreshBaselineBtn,
		layout.NewSpacer(),
		widget.NewLabel("并发:"),
		container.NewGridWrap(fyne.NewSize(70, concurrencyEntry.MinSize().Height), concurrencyEntry),
	)

	leftPane := container.NewBorder(
		container.NewVBox(
			widget.NewLabelWithStyle("监控 URL 输入", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			urlEntry,
			widget.NewSeparator(),
			controls,
			widget.NewSeparator(),
			summaryLabel,
		),
		nil,
		nil,
		nil,
		container.NewBorder(widget.NewLabelWithStyle("探活与检测结果", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), nil, nil, nil, targetList),
	)

	targetActions := container.NewHBox(copyURLBtn, retryBtn, openScreenshotBtn)
	baselineActions := container.NewHBox(appendBaselineBtn, deleteBaselineBtn)
	rightPane := container.NewVSplit(
		container.NewBorder(
			container.NewVBox(widget.NewLabelWithStyle("所选 URL 详情", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), targetActions, widget.NewSeparator()),
			nil,
			nil,
			nil,
			detailEntry,
		),
		container.NewBorder(
			container.NewVBox(widget.NewLabelWithStyle("基线管理", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), baselineActions, widget.NewSeparator()),
			nil,
			nil,
			nil,
			container.NewVSplit(baselineList, baselineDetail),
		),
	)
	rightPane.Offset = 0.58

	content := container.NewHSplit(leftPane, rightPane)
	content.Offset = 0.56

	refreshBaselines()
	refreshDetails()

	return container.NewBorder(
		container.NewVBox(
			container.NewHBox(widget.NewIcon(theme.ComputerIcon()), widget.NewLabelWithStyle("原生 URL 监控", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), layout.NewSpacer(), statusLabel),
			widget.NewSeparator(),
		),
		nil,
		nil,
		nil,
		content,
	)
}

func createHistoryTab(window fyne.Window, state *AppState) fyne.CanvasObject {
	var (
		urlItems       []historyURLItem
		records        []*tamper.CheckRecord
		recordsByURL   map[string][]*tamper.CheckRecord
		selectedURL    = -1
		selectedRecord = -1
	)
	recordsByURL = make(map[string][]*tamper.CheckRecord)

	statsLabel := widget.NewLabel("就绪")
	urlDetail := widget.NewMultiLineEntry()
	urlDetail.Disable()
	urlDetail.SetMinRowsVisible(6)
	recordDetail := widget.NewMultiLineEntry()
	recordDetail.Disable()
	recordDetail.SetMinRowsVisible(16)

	refreshDetail := func() {
		if selectedURL < 0 || selectedURL >= len(urlItems) {
			urlDetail.SetText("选择左侧 URL 查看统计信息。")
		} else {
			item := urlItems[selectedURL]
			stats, _ := state.TamperStorage.GetCheckStats(item.URL)
			urlDetail.SetText(formatHistoryURLDetail(item, stats))
		}
		if selectedRecord < 0 || selectedRecord >= len(records) {
			recordDetail.SetText("选择中间的检测记录查看变更详情。")
		} else {
			recordDetail.SetText(formatCheckRecordDetail(records[selectedRecord]))
		}
	}

	urlList := widget.NewList(
		func() int { return len(urlItems) },
		func() fyne.CanvasObject {
			primary := widget.NewLabel("")
			primary.TextStyle = fyne.TextStyle{Bold: true}
			secondary := widget.NewLabel("")
			return container.NewVBox(primary, secondary)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			item := urlItems[id]
			box := obj.(*fyne.Container)
			box.Objects[0].(*widget.Label).SetText(item.URL)
			box.Objects[1].(*widget.Label).SetText(fmt.Sprintf("记录 %d | 基线 %s | 最近 %s", item.RecordCount, yesNo(item.HasBaseline), formatTimestamp(item.LastCheckAt)))
		},
	)

	recordList := widget.NewList(
		func() int { return len(records) },
		func() fyne.CanvasObject {
			primary := widget.NewLabel("")
			primary.TextStyle = fyne.TextStyle{Bold: true}
			secondary := widget.NewLabel("")
			return container.NewVBox(primary, secondary)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			record := records[id]
			box := obj.(*fyne.Container)
			box.Objects[0].(*widget.Label).SetText(fmt.Sprintf("%s | %s", record.CheckType, formatTimestamp(record.Timestamp)))
			box.Objects[1].(*widget.Label).SetText(historyRecordSummary(record))
		},
	)

	loadRecords := func() {
		if selectedURL < 0 || selectedURL >= len(urlItems) {
			records = nil
			selectedRecord = -1
			recordList.Refresh()
			refreshDetail()
			return
		}
		records = recordsByURL[urlItems[selectedURL].URL]
		if selectedRecord >= len(records) {
			selectedRecord = -1
		}
		recordList.Refresh()
		refreshDetail()
	}

	refreshURLs := func() {
		recordsByURL = make(map[string][]*tamper.CheckRecord)
		baselineURLs, baselineSource, baselineErr := listBaselinesPreferAPI(context.Background(), state)
		if baselineErr != nil {
			statsLabel.SetText("读取基线失败: " + baselineErr.Error())
			return
		}
		baselineSet := make(map[string]bool, len(baselineURLs))
		for _, item := range baselineURLs {
			baselineSet[item] = true
		}

		byURL := make(map[string]*historyURLItem)
		source := "API"
		apiRecords, apiErr := listTamperHistoryViaAPI(context.Background(), 1000)
		if apiErr == nil {
			for _, item := range baselineURLs {
				if _, ok := byURL[item]; !ok {
					byURL[item] = &historyURLItem{URL: item, HasBaseline: true}
				}
			}
			for _, rec := range apiRecords {
				record := rec.toCheckRecord()
				if record == nil || strings.TrimSpace(record.URL) == "" {
					continue
				}
				recordsByURL[record.URL] = append(recordsByURL[record.URL], record)
				info, ok := byURL[record.URL]
				if !ok {
					info = &historyURLItem{URL: record.URL}
					byURL[record.URL] = info
				}
				info.RecordCount++
				if record.Timestamp > info.LastCheckAt {
					info.LastCheckAt = record.Timestamp
				}
				if baselineSet[record.URL] {
					info.HasBaseline = true
				}
			}
			for urlText := range recordsByURL {
				sort.Slice(recordsByURL[urlText], func(i, j int) bool {
					return recordsByURL[urlText][i].Timestamp > recordsByURL[urlText][j].Timestamp
				})
			}
		} else {
			source = "本地"
			allRecords, err := state.TamperStorage.ListAllCheckRecords()
			if err != nil {
				statsLabel.SetText("读取历史索引失败: " + err.Error())
				return
			}
			for _, item := range baselineURLs {
				if _, ok := byURL[item]; !ok {
					byURL[item] = &historyURLItem{URL: item, HasBaseline: true}
				}
			}
			for _, list := range allRecords {
				for _, record := range list {
					if strings.TrimSpace(record.URL) == "" {
						continue
					}
					recordsByURL[record.URL] = append(recordsByURL[record.URL], record)
					info, ok := byURL[record.URL]
					if !ok {
						info = &historyURLItem{URL: record.URL}
						byURL[record.URL] = info
					}
					info.RecordCount++
					if record.Timestamp > info.LastCheckAt {
						info.LastCheckAt = record.Timestamp
					}
					if baselineSet[record.URL] {
						info.HasBaseline = true
					}
				}
			}
		}

		urlItems = urlItems[:0]
		for _, item := range byURL {
			urlItems = append(urlItems, *item)
		}
		sort.Slice(urlItems, func(i, j int) bool {
			if urlItems[i].LastCheckAt == urlItems[j].LastCheckAt {
				return urlItems[i].URL < urlItems[j].URL
			}
			return urlItems[i].LastCheckAt > urlItems[j].LastCheckAt
		})

		if selectedURL >= len(urlItems) {
			selectedURL = -1
		}
		urlList.Refresh()
		loadRecords()
		statsLabel.SetText(fmt.Sprintf("已加载 %d 个监控目标（历史来源: %s，基线来源: %s）", len(urlItems), source, baselineSource))
	}

	urlList.OnSelected = func(id widget.ListItemID) {
		selectedURL = id
		loadRecords()
	}
	recordList.OnSelected = func(id widget.ListItemID) {
		selectedRecord = id
		refreshDetail()
	}

	refreshBtn := widget.NewButtonWithIcon("刷新历史", theme.ViewRefreshIcon(), func() {
		refreshURLs()
	})
	copyBtn := widget.NewButton("复制所选 URL", func() {
		if selectedURL < 0 || selectedURL >= len(urlItems) {
			dialog.ShowInformation("提示", "请先选择一个 URL", window)
			return
		}
		window.Clipboard().SetContent(urlItems[selectedURL].URL)
		statsLabel.SetText("已复制所选 URL")
	})
	deleteHistoryBtn := widget.NewButton("删除该 URL 历史", func() {
		if selectedURL < 0 || selectedURL >= len(urlItems) {
			dialog.ShowInformation("提示", "请先选择一个 URL", window)
			return
		}
		selected := urlItems[selectedURL]
		dialog.ShowConfirm("删除历史", "确认删除该 URL 的全部检测记录？", func(confirm bool) {
			if !confirm {
				return
			}
			source, err := runDeleteHistoryPreferAPI(context.Background(), state, selected.URL)
			if err != nil {
				dialog.ShowError(err, window)
				return
			}
			refreshURLs()
			statsLabel.SetText("历史已删除（来源: " + source + "）")
		}, window)
	})
	deleteBaselineBtn := widget.NewButton("删除该 URL 基线", func() {
		if selectedURL < 0 || selectedURL >= len(urlItems) {
			dialog.ShowInformation("提示", "请先选择一个 URL", window)
			return
		}
		selected := urlItems[selectedURL]
		source, err := runDeleteBaselinePreferAPI(context.Background(), state, selected.URL)
		if err != nil {
			dialog.ShowError(err, window)
			return
		}
		refreshURLs()
		statsLabel.SetText("基线已删除（来源: " + source + "）")
	})

	left := container.NewBorder(
		container.NewVBox(widget.NewLabelWithStyle("监控目标", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), widget.NewSeparator()),
		nil,
		nil,
		nil,
		urlList,
	)
	middle := container.NewBorder(
		container.NewVBox(widget.NewLabelWithStyle("检测记录", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), widget.NewSeparator()),
		nil,
		nil,
		nil,
		recordList,
	)
	right := container.NewVSplit(urlDetail, recordDetail)
	right.Offset = 0.33
	content := container.NewHSplit(container.NewHSplit(left, middle), right)
	content.Offset = 0.58
	content.Leading.(*container.Split).Offset = 0.42

	refreshURLs()
	refreshDetail()

	return container.NewBorder(
		container.NewVBox(
			container.NewHBox(widget.NewIcon(theme.HistoryIcon()), widget.NewLabelWithStyle("检测历史", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), layout.NewSpacer(), statsLabel),
			widget.NewSeparator(),
			container.NewHBox(refreshBtn, copyBtn, deleteHistoryBtn, deleteBaselineBtn),
			widget.NewSeparator(),
		),
		nil,
		nil,
		nil,
		content,
	)
}

func createScreenshotTab(window fyne.Window, state *AppState) fyne.CanvasObject {
	if state.ScreenshotMgr == nil {
		return container.NewCenter(widget.NewLabel("截图功能未启用。请在配置中开启 screenshot.enabled，并配置 Chrome 路径或远程调试地址。"))
	}

	var (
		batches       []screenshotBatchItem
		files         []screenshotFileItem
		selectedBatch = -1
		selectedFile  = -1
	)

	baseDir := state.ScreenshotMgr.GetScreenshotDirectory()
	statusLabel := widget.NewLabel("就绪")
	detailEntry := widget.NewMultiLineEntry()
	detailEntry.Disable()
	detailEntry.SetMinRowsVisible(18)

	refreshDetail := func() {
		if selectedFile >= 0 && selectedFile < len(files) {
			detailEntry.SetText(formatScreenshotFileDetail(files[selectedFile]))
			return
		}
		if selectedBatch >= 0 && selectedBatch < len(batches) {
			detailEntry.SetText(formatScreenshotBatchDetail(batches[selectedBatch]))
			return
		}
		detailEntry.SetText("选择左侧批次或中间文件查看详情。")
	}

	fileList := widget.NewList(
		func() int { return len(files) },
		func() fyne.CanvasObject {
			primary := widget.NewLabel("")
			primary.TextStyle = fyne.TextStyle{Bold: true}
			secondary := widget.NewLabel("")
			return container.NewVBox(primary, secondary)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			item := files[id]
			box := obj.(*fyne.Container)
			box.Objects[0].(*widget.Label).SetText(item.Name)
			box.Objects[1].(*widget.Label).SetText(fmt.Sprintf("%s | %s", formatFileSize(item.Size), item.UpdatedAt.Format("2006-01-02 15:04:05")))
		},
	)
	fileList.OnSelected = func(id widget.ListItemID) {
		selectedFile = id
		refreshDetail()
	}

	loadFiles := func() {
		files = nil
		selectedFile = -1
		if selectedBatch < 0 || selectedBatch >= len(batches) {
			fileList.Refresh()
			refreshDetail()
			return
		}
		batchName := batches[selectedBatch].Name
		apiFiles, source, err := listScreenshotBatchFilesPreferAPI(context.Background(), batchName, baseDir)
		if err != nil {
			statusLabel.SetText("读取截图批次失败: " + err.Error())
			return
		}
		files = apiFiles
		fileList.Refresh()
		refreshDetail()
		statusLabel.SetText(fmt.Sprintf("已加载批次 %s 的 %d 个文件（来源: %s）", batchName, len(files), source))
	}

	batchList := widget.NewList(
		func() int { return len(batches) },
		func() fyne.CanvasObject {
			primary := widget.NewLabel("")
			primary.TextStyle = fyne.TextStyle{Bold: true}
			secondary := widget.NewLabel("")
			return container.NewVBox(primary, secondary)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			item := batches[id]
			box := obj.(*fyne.Container)
			box.Objects[0].(*widget.Label).SetText(item.Name)
			box.Objects[1].(*widget.Label).SetText(fmt.Sprintf("文件 %d | %s", item.FileCount, item.UpdatedAt.Format("2006-01-02 15:04:05")))
		},
	)
	batchList.OnSelected = func(id widget.ListItemID) {
		selectedBatch = id
		loadFiles()
	}

	refreshBatches := func() {
		batches = nil
		apiBatches, source, err := listScreenshotBatchesPreferAPI(context.Background(), baseDir)
		if err != nil {
			statusLabel.SetText("读取截图目录失败: " + err.Error())
			return
		}
		batches = apiBatches
		if selectedBatch >= len(batches) {
			selectedBatch = -1
		}
		batchList.Refresh()
		loadFiles()
		statusLabel.SetText(fmt.Sprintf("已加载 %d 个截图批次（来源: %s）", len(batches), source))
	}

	refreshBtn := widget.NewButtonWithIcon("刷新截图", theme.ViewRefreshIcon(), func() {
		refreshBatches()
	})
	openRootBtn := widget.NewButton("打开根目录", func() {
		if err := openPathInSystem(baseDir); err != nil {
			dialog.ShowError(err, window)
		}
	})
	openBatchBtn := widget.NewButton("打开所选批次", func() {
		if selectedBatch < 0 || selectedBatch >= len(batches) {
			dialog.ShowInformation("提示", "请先选择一个批次", window)
			return
		}
		if err := openPathInSystem(batches[selectedBatch].Path); err != nil {
			dialog.ShowError(err, window)
		}
	})
	openFileBtn := widget.NewButton("打开所选文件", func() {
		if selectedFile < 0 || selectedFile >= len(files) {
			dialog.ShowInformation("提示", "请先选择一个截图文件", window)
			return
		}
		if err := openPathInSystem(files[selectedFile].Path); err != nil {
			dialog.ShowError(err, window)
		}
	})
	deleteFileBtn := widget.NewButton("删除所选文件", func() {
		if selectedBatch < 0 || selectedBatch >= len(batches) || selectedFile < 0 || selectedFile >= len(files) {
			dialog.ShowInformation("提示", "请先选择批次和截图文件", window)
			return
		}
		batchName := batches[selectedBatch].Name
		fileName := files[selectedFile].Name
		dialog.ShowConfirm("删除截图文件", "确认删除所选截图文件？", func(confirm bool) {
			if !confirm {
				return
			}
			source, err := runDeleteScreenshotFilePreferAPI(context.Background(), batchName, fileName, files[selectedFile].Path)
			if err != nil {
				dialog.ShowError(err, window)
				return
			}
			loadFiles()
			statusLabel.SetText("截图文件已删除（来源: " + source + "）")
		}, window)
	})
	deleteBatchBtn := widget.NewButton("删除所选批次", func() {
		if selectedBatch < 0 || selectedBatch >= len(batches) {
			dialog.ShowInformation("提示", "请先选择一个批次", window)
			return
		}
		batchName := batches[selectedBatch].Name
		batchPath := batches[selectedBatch].Path
		dialog.ShowConfirm("删除截图批次", "确认删除该批次及其全部截图文件？", func(confirm bool) {
			if !confirm {
				return
			}
			source, err := runDeleteScreenshotBatchPreferAPI(context.Background(), batchName, batchPath)
			if err != nil {
				dialog.ShowError(err, window)
				return
			}
			refreshBatches()
			statusLabel.SetText("截图批次已删除（来源: " + source + "）")
		}, window)
	})

	content := container.NewHSplit(
		container.NewBorder(widget.NewLabelWithStyle("截图批次", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), nil, nil, nil, batchList),
		container.NewHSplit(
			container.NewBorder(widget.NewLabelWithStyle("批次文件", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), nil, nil, nil, fileList),
			container.NewBorder(widget.NewLabelWithStyle("详情", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), nil, nil, nil, detailEntry),
		),
	)
	content.Offset = 0.34
	content.Trailing.(*container.Split).Offset = 0.45

	refreshBatches()
	refreshDetail()

	return container.NewBorder(
		container.NewVBox(
			container.NewHBox(widget.NewIcon(theme.FolderIcon()), widget.NewLabelWithStyle("截图管理", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), layout.NewSpacer(), statusLabel),
			widget.NewLabel("截图根目录: "+baseDir),
			widget.NewSeparator(),
			container.NewHBox(refreshBtn, openRootBtn, openBatchBtn, openFileBtn, deleteFileBtn, deleteBatchBtn),
			widget.NewSeparator(),
		),
		nil,
		nil,
		nil,
		content,
	)
}

func parseMonitorTargets(raw string) []monitorTarget {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	seen := make(map[string]bool)
	items := make([]monitorTarget, 0, len(lines))
	for _, line := range lines {
		value := strings.TrimSpace(line)
		if value == "" {
			continue
		}
		if seen[value] {
			continue
		}
		seen[value] = true
		items = append(items, monitorTarget{InputURL: value})
	}
	return items
}

func probeMonitorTargets(ctx context.Context, items []monitorTarget, concurrency int) []monitorTarget {
	if concurrency <= 0 {
		concurrency = 5
	}
	results := make([]monitorTarget, len(items))
	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrency)
	for index, item := range items {
		wg.Add(1)
		go func(i int, target monitorTarget) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			result := target
			normalized, err := normalizeMonitorURL(target.InputURL)
			if err != nil {
				result.FormatValid = false
				result.ReasonType = "invalid_format"
				result.Reason = err.Error()
				results[i] = result
				return
			}
			result.FormatValid = true
			result.NormalizedURL = normalized
			reachable, statusCode, reasonType, reason := probeURLReachability(ctx, normalized)
			result.Reachable = reachable
			result.StatusCode = statusCode
			result.ReasonType = reasonType
			result.Reason = reason
			if reachable {
				result.BaselineStatus = "可设置基线"
			}
			results[i] = result
		}(index, item)
	}
	wg.Wait()
	return results
}

func normalizeMonitorURL(rawURL string) (string, error) {
	urlText := strings.TrimSpace(rawURL)
	if urlText == "" {
		return "", fmt.Errorf("empty URL")
	}
	if !strings.HasPrefix(urlText, "http://") && !strings.HasPrefix(urlText, "https://") {
		urlText = "https://" + urlText
	}
	parsed, err := url.ParseRequestURI(urlText)
	if err != nil {
		return "", err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("unsupported scheme: %s", parsed.Scheme)
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return "", fmt.Errorf("missing host")
	}
	return parsed.String(), nil
}

type guiTamperAPIResponse struct {
	Success bool                       `json:"success"`
	Mode    string                     `json:"mode"`
	Summary map[string]int             `json:"summary"`
	Results []tamper.TamperCheckResult `json:"results"`
}

type guiBaselineAPIResponse struct {
	Success bool                    `json:"success"`
	Summary map[string]int          `json:"summary"`
	Results []tamper.PageHashResult `json:"results"`
}

type guiBaselinesListResponse struct {
	Success bool     `json:"success"`
	URLs    []string `json:"urls"`
}

type guiTamperHistoryResponse struct {
	Success bool                     `json:"success"`
	Records []guiTamperHistoryRecord `json:"records"`
}

type guiTamperHistoryRecord struct {
	URL              string   `json:"url"`
	CheckType        string   `json:"check_type"`
	Tampered         bool     `json:"tampered"`
	TamperedSegments []string `json:"tampered_segments"`
	Timestamp        int64    `json:"timestamp"`
	CurrentFullHash  string   `json:"current_full_hash"`
	BaselineFullHash string   `json:"baseline_full_hash"`
}

func (r guiTamperHistoryRecord) toCheckRecord() *tamper.CheckRecord {
	rec := &tamper.CheckRecord{
		URL:              strings.TrimSpace(r.URL),
		CheckType:        strings.TrimSpace(r.CheckType),
		Tampered:         r.Tampered,
		TamperedSegments: r.TamperedSegments,
		Timestamp:        r.Timestamp,
	}
	if strings.TrimSpace(r.CurrentFullHash) != "" {
		rec.CurrentHash = &tamper.PageHashResult{FullHash: r.CurrentFullHash}
	}
	if strings.TrimSpace(r.BaselineFullHash) != "" {
		rec.BaselineHash = &tamper.PageHashResult{FullHash: r.BaselineFullHash}
	}
	if rec.URL == "" {
		return nil
	}
	if rec.CheckType == "" {
		rec.CheckType = "check"
	}
	return rec
}

type guiScreenshotBatchesResponse struct {
	Success bool                    `json:"success"`
	Batches []guiScreenshotBatchDTO `json:"batches"`
}

type guiScreenshotBatchDTO struct {
	Name      string `json:"name"`
	FileCount int    `json:"file_count"`
	UpdatedAt int64  `json:"updated_at"`
}

type guiScreenshotFilesResponse struct {
	Success bool                   `json:"success"`
	Files   []guiScreenshotFileDTO `json:"files"`
}

type guiScreenshotFileDTO struct {
	Name       string `json:"name"`
	Size       int64  `json:"size"`
	UpdatedAt  int64  `json:"updated_at"`
	PreviewURL string `json:"preview_url"`
}

func resolveGUIAPIBase() string {
	if raw := strings.TrimSpace(os.Getenv("UNIMAP_API_BASE")); raw != "" {
		return strings.TrimRight(raw, "/")
	}
	return "http://127.0.0.1:8448"
}

func runTamperCheckViaAPI(ctx context.Context, urls []string, concurrency int) ([]tamper.TamperCheckResult, error) {
	base := resolveGUIAPIBase()
	payload := map[string]interface{}{
		"urls":        urls,
		"concurrency": concurrency,
		"mode":        tamper.DetectionModeRelaxed,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/tamper/check", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var decoded guiTamperAPIResponse
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		return nil, err
	}
	if !decoded.Success {
		return nil, fmt.Errorf("tamper api returned unsuccessful response")
	}
	return decoded.Results, nil
}

func runTamperCheckPreferAPI(ctx context.Context, state *AppState, urls []string, concurrency int) ([]tamper.TamperCheckResult, string, error) {
	apiResults, apiErr := runTamperCheckViaAPI(ctx, urls, concurrency)
	if apiErr == nil {
		return apiResults, "API", nil
	}

	localResults, localErr := state.Detector.BatchCheckTampering(ctx, urls, concurrency)
	if localErr == nil {
		return localResults, "本地", nil
	}

	return nil, "", fmt.Errorf("API 模式失败: %v; 本地模式失败: %v", apiErr, localErr)
}

func runSetBaselineViaAPI(ctx context.Context, urls []string, concurrency int) ([]tamper.PageHashResult, error) {
	base := resolveGUIAPIBase()
	payload := map[string]interface{}{
		"urls":        urls,
		"concurrency": concurrency,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/tamper/baseline", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var decoded guiBaselineAPIResponse
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		return nil, err
	}
	if !decoded.Success {
		return nil, fmt.Errorf("baseline api returned unsuccessful response")
	}
	return decoded.Results, nil
}

func runSetBaselinePreferAPI(ctx context.Context, state *AppState, urls []string, concurrency int) ([]tamper.PageHashResult, string, error) {
	apiResults, apiErr := runSetBaselineViaAPI(ctx, urls, concurrency)
	if apiErr == nil {
		return apiResults, "API", nil
	}

	localResults, localErr := state.Detector.BatchSetBaseline(ctx, urls, concurrency)
	if localErr == nil {
		return localResults, "本地", nil
	}

	return nil, "", fmt.Errorf("API 模式失败: %v; 本地模式失败: %v", apiErr, localErr)
}

func listBaselinesViaAPI(ctx context.Context) ([]string, error) {
	base := resolveGUIAPIBase()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/tamper/baseline/list", nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var decoded guiBaselinesListResponse
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		return nil, err
	}
	if !decoded.Success {
		return nil, fmt.Errorf("baseline list api returned unsuccessful response")
	}
	return decoded.URLs, nil
}

func listBaselinesPreferAPI(ctx context.Context, state *AppState) ([]string, string, error) {
	apiURLs, apiErr := listBaselinesViaAPI(ctx)
	if apiErr == nil {
		return apiURLs, "API", nil
	}

	localURLs, localErr := state.Detector.ListBaselines()
	if localErr == nil {
		return localURLs, "本地", nil
	}

	return nil, "", fmt.Errorf("API 模式失败: %v; 本地模式失败: %v", apiErr, localErr)
}

func runDeleteBaselineViaAPI(ctx context.Context, targetURL string) error {
	base := resolveGUIAPIBase()
	endpoint := fmt.Sprintf("%s/api/tamper/baseline/delete?url=%s", base, url.QueryEscape(targetURL))
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return nil
}

func runDeleteBaselinePreferAPI(ctx context.Context, state *AppState, targetURL string) (string, error) {
	apiErr := runDeleteBaselineViaAPI(ctx, targetURL)
	if apiErr == nil {
		return "API", nil
	}

	localErr := state.Detector.DeleteBaseline(targetURL)
	if localErr == nil {
		return "本地", nil
	}

	return "", fmt.Errorf("API 模式失败: %v; 本地模式失败: %v", apiErr, localErr)
}

func listTamperHistoryViaAPI(ctx context.Context, limit int) ([]guiTamperHistoryRecord, error) {
	base := resolveGUIAPIBase()
	endpoint := fmt.Sprintf("%s/api/tamper/history?limit=%d", base, limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var decoded guiTamperHistoryResponse
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		return nil, err
	}
	if !decoded.Success {
		return nil, fmt.Errorf("history api returned unsuccessful response")
	}
	return decoded.Records, nil
}

func runDeleteHistoryViaAPI(ctx context.Context, targetURL string) error {
	base := resolveGUIAPIBase()
	endpoint := fmt.Sprintf("%s/api/tamper/history/delete?url=%s", base, url.QueryEscape(targetURL))
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return nil
}

func runDeleteHistoryPreferAPI(ctx context.Context, state *AppState, targetURL string) (string, error) {
	apiErr := runDeleteHistoryViaAPI(ctx, targetURL)
	if apiErr == nil {
		return "API", nil
	}

	localErr := state.TamperStorage.DeleteCheckRecords(targetURL)
	if localErr == nil {
		return "本地", nil
	}

	return "", fmt.Errorf("API 模式失败: %v; 本地模式失败: %v", apiErr, localErr)
}

func listScreenshotBatchesViaAPI(ctx context.Context, baseDir string) ([]screenshotBatchItem, error) {
	base := resolveGUIAPIBase()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/screenshot/batches", nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var decoded guiScreenshotBatchesResponse
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		return nil, err
	}
	if !decoded.Success {
		return nil, fmt.Errorf("screenshot batches api returned unsuccessful response")
	}

	items := make([]screenshotBatchItem, 0, len(decoded.Batches))
	for _, item := range decoded.Batches {
		items = append(items, screenshotBatchItem{
			Name:      item.Name,
			Path:      filepath.Join(baseDir, item.Name),
			FileCount: item.FileCount,
			UpdatedAt: time.Unix(item.UpdatedAt, 0),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	return items, nil
}

func listScreenshotBatchesPreferAPI(ctx context.Context, baseDir string) ([]screenshotBatchItem, string, error) {
	apiItems, apiErr := listScreenshotBatchesViaAPI(ctx, baseDir)
	if apiErr == nil {
		return apiItems, "API", nil
	}

	entries, localErr := os.ReadDir(baseDir)
	if localErr != nil {
		if os.IsNotExist(localErr) {
			return []screenshotBatchItem{}, "本地", nil
		}
		return nil, "", fmt.Errorf("API 模式失败: %v; 本地模式失败: %v", apiErr, localErr)
	}

	items := make([]screenshotBatchItem, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		fileCount := 0
		children, err := os.ReadDir(filepath.Join(baseDir, entry.Name()))
		if err == nil {
			for _, child := range children {
				if !child.IsDir() {
					fileCount++
				}
			}
		}
		items = append(items, screenshotBatchItem{
			Name:      entry.Name(),
			Path:      filepath.Join(baseDir, entry.Name()),
			FileCount: fileCount,
			UpdatedAt: info.ModTime(),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	return items, "本地", nil
}

func listScreenshotBatchFilesViaAPI(ctx context.Context, batchName, baseDir string) ([]screenshotFileItem, error) {
	base := resolveGUIAPIBase()
	endpoint := fmt.Sprintf("%s/api/screenshot/batches/files?batch=%s", base, url.QueryEscape(batchName))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var decoded guiScreenshotFilesResponse
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		return nil, err
	}
	if !decoded.Success {
		return nil, fmt.Errorf("screenshot files api returned unsuccessful response")
	}

	items := make([]screenshotFileItem, 0, len(decoded.Files))
	for _, item := range decoded.Files {
		items = append(items, screenshotFileItem{
			Name:       item.Name,
			Path:       filepath.Join(baseDir, batchName, item.Name),
			PreviewURL: item.PreviewURL,
			Size:       item.Size,
			UpdatedAt:  time.Unix(item.UpdatedAt, 0),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	return items, nil
}

func listScreenshotBatchFilesPreferAPI(ctx context.Context, batchName, baseDir string) ([]screenshotFileItem, string, error) {
	apiItems, apiErr := listScreenshotBatchFilesViaAPI(ctx, batchName, baseDir)
	if apiErr == nil {
		return apiItems, "API", nil
	}

	batchPath := filepath.Join(baseDir, batchName)
	entries, localErr := os.ReadDir(batchPath)
	if localErr != nil {
		return nil, "", fmt.Errorf("API 模式失败: %v; 本地模式失败: %v", apiErr, localErr)
	}

	items := make([]screenshotFileItem, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		items = append(items, screenshotFileItem{
			Name:      entry.Name(),
			Path:      filepath.Join(batchPath, entry.Name()),
			Size:      info.Size(),
			UpdatedAt: info.ModTime(),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	return items, "本地", nil
}

func runDeleteScreenshotBatchViaAPI(ctx context.Context, batchName string) error {
	base := resolveGUIAPIBase()
	endpoint := fmt.Sprintf("%s/api/screenshot/batches/delete?batch=%s", base, url.QueryEscape(batchName))
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return nil
}

func runDeleteScreenshotBatchPreferAPI(ctx context.Context, batchName, batchPath string) (string, error) {
	apiErr := runDeleteScreenshotBatchViaAPI(ctx, batchName)
	if apiErr == nil {
		return "API", nil
	}

	localErr := os.RemoveAll(batchPath)
	if localErr == nil {
		return "本地", nil
	}

	return "", fmt.Errorf("API 模式失败: %v; 本地模式失败: %v", apiErr, localErr)
}

func runDeleteScreenshotFileViaAPI(ctx context.Context, batchName, fileName string) error {
	base := resolveGUIAPIBase()
	endpoint := fmt.Sprintf("%s/api/screenshot/file/delete?batch=%s&file=%s", base, url.QueryEscape(batchName), url.QueryEscape(fileName))
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return nil
}

func runDeleteScreenshotFilePreferAPI(ctx context.Context, batchName, fileName, filePath string) (string, error) {
	apiErr := runDeleteScreenshotFileViaAPI(ctx, batchName, fileName)
	if apiErr == nil {
		return "API", nil
	}

	localErr := os.Remove(filePath)
	if localErr == nil {
		return "本地", nil
	}

	return "", fmt.Errorf("API 模式失败: %v; 本地模式失败: %v", apiErr, localErr)
}

func runBatchScreenshotViaAPI(ctx context.Context, urls []string, batchID string, concurrency int) ([]screenshot.BatchScreenshotResult, error) {
	base := resolveGUIAPIBase()
	payload := map[string]interface{}{
		"urls":        urls,
		"batch_id":    batchID,
		"concurrency": concurrency,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/screenshot/batch-urls", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 180 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var decoded struct {
		Results []screenshot.BatchScreenshotResult `json:"results"`
	}
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		return nil, err
	}
	return decoded.Results, nil
}

func runBatchScreenshotPreferAPI(ctx context.Context, state *AppState, urls []string, batchID string, concurrency int) ([]screenshot.BatchScreenshotResult, string, error) {
	apiResults, apiErr := runBatchScreenshotViaAPI(ctx, urls, batchID, concurrency)
	if apiErr == nil {
		return apiResults, "API", nil
	}

	localResults, localErr := state.ScreenshotMgr.CaptureBatchURLs(ctx, urls, batchID, concurrency)
	if localErr == nil {
		return localResults, "本地", nil
	}

	return nil, "", fmt.Errorf("API 模式失败: %v; 本地模式失败: %v", apiErr, localErr)
}

func probeURLReachability(ctx context.Context, targetURL string) (bool, int, string, string) {
	client := &http.Client{Timeout: 8 * time.Second}
	var headErr error

	headReq, err := http.NewRequestWithContext(ctx, http.MethodHead, targetURL, nil)
	if err != nil {
		errType, reason := classifyReachabilityError(err)
		return false, 0, errType, reason
	}

	headResp, err := client.Do(headReq)
	if err == nil {
		defer headResp.Body.Close()
		if headResp.StatusCode != http.StatusMethodNotAllowed {
			return true, headResp.StatusCode, "http_status", fmt.Sprintf("HTTP %d", headResp.StatusCode)
		}
	} else {
		headErr = err
	}

	getReq, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if reqErr != nil {
		if headErr != nil {
			errType, reason := classifyReachabilityError(headErr)
			return false, 0, errType, reason
		}
		errType, reason := classifyReachabilityError(reqErr)
		return false, 0, errType, reason
	}

	getResp, err := client.Do(getReq)
	if err != nil {
		if headErr != nil {
			errType, reason := classifyReachabilityError(headErr)
			return false, 0, errType, reason
		}
		errType, reason := classifyReachabilityError(err)
		return false, 0, errType, reason
	}
	defer getResp.Body.Close()
	return true, getResp.StatusCode, "http_status", fmt.Sprintf("HTTP %d", getResp.StatusCode)
}

func classifyReachabilityError(err error) (string, string) {
	if err == nil {
		return "unknown", "unknown error"
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return "dns", dnsErr.Error()
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "timeout", netErr.Error()
	}
	var certErr x509.UnknownAuthorityError
	if errors.As(err, &certErr) {
		return "tls", err.Error()
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(msg, "tls") || strings.Contains(msg, "certificate") || strings.Contains(msg, "ssl"):
		return "tls", err.Error()
	case strings.Contains(msg, "connection refused") || strings.Contains(msg, "connrefused"):
		return "connection_refused", err.Error()
	case strings.Contains(msg, "timeout") || strings.Contains(msg, "timed out"):
		return "timeout", err.Error()
	case strings.Contains(msg, "name not resolved") || strings.Contains(msg, "no such host") || strings.Contains(msg, "dns"):
		return "dns", err.Error()
	default:
		return "network", err.Error()
	}
}

func syncBaselineFlags(items []monitorTarget, baselines []string) {
	baselineSet := make(map[string]bool, len(baselines))
	for _, item := range baselines {
		baselineSet[item] = true
	}
	for index := range items {
		urlText := selectedMonitorURL(items[index])
		items[index].BaselineExists = baselineSet[urlText]
		if items[index].BaselineExists && items[index].BaselineStatus == "" {
			items[index].BaselineStatus = "已有基线"
		}
		if !items[index].BaselineExists && items[index].BaselineStatus == "已有基线" {
			items[index].BaselineStatus = ""
		}
	}
}

func reachableURLs(items []monitorTarget) []string {
	urls := make([]string, 0, len(items))
	for _, item := range items {
		if item.Reachable {
			urls = append(urls, selectedMonitorURL(item))
		}
	}
	return urls
}

func annotateUnreachableBaseline(items []monitorTarget) {
	for index := range items {
		if !items[index].Reachable {
			items[index].BaselineStatus = "目标不可达"
		}
	}
}

func annotateUnreachableTamper(items []monitorTarget) {
	for index := range items {
		if !items[index].Reachable {
			items[index].TamperStatus = "目标不可达"
		}
	}
}

func annotateUnreachableScreenshot(items []monitorTarget) {
	for index := range items {
		if !items[index].Reachable {
			items[index].ScreenshotError = "目标不可达"
		}
	}
}

func applyBaselineResults(items []monitorTarget, results []tamper.PageHashResult) {
	resultMap := make(map[string]tamper.PageHashResult, len(results))
	for _, result := range results {
		resultMap[result.URL] = result
	}
	for index := range items {
		if !items[index].Reachable {
			items[index].BaselineStatus = "目标不可达"
			continue
		}
		result, ok := resultMap[selectedMonitorURL(items[index])]
		if !ok {
			items[index].BaselineStatus = "未返回结果"
			continue
		}
		if strings.HasPrefix(result.Status, "error") {
			items[index].BaselineStatus = result.Status
			continue
		}
		items[index].BaselineExists = true
		items[index].BaselineStatus = "基线已保存"
		items[index].Reason = fmt.Sprintf("标题: %s", result.Title)
	}
}

func applyTamperResults(items []monitorTarget, results []tamper.TamperCheckResult) {
	resultMap := make(map[string]tamper.TamperCheckResult, len(results))
	for _, result := range results {
		resultMap[result.URL] = result
	}
	for index := range items {
		if !items[index].Reachable {
			items[index].TamperStatus = "目标不可达"
			continue
		}
		result, ok := resultMap[selectedMonitorURL(items[index])]
		if !ok {
			items[index].TamperStatus = "未返回结果"
			continue
		}
		items[index].Tampered = result.Tampered
		items[index].TamperedSegments = result.TamperedSegments
		items[index].Changes = result.Changes
		items[index].LastCheckedAt = result.Timestamp
		switch result.Status {
		case "no_baseline":
			items[index].TamperStatus = "无可比对基线"
		case "unreachable":
			items[index].TamperStatus = fmt.Sprintf("不可达: %s", strings.TrimSpace(result.ErrorMessage))
		case "tampered":
			items[index].TamperStatus = "检测到页面变化"
		case "normal":
			items[index].TamperStatus = "页面正常"
		default:
			items[index].TamperStatus = result.Status
		}
	}
}

func applyScreenshotResults(items []monitorTarget, results []screenshot.BatchScreenshotResult) {
	resultMap := make(map[string]screenshot.BatchScreenshotResult, len(results))
	for _, result := range results {
		resultMap[result.URL] = result
	}
	for index := range items {
		if !items[index].Reachable {
			items[index].ScreenshotError = "目标不可达"
			continue
		}
		result, ok := resultMap[selectedMonitorURL(items[index])]
		if !ok {
			items[index].ScreenshotError = "未返回截图结果"
			continue
		}
		if result.Success {
			items[index].ScreenshotPath = result.FilePath
			items[index].ScreenshotError = ""
			continue
		}
		items[index].ScreenshotPath = ""
		items[index].ScreenshotError = result.Error
	}
}

func summarizeProbeStatus(items []monitorTarget) string {
	reachable := 0
	invalid := 0
	for _, item := range items {
		if item.Reachable {
			reachable++
		}
		if !item.FormatValid {
			invalid++
		}
	}
	return fmt.Sprintf("探活完成: 可达 %d / %d，格式非法 %d", reachable, len(items), invalid)
}

func summarizeBaselineStatus(items []monitorTarget) string {
	saved := 0
	failed := 0
	for _, item := range items {
		switch {
		case item.BaselineStatus == "基线已保存":
			saved++
		case item.BaselineStatus != "" && item.BaselineStatus != "目标不可达":
			failed++
		}
	}
	return fmt.Sprintf("基线设置完成: 成功 %d，失败 %d", saved, failed)
}

func summarizeTamperStatus(items []monitorTarget) string {
	tampered := 0
	normal := 0
	noBaseline := 0
	for _, item := range items {
		switch item.TamperStatus {
		case "检测到页面变化":
			tampered++
		case "页面正常":
			normal++
		case "无可比对基线":
			noBaseline++
		}
	}
	return fmt.Sprintf("篡改检测完成: 正常 %d，变化 %d，无基线 %d", normal, tampered, noBaseline)
}

func summarizeScreenshotStatus(items []monitorTarget) string {
	success := 0
	for _, item := range items {
		if strings.TrimSpace(item.ScreenshotPath) != "" {
			success++
		}
	}
	return fmt.Sprintf("批量截图完成: 成功 %d / %d", success, len(items))
}

func selectedMonitorURL(item monitorTarget) string {
	if strings.TrimSpace(item.NormalizedURL) != "" {
		return item.NormalizedURL
	}
	return strings.TrimSpace(item.InputURL)
}

func monitorTargetTitle(item monitorTarget) string {
	return selectedMonitorURL(item)
}

func monitorTargetSubtitle(item monitorTarget) string {
	probeStatus := "格式非法"
	if item.FormatValid {
		if item.Reachable {
			probeStatus = fmt.Sprintf("可达 (%d)", item.StatusCode)
		} else {
			probeStatus = "不可达"
		}
	}
	baselineStatus := item.BaselineStatus
	if baselineStatus == "" {
		if item.BaselineExists {
			baselineStatus = "已有基线"
		} else {
			baselineStatus = "未设置"
		}
	}
	tamperStatus := item.TamperStatus
	if tamperStatus == "" {
		tamperStatus = "未检测"
	}
	return fmt.Sprintf("探活: %s | 基线: %s | 篡改: %s", probeStatus, baselineStatus, tamperStatus)
}

func formatMonitorDetail(item monitorTarget) string {
	lines := []string{
		"URL: " + selectedMonitorURL(item),
		"探活状态: " + monitorTargetSubtitle(item),
	}
	if item.ReasonType != "" {
		lines = append(lines, "失败类型: "+formatReasonType(item.ReasonType))
	}
	if item.Reason != "" {
		lines = append(lines, "说明: "+item.Reason)
	}
	if item.LastCheckedAt > 0 {
		lines = append(lines, "最近检测: "+formatTimestamp(item.LastCheckedAt))
	}
	if len(item.TamperedSegments) > 0 {
		lines = append(lines, "变更段落: "+strings.Join(item.TamperedSegments, ", "))
	}
	if len(item.Changes) > 0 {
		lines = append(lines, "变更详情:")
		for _, change := range item.Changes {
			lines = append(lines, fmt.Sprintf("- %s | %s | %s", change.Segment, change.ChangeType, change.Description))
		}
	}
	if item.ScreenshotPath != "" {
		lines = append(lines, "截图文件: "+item.ScreenshotPath)
	}
	if item.ScreenshotError != "" {
		lines = append(lines, "截图状态: "+item.ScreenshotError)
	}
	return strings.Join(lines, "\n")
}

func formatBaselineDetail(state *AppState, baselineURL string, records []*tamper.CheckRecord) string {
	stats, _ := state.TamperStorage.GetCheckStats(baselineURL)
	lines := []string{"URL: " + baselineURL}
	if stats != nil {
		lines = append(lines, fmt.Sprintf("检测总数: %v", stats["total_checks"]))
		lines = append(lines, fmt.Sprintf("篡改次数: %v", stats["tampered_count"]))
		lines = append(lines, fmt.Sprintf("安全次数: %v", stats["safe_count"]))
	}
	if len(records) == 0 {
		lines = append(lines, "最近记录: 暂无")
		return strings.Join(lines, "\n")
	}
	lines = append(lines, "最近记录:")
	for _, record := range records {
		lines = append(lines, fmt.Sprintf("- %s | %s | tampered=%s", formatTimestamp(record.Timestamp), record.CheckType, yesNo(record.Tampered)))
	}
	return strings.Join(lines, "\n")
}

func baselineMetaText(state *AppState, baselineURL string) string {
	stats, err := state.TamperStorage.GetCheckStats(baselineURL)
	if err != nil || stats == nil {
		return "已保存基线"
	}
	return fmt.Sprintf("检测 %v 次 | 最近 %s", stats["total_checks"], formatAnyTimestamp(stats["last_check_time"]))
}

func formatHistoryURLDetail(item historyURLItem, stats map[string]interface{}) string {
	lines := []string{
		"URL: " + item.URL,
		"是否有基线: " + yesNo(item.HasBaseline),
		fmt.Sprintf("历史记录数: %d", item.RecordCount),
		"最近检测: " + formatTimestamp(item.LastCheckAt),
	}
	if stats != nil {
		lines = append(lines,
			fmt.Sprintf("篡改次数: %v", stats["tampered_count"]),
			fmt.Sprintf("安全次数: %v", stats["safe_count"]),
			fmt.Sprintf("首次检测次数: %v", stats["first_check_count"]),
		)
	}
	return strings.Join(lines, "\n")
}

func historyRecordSummary(record *tamper.CheckRecord) string {
	parts := []string{"tampered=" + yesNo(record.Tampered)}
	if len(record.TamperedSegments) > 0 {
		parts = append(parts, "segments="+strings.Join(record.TamperedSegments, ","))
	}
	return strings.Join(parts, " | ")
}

func formatCheckRecordDetail(record *tamper.CheckRecord) string {
	lines := []string{
		"URL: " + record.URL,
		"检测类型: " + record.CheckType,
		"时间: " + formatTimestamp(record.Timestamp),
		"是否篡改: " + yesNo(record.Tampered),
	}
	if record.CurrentHash != nil {
		lines = append(lines, "当前标题: "+record.CurrentHash.Title)
		lines = append(lines, "当前哈希: "+record.CurrentHash.FullHash)
	}
	if record.BaselineHash != nil {
		lines = append(lines, "基线哈希: "+record.BaselineHash.FullHash)
	}
	if len(record.TamperedSegments) > 0 {
		lines = append(lines, "变更段落: "+strings.Join(record.TamperedSegments, ", "))
	}
	if len(record.Changes) > 0 {
		lines = append(lines, "变更详情:")
		for _, change := range record.Changes {
			lines = append(lines, fmt.Sprintf("- %s | %s | %s", change.Segment, change.ChangeType, change.Description))
		}
	}
	return strings.Join(lines, "\n")
}

func formatScreenshotBatchDetail(item screenshotBatchItem) string {
	return strings.Join([]string{
		"批次: " + item.Name,
		"目录: " + item.Path,
		fmt.Sprintf("文件数: %d", item.FileCount),
		"更新时间: " + item.UpdatedAt.Format("2006-01-02 15:04:05"),
	}, "\n")
}

func formatScreenshotFileDetail(item screenshotFileItem) string {
	return strings.Join([]string{
		"文件: " + item.Name,
		"路径: " + item.Path,
		"大小: " + formatFileSize(item.Size),
		"更新时间: " + item.UpdatedAt.Format("2006-01-02 15:04:05"),
	}, "\n")
}

func formatReasonType(reasonType string) string {
	switch reasonType {
	case "invalid_format":
		return "格式非法"
	case "dns":
		return "DNS 解析失败"
	case "timeout":
		return "连接超时"
	case "tls":
		return "TLS/证书错误"
	case "connection_refused":
		return "连接被拒绝"
	case "http_status":
		return "HTTP 状态"
	case "network":
		return "网络错误"
	default:
		return reasonType
	}
}

func formatTimestamp(ts int64) string {
	if ts <= 0 {
		return "-"
	}
	return time.Unix(ts, 0).Format("2006-01-02 15:04:05")
}

func formatAnyTimestamp(value interface{}) string {
	switch v := value.(type) {
	case int64:
		return formatTimestamp(v)
	case int:
		return formatTimestamp(int64(v))
	case float64:
		return formatTimestamp(int64(v))
	default:
		return "-"
	}
}

func formatFileSize(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	if size < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(size)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
}

func yesNo(value bool) string {
	if value {
		return "是"
	}
	return "否"
}

func openPathInSystem(path string) error {
	cleanPath := filepath.Clean(path)
	if _, err := os.Stat(cleanPath); err != nil {
		return err
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", cleanPath)
	case "darwin":
		cmd = exec.Command("open", cleanPath)
	default:
		cmd = exec.Command("xdg-open", cleanPath)
	}
	return cmd.Start()
}
