package codequality

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/unimap-icp-hunter/project/internal/logger"
)

// ComplexityMetrics 代码复杂度指标
type ComplexityMetrics struct {
	FilePath       string
	TotalFunctions int
	HighComplexity int
	AverageComplexity float64
	MaxComplexity  int
	Issues         []ComplexityIssue
}

// ComplexityIssue 复杂度问题
type ComplexityIssue struct {
	FunctionName string
	Line         int
	Complexity   int
	Severity     string // "low", "medium", "high", "critical"
}

// ComplexityAnalyzer 代码复杂度分析器
type ComplexityAnalyzer struct {
	thresholds map[string]int
}

// NewComplexityAnalyzer 创建复杂度分析器
func NewComplexityAnalyzer() *ComplexityAnalyzer {
	return &ComplexityAnalyzer{
		thresholds: map[string]int{
			"low":      5,
			"medium":   10,
			"high":     15,
			"critical": 20,
		},
	}
}

// AnalyzeFile 分析单个文件的复杂度
func (ca *ComplexityAnalyzer) AnalyzeFile(filePath string) (*ComplexityMetrics, error) {
	// 读取文件内容
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	// 解析Go文件
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, content, parser.AllErrors)
	if err != nil {
		return nil, err
	}

	metrics := &ComplexityMetrics{
		FilePath: filePath,
		Issues:   make([]ComplexityIssue, 0),
	}

	// 遍历所有函数
	ast.Inspect(node, func(n ast.Node) bool {
		if fn, ok := n.(*ast.FuncDecl); ok {
			metrics.TotalFunctions++
			complexity := ca.calculateComplexity(fn)
			
			if complexity > 0 {
				metrics.AverageComplexity += float64(complexity)
				
				if complexity > metrics.MaxComplexity {
					metrics.MaxComplexity = complexity
				}
				
				// 检查是否超过阈值
				if complexity >= ca.thresholds["high"] {
					metrics.HighComplexity++
					
					severity := "low"
					if complexity >= ca.thresholds["critical"] {
						severity = "critical"
					} else if complexity >= ca.thresholds["high"] {
						severity = "high"
					} else if complexity >= ca.thresholds["medium"] {
						severity = "medium"
					}
					
					metrics.Issues = append(metrics.Issues, ComplexityIssue{
						FunctionName: fn.Name.Name,
						Line:         fset.Position(fn.Pos()).Line,
						Complexity:   complexity,
						Severity:     severity,
					})
				}
			}
		}
		return true
	})

	// 计算平均复杂度
	if metrics.TotalFunctions > 0 {
		metrics.AverageComplexity /= float64(metrics.TotalFunctions)
	}

	return metrics, nil
}

// AnalyzeDirectory 分析目录下的所有Go文件
func (ca *ComplexityAnalyzer) AnalyzeDirectory(dirPath string) ([]*ComplexityMetrics, error) {
	var allMetrics []*ComplexityMetrics

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 跳过目录和非Go文件
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}

		// 分析文件
		metrics, err := ca.AnalyzeFile(path)
		if err != nil {
			logger.Errorf("Failed to analyze file %s: %v", path, err)
			return nil // 继续分析其他文件
		}

		allMetrics = append(allMetrics, metrics)
		return nil
	})

	return allMetrics, err
}

// calculateComplexity 计算函数复杂度
func (ca *ComplexityAnalyzer) calculateComplexity(fn *ast.FuncDecl) int {
	if fn.Body == nil {
		return 0
	}

	complexity := 1 // 基础复杂度为1

	// 遍历函数体中的所有节点
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch n.(type) {
		case *ast.IfStmt:
			complexity++
		case *ast.ForStmt:
			complexity++
		case *ast.RangeStmt:
			complexity++
		case *ast.SwitchStmt:
			complexity++
		case *ast.SelectStmt:
			complexity++
		case *ast.CaseClause:
			// 每个case增加1，但default不算
			if cc, ok := n.(*ast.CaseClause); ok {
				if len(cc.List) > 0 {
					complexity++
				}
			}
		case *ast.BinaryExpr:
			// 逻辑运算符增加复杂度
			if be, ok := n.(*ast.BinaryExpr); ok {
				if be.Op == token.LAND || be.Op == token.LOR {
					complexity++
				}
			}
		case *ast.UnaryExpr:
			// 逻辑非增加复杂度
			if ue, ok := n.(*ast.UnaryExpr); ok {
				if ue.Op == token.NOT {
					complexity++
				}
			}
		}
		return true
	})

	return complexity
}

// GetSummary 获取分析摘要
func (ca *ComplexityAnalyzer) GetSummary(metricsList []*ComplexityMetrics) map[string]interface{} {
	totalFunctions := 0
	totalHighComplexity := 0
	totalIssues := 0
	maxComplexity := 0
	totalComplexity := 0.0

	for _, metrics := range metricsList {
		totalFunctions += metrics.TotalFunctions
		totalHighComplexity += metrics.HighComplexity
		totalIssues += len(metrics.Issues)
		totalComplexity += metrics.AverageComplexity * float64(metrics.TotalFunctions)
		
		if metrics.MaxComplexity > maxComplexity {
			maxComplexity = metrics.MaxComplexity
		}
	}

	averageComplexity := 0.0
	if totalFunctions > 0 {
		averageComplexity = totalComplexity / float64(totalFunctions)
	}

	return map[string]interface{}{
		"total_files":           len(metricsList),
		"total_functions":       totalFunctions,
		"high_complexity_count": totalHighComplexity,
		"total_issues":          totalIssues,
		"max_complexity":        maxComplexity,
		"average_complexity":    averageComplexity,
	}
}