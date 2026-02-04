package exporter

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/unimap-icp-hunter/project/internal/model"
	"github.com/xuri/excelize/v2"
)

// Exporter 导出器接口
type Exporter interface {
	Export(assets []model.UnifiedAsset, filepath string) error
}

// JSONExporter JSON导出器
type JSONExporter struct{}

// NewJSONExporter 创建JSON导出器
func NewJSONExporter() *JSONExporter {
	return &JSONExporter{}
}

// Export 导出为JSON文件
func (e *JSONExporter) Export(assets []model.UnifiedAsset, filepath string) error {
	file, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(assets); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}

	return nil
}

// ExcelExporter Excel导出器
type ExcelExporter struct{}

// NewExcelExporter 创建Excel导出器
func NewExcelExporter() *ExcelExporter {
	return &ExcelExporter{}
}

// Export 导出为Excel文件
func (e *ExcelExporter) Export(assets []model.UnifiedAsset, filepath string) error {
	f := excelize.NewFile()
	defer func() {
		if err := f.Close(); err != nil {
			fmt.Printf("Warning: failed to close Excel file: %v\n", err)
		}
	}()

	sheetName := "Assets"
	index, err := f.NewSheet(sheetName)
	if err != nil {
		return fmt.Errorf("failed to create sheet: %w", err)
	}

	// 设置表头
	headers := []string{"IP", "Port", "Protocol", "Host", "URL", "Title", "Server", "Status Code", "Country", "Region", "City", "ASN", "Org", "ISP", "Source"}
	for i, header := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheetName, cell, header)
	}

	// 写入数据
	for i, asset := range assets {
		row := i + 2 // 从第2行开始（第1行是表头）
		f.SetCellValue(sheetName, fmt.Sprintf("A%d", row), asset.IP)
		f.SetCellValue(sheetName, fmt.Sprintf("B%d", row), asset.Port)
		f.SetCellValue(sheetName, fmt.Sprintf("C%d", row), asset.Protocol)
		f.SetCellValue(sheetName, fmt.Sprintf("D%d", row), asset.Host)
		f.SetCellValue(sheetName, fmt.Sprintf("E%d", row), asset.URL)
		f.SetCellValue(sheetName, fmt.Sprintf("F%d", row), asset.Title)
		f.SetCellValue(sheetName, fmt.Sprintf("G%d", row), asset.Server)
		f.SetCellValue(sheetName, fmt.Sprintf("H%d", row), asset.StatusCode)
		f.SetCellValue(sheetName, fmt.Sprintf("I%d", row), asset.CountryCode)
		f.SetCellValue(sheetName, fmt.Sprintf("J%d", row), asset.Region)
		f.SetCellValue(sheetName, fmt.Sprintf("K%d", row), asset.City)
		f.SetCellValue(sheetName, fmt.Sprintf("L%d", row), asset.ASN)
		f.SetCellValue(sheetName, fmt.Sprintf("M%d", row), asset.Org)
		f.SetCellValue(sheetName, fmt.Sprintf("N%d", row), asset.ISP)
		f.SetCellValue(sheetName, fmt.Sprintf("O%d", row), asset.Source)
	}

	// 设置默认活动工作表
	f.SetActiveSheet(index)

	// 保存文件
	if err := f.SaveAs(filepath); err != nil {
		return fmt.Errorf("failed to save Excel file: %w", err)
	}

	return nil
}
