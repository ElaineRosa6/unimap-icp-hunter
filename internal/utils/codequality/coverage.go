package codequality

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/unimap-icp-hunter/project/internal/logger"
)

// CoverageReport 测试覆盖率报告
type CoverageReport struct {
	FilePath    string
	Coverage    float64
	Functions   int
	CoveredFunc int
	Branches    int
	CoveredBranch int
	Lines       int
	CoveredLine int
}

// CoverageAnalyzer 覆盖率分析器
type CoverageAnalyzer struct{}

// NewCoverageAnalyzer 创建覆盖率分析器
func NewCoverageAnalyzer() *CoverageAnalyzer {
	return &CoverageAnalyzer{}
}

// RunCoverage 运行测试覆盖率分析
func (ca *CoverageAnalyzer) RunCoverage(packagePath string) ([]*CoverageReport, error) {
	// 创建临时文件存储覆盖率数据
	tempFile, err := os.CreateTemp("", "coverage-*.out")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	tempFile.Close()
	defer os.Remove(tempFile.Name())

	// 运行测试并生成覆盖率报告
	cmd := exec.Command("go", "test", "-coverprofile", tempFile.Name(), packagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to run coverage test: %w, output: %s", err, string(output))
	}

	// 分析覆盖率数据
	return ca.analyzeCoverage(tempFile.Name())
}

// RunDetailedCoverage 运行详细的覆盖率分析
func (ca *CoverageAnalyzer) RunDetailedCoverage(packagePath string) ([]*CoverageReport, error) {
	// 创建临时文件存储覆盖率数据
	tempFile, err := os.CreateTemp("", "coverage-*.out")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	tempFile.Close()
	defer os.Remove(tempFile.Name())

	// 运行测试并生成覆盖率报告（包含分支覆盖率）
	cmd := exec.Command("go", "test", "-coverprofile", tempFile.Name(), "-covermode=atomic", packagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to run detailed coverage test: %w, output: %s", err, string(output))
	}

	// 分析覆盖率数据
	return ca.analyzeCoverage(tempFile.Name())
}

// analyzeCoverage 分析覆盖率文件
func (ca *CoverageAnalyzer) analyzeCoverage(coverageFile string) ([]*CoverageReport, error) {
	data, err := os.ReadFile(coverageFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read coverage file: %w", err)
	}

	reports := make([]*CoverageReport, 0)
	lines := strings.Split(string(data), "\n")
	
	currentReport := &CoverageReport{}
	inFileSection := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "mode:") {
			continue
		}

		// 检查是否是文件行
		parts := strings.Split(line, ":")
		if len(parts) >= 2 && strings.HasSuffix(parts[0], ".go") {
			// 保存之前的报告
			if inFileSection && currentReport.FilePath != "" {
				reports = append(reports, currentReport)
			}

			// 开始新文件的报告
			currentReport = &CoverageReport{
				FilePath: parts[0],
			}
			inFileSection = true
		} else if inFileSection {
			// 解析覆盖率数据
			ca.parseCoverageLine(line, currentReport)
		}
	}

	// 保存最后一个报告
	if inFileSection && currentReport.FilePath != "" {
		reports = append(reports, currentReport)
	}

	// 计算覆盖率百分比
	for _, report := range reports {
		if report.Lines > 0 {
			report.Coverage = float64(report.CoveredLine) / float64(report.Lines) * 100
		}
	}

	return reports, nil
}

// parseCoverageLine 解析覆盖率行
func (ca *CoverageAnalyzer) parseCoverageLine(line string, report *CoverageReport) {
	parts := strings.Split(line, " ")
	if len(parts) < 3 {
		return
	}

	// 解析行号
	lineNum := 0
	fmt.Sscanf(parts[0], "%d", &lineNum)

	// 解析计数
	count := 0
	fmt.Sscanf(parts[1], "%d", &count)

	report.Lines++
	if count > 0 {
		report.CoveredLine++
	}

	// 如果有分支信息
	if len(parts) >= 4 {
		branchInfo := parts[3]
		if strings.HasPrefix(branchInfo, "(") && strings.HasSuffix(branchInfo, ")") {
			// 解析分支覆盖率
			var covered, total int
			fmt.Sscanf(branchInfo, "(%d/%d)", &covered, &total)
			report.Branches += total
			report.CoveredBranch += covered
		}
	}
}

// GetCoverageSummary 获取覆盖率摘要
func (ca *CoverageAnalyzer) GetCoverageSummary(reports []*CoverageReport) map[string]interface{} {
	totalLines := 0
	totalCovered := 0
	totalBranches := 0
	totalCoveredBranches := 0

	for _, report := range reports {
		totalLines += report.Lines
		totalCovered += report.CoveredLine
		totalBranches += report.Branches
		totalCoveredBranches += report.CoveredBranch
	}

	lineCoverage := 0.0
	if totalLines > 0 {
		lineCoverage = float64(totalCovered) / float64(totalLines) * 100
	}

	branchCoverage := 0.0
	if totalBranches > 0 {
		branchCoverage = float64(totalCoveredBranches) / float64(totalBranches) * 100
	}

	return map[string]interface{}{
		"total_files":           len(reports),
		"total_lines":           totalLines,
		"covered_lines":         totalCovered,
		"line_coverage":         lineCoverage,
		"total_branches":        totalBranches,
		"covered_branches":      totalCoveredBranches,
		"branch_coverage":       branchCoverage,
	}
}

// GenerateHTMLReport 生成HTML覆盖率报告
func (ca *CoverageAnalyzer) GenerateHTMLReport(coverageFile, outputDir string) error {
	// 确保输出目录存在
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// 生成HTML报告
	cmd := exec.Command("go", "tool", "cover", "-html", coverageFile, "-o", filepath.Join(outputDir, "coverage.html"))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to generate HTML report: %w, output: %s", err, string(output))
	}

	logger.Infof("HTML coverage report generated at: %s", filepath.Join(outputDir, "coverage.html"))
	return nil
}

// GenerateJSONReport 生成JSON覆盖率报告
func (ca *CoverageAnalyzer) GenerateJSONReport(reports []*CoverageReport, outputFile string) error {
	summary := ca.GetCoverageSummary(reports)
	
	report := map[string]interface{}{
		"summary": summary,
		"details": reports,
	}

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON report: %w", err)
	}

	if err := os.WriteFile(outputFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write JSON report: %w", err)
	}

	logger.Infof("JSON coverage report generated at: %s", outputFile)
	return nil
}