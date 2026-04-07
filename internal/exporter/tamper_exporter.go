package exporter

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/unimap-icp-hunter/project/internal/logger"
	"github.com/unimap-icp-hunter/project/internal/tamper"
	"github.com/xuri/excelize/v2"
)

// TamperExportResult 篡改检测导出结果
type TamperExportResult struct {
	URL          string `json:"url"`
	Reachable    string `json:"reachable"`
	Screenshot   string `json:"screenshot"`
	Tampered     string `json:"tampered"`
	BaselineTime string `json:"baseline_time"`
	CheckTime    string `json:"check_time"`
}

// TamperExporter 篡改检测导出器接口
type TamperExporter interface {
	Export(results []tamper.TamperCheckResult, filepath string) error
}

// TamperJSONExporter JSON导出器
type TamperJSONExporter struct{}

// NewTamperJSONExporter 创建JSON导出器
func NewTamperJSONExporter() *TamperJSONExporter {
	return &TamperJSONExporter{}
}

// Export 导出为JSON文件
func (e *TamperJSONExporter) Export(results []tamper.TamperCheckResult, filepath string) error {
	exportResults := e.convertToExportFormat(results)
	
	file, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(exportResults); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}

	return nil
}

// TamperExcelExporter Excel导出器
type TamperExcelExporter struct{}

// NewTamperExcelExporter 创建Excel导出器
func NewTamperExcelExporter() *TamperExcelExporter {
	return &TamperExcelExporter{}
}

// Export 导出为Excel文件
func (e *TamperExcelExporter) Export(results []tamper.TamperCheckResult, filepath string) error {
	exportResults := e.convertToExportFormat(results)
	
	f := excelize.NewFile()
	defer func() {
		if err := f.Close(); err != nil {
			logger.Warnf("Failed to close Excel file: %v", err)
		}
	}()

	sheetName := "Tamper Detection"
	index, err := f.NewSheet(sheetName)
	if err != nil {
		return fmt.Errorf("failed to create sheet: %w", err)
	}

	// 设置表头
	headers := []string{"URL", "可达性", "网页截图", "是否被篡改", "基线时间", "检测时间"}
	for i, header := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheetName, cell, header)
	}

	// 写入数据
	for i, result := range exportResults {
		row := i + 2 // 从第2行开始（第1行是表头）
		f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), result.URL)
		f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), result.Reachable)
		f.SetCellValue(sheetName, fmt.Sprintf("C%d", row), result.Screenshot)
		f.SetCellValue(sheetName, fmt.Sprintf("D%d", row), result.Tampered)
		f.SetCellValue(sheetName, fmt.Sprintf("E%d", row), result.BaselineTime)
		f.SetCellValue(sheetName, fmt.Sprintf("F%d", row), result.CheckTime)
	}

	// 设置默认活动工作表
	f.SetActiveSheet(index)

	// 保存文件
	if err := f.SaveAs(filepath); err != nil {
		return fmt.Errorf("failed to save Excel file: %w", err)
	}

	return nil
}

// convertToExportFormat 转换为导出格式
func (e *TamperJSONExporter) convertToExportFormat(results []tamper.TamperCheckResult) []TamperExportResult {
	var exportResults []TamperExportResult
	
	for _, result := range results {
		exportResult := TamperExportResult{
			URL:          result.URL,
			Reachable:    "是",
			Screenshot:   "",
			Tampered:     "否",
			BaselineTime: "",
			CheckTime:    time.Unix(result.Timestamp, 0).Format("2006-01-02 15:04:05"),
		}

		// 检查可达性
		if result.Status == "unreachable" || result.Status == "failed" {
			exportResult.Reachable = "否"
		}

		// 检查篡改状态
		if result.Tampered {
			exportResult.Tampered = "是"
		}

		// 检查基线时间
		if result.BaselineHash != nil {
			exportResult.BaselineTime = time.Unix(result.BaselineHash.Timestamp, 0).Format("2006-01-02 15:04:05")
		}

		exportResults = append(exportResults, exportResult)
	}

	return exportResults
}

// convertToExportFormat 转换为导出格式
func (e *TamperExcelExporter) convertToExportFormat(results []tamper.TamperCheckResult) []TamperExportResult {
	jsonExporter := &TamperJSONExporter{}
	return jsonExporter.convertToExportFormat(results)
}