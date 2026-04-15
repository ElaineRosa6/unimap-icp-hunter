package codequality

import (
	"os"
	"path/filepath"
	"testing"
)

// --- Complexity Analyzer Tests ---

func TestNewComplexityAnalyzer(t *testing.T) {
	ca := NewComplexityAnalyzer()
	if ca.thresholds == nil {
		t.Error("expected thresholds to be initialized")
	}
	if ca.thresholds["low"] != 5 {
		t.Errorf("expected low threshold 5, got %d", ca.thresholds["low"])
	}
	if ca.thresholds["medium"] != 10 {
		t.Errorf("expected medium threshold 10, got %d", ca.thresholds["medium"])
	}
	if ca.thresholds["high"] != 15 {
		t.Errorf("expected high threshold 15, got %d", ca.thresholds["high"])
	}
	if ca.thresholds["critical"] != 20 {
		t.Errorf("expected critical threshold 20, got %d", ca.thresholds["critical"])
	}
}

func TestAnalyzeFile(t *testing.T) {
	// Create a temp Go file for analysis
	tmpDir := t.TempDir()
	goFile := filepath.Join(tmpDir, "sample.go")

	t.Run("analyzes simple file", func(t *testing.T) {
		content := `package sample

func Simple() int {
	return 1
}

func WithIf(x int) int {
	if x > 0 {
		return x
	}
	return 0
}

func WithLoop() int {
	sum := 0
	for i := 0; i < 10; i++ {
		sum += i
	}
	return sum
}

func ComplexFunction(a, b, c int) int {
	if a > 0 {
		if b > 0 {
			for i := 0; i < c; i++ {
				if i > 5 && c > 10 {
					switch i {
					case 1:
						return 1
					case 2:
						return 2
					case 3:
						return 3
					}
				}
			}
		}
	}
	return 0
}
`
		if err := os.WriteFile(goFile, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		ca := NewComplexityAnalyzer()
		metrics, err := ca.AnalyzeFile(goFile)
		if err != nil {
			t.Fatalf("AnalyzeFile failed: %v", err)
		}

		if metrics.TotalFunctions != 4 {
			t.Errorf("expected 4 functions, got %d", metrics.TotalFunctions)
		}
		if metrics.MaxComplexity < 1 {
			t.Error("expected max complexity > 0")
		}
	})

	t.Run("file not found", func(t *testing.T) {
		ca := NewComplexityAnalyzer()
		_, err := ca.AnalyzeFile(filepath.Join(tmpDir, "nonexistent.go"))
		if err == nil {
			t.Error("expected error for nonexistent file")
		}
	})

	t.Run("invalid go file", func(t *testing.T) {
		invalidFile := filepath.Join(tmpDir, "invalid.go")
		if err := os.WriteFile(invalidFile, []byte("this is not go code {{{"), 0644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		ca := NewComplexityAnalyzer()
		_, err := ca.AnalyzeFile(invalidFile)
		if err == nil {
			t.Error("expected error for invalid go file")
		}
	})

	t.Run("high complexity detected", func(t *testing.T) {
		// Create a file with a function that exceeds the "high" threshold (15)
		content := `package sample

func VeryComplex() {
	if true { if true { if true { if true { if true {
	if true { if true { if true { if true { if true {
	if true { if true { if true { if true { if true {
	if true { if true { if true { if true { if true {
	} } } } } } } } } } } } } } } } } } } }
}
`
		if err := os.WriteFile(goFile, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		ca := NewComplexityAnalyzer()
		metrics, err := ca.AnalyzeFile(goFile)
		if err != nil {
			t.Fatalf("AnalyzeFile failed: %v", err)
		}

		if metrics.TotalFunctions != 1 {
			t.Errorf("expected 1 function, got %d", metrics.TotalFunctions)
		}
		// 20 if statements + 1 base = 21, which is >= critical (20)
		if len(metrics.Issues) == 0 {
			t.Error("expected complexity issues to be reported")
		}
	})

	t.Run("severity levels", func(t *testing.T) {
		// 20 if statements: complexity = 1 + 20 = 21, which is >= critical (20)
		content := "package sample\n\nfunc Critical() int {\n\tx := 0\n"
		for i := 0; i < 20; i++ {
			content += "\tif x > 0 {\n"
		}
		content += "\t\tx = 1\n"
		for i := 0; i < 20; i++ {
			content += "\t}\n"
		}
		content += "\treturn x\n}\n"

		if err := os.WriteFile(goFile, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		ca := NewComplexityAnalyzer()
		metrics, err := ca.AnalyzeFile(goFile)
		if err != nil {
			t.Fatalf("AnalyzeFile failed: %v", err)
		}

		if len(metrics.Issues) > 0 {
			issue := metrics.Issues[0]
			if issue.Severity != "critical" {
				t.Errorf("expected critical severity, got %q", issue.Severity)
			}
		}
	})
}

func TestAnalyzeDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	// Create multiple Go files
	files := map[string]string{
		"simple.go": `package sample
func Simple() int { return 1 }
`,
		"another.go": `package sample
func Another() int { return 2 }
`,
	}

	for name, content := range files {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte(content), 0644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}
	}

	ca := NewComplexityAnalyzer()
	metricsList, err := ca.AnalyzeDirectory(tmpDir)
	if err != nil {
		t.Fatalf("AnalyzeDirectory failed: %v", err)
	}

	if len(metricsList) != 2 {
		t.Errorf("expected 2 metrics, got %d", len(metricsList))
	}
}

func TestAnalyzeDirectoryInvalidPath(t *testing.T) {
	ca := NewComplexityAnalyzer()
	_, err := ca.AnalyzeDirectory("/nonexistent/path")
	if err == nil {
		t.Error("expected error for nonexistent directory")
	}
}

func TestGetSummary(t *testing.T) {
	ca := NewComplexityAnalyzer()

	t.Run("empty metrics list", func(t *testing.T) {
		summary := ca.GetSummary(nil)
		if summary["total_files"] != 0 {
			t.Errorf("expected 0 files, got %v", summary["total_files"])
		}
		if summary["total_functions"] != 0 {
			t.Errorf("expected 0 functions, got %v", summary["total_functions"])
		}
	})

	t.Run("aggregates metrics", func(t *testing.T) {
		metrics := []*ComplexityMetrics{
			{
				FilePath:        "a.go",
				TotalFunctions:  2,
				HighComplexity:  1,
				AverageComplexity: 5.0,
				MaxComplexity:   10,
				Issues:          []ComplexityIssue{{Severity: "high"}},
			},
			{
				FilePath:        "b.go",
				TotalFunctions:  3,
				HighComplexity:  2,
				AverageComplexity: 3.0,
				MaxComplexity:   8,
				Issues:          []ComplexityIssue{{Severity: "medium"}, {Severity: "low"}},
			},
		}

		summary := ca.GetSummary(metrics)
		if summary["total_files"] != 2 {
			t.Errorf("expected 2 files, got %v", summary["total_files"])
		}
		if summary["total_functions"] != 5 {
			t.Errorf("expected 5 functions, got %v", summary["total_functions"])
		}
		if summary["high_complexity_count"] != 3 {
			t.Errorf("expected 3 high complexity, got %v", summary["high_complexity_count"])
		}
		if summary["total_issues"] != 3 {
			t.Errorf("expected 3 total issues, got %v", summary["total_issues"])
		}
		if summary["max_complexity"] != 10 {
			t.Errorf("expected max complexity 10, got %v", summary["max_complexity"])
		}
	})
}

// --- Coverage Analyzer Tests ---

func TestNewCoverageAnalyzer(t *testing.T) {
	ca := NewCoverageAnalyzer()
	if ca == nil {
		t.Error("expected non-nil analyzer")
	}
}

func TestGetCoverageSummary(t *testing.T) {
	ca := NewCoverageAnalyzer()

	t.Run("empty reports", func(t *testing.T) {
		summary := ca.GetCoverageSummary(nil)
		if summary["total_files"] != 0 {
			t.Errorf("expected 0 files, got %v", summary["total_files"])
		}
		if summary["total_lines"] != 0 {
			t.Errorf("expected 0 lines, got %v", summary["total_lines"])
		}
	})

	t.Run("aggregates coverage", func(t *testing.T) {
		reports := []*CoverageReport{
			{
				FilePath:    "a.go",
				Lines:       100,
				CoveredLine: 80,
				Branches:    20,
				CoveredBranch: 10,
			},
			{
				FilePath:    "b.go",
				Lines:       50,
				CoveredLine: 25,
				Branches:    10,
				CoveredBranch: 5,
			},
		}

		summary := ca.GetCoverageSummary(reports)
		if summary["total_files"] != 2 {
			t.Errorf("expected 2 files, got %v", summary["total_files"])
		}
		if summary["total_lines"] != 150 {
			t.Errorf("expected 150 lines, got %v", summary["total_lines"])
		}
		if summary["covered_lines"] != 105 {
			t.Errorf("expected 105 covered lines, got %v", summary["covered_lines"])
		}
	})
}

func TestGenerateJSONReport(t *testing.T) {
	ca := NewCoverageAnalyzer()
	tmpDir := t.TempDir()
	outputFile := filepath.Join(tmpDir, "report.json")

	reports := []*CoverageReport{
		{
			FilePath:    "a.go",
			Lines:       10,
			CoveredLine: 8,
		},
	}

	t.Run("generates valid JSON", func(t *testing.T) {
		if err := ca.GenerateJSONReport(reports, outputFile); err != nil {
			t.Fatalf("GenerateJSONReport failed: %v", err)
		}

		data, err := os.ReadFile(outputFile)
		if err != nil {
			t.Fatalf("failed to read output: %v", err)
		}
		if len(data) == 0 {
			t.Error("expected non-empty JSON output")
		}
	})

	t.Run("invalid output path", func(t *testing.T) {
		err := ca.GenerateJSONReport(reports, "/nonexistent/path/report.json")
		if err == nil {
			t.Error("expected error for invalid output path")
		}
	})
}

func TestGenerateHTMLReport(t *testing.T) {
	ca := NewCoverageAnalyzer()
	tmpDir := t.TempDir()

	// Create a dummy coverage file
	coverageFile := filepath.Join(tmpDir, "coverage.out")
	if err := os.WriteFile(coverageFile, []byte("mode: atomic\n"), 0644); err != nil {
		t.Fatalf("failed to write coverage file: %v", err)
	}

	t.Run("generates HTML report", func(t *testing.T) {
		err := ca.GenerateHTMLReport(coverageFile, tmpDir)
		if err != nil {
			t.Fatalf("GenerateHTMLReport failed: %v", err)
		}

		// Check the file was created
		htmlFile := filepath.Join(tmpDir, "coverage.html")
		if _, err := os.Stat(htmlFile); err != nil {
			t.Errorf("expected HTML file to exist: %v", err)
		}
	})

	t.Run("creates output directory", func(t *testing.T) {
		outputDir := filepath.Join(tmpDir, "new-dir")
		err := ca.GenerateHTMLReport(coverageFile, outputDir)
		if err != nil {
			t.Fatalf("GenerateHTMLReport failed: %v", err)
		}
	})
}

func TestParseCoverageLine(t *testing.T) {
	ca := NewCoverageAnalyzer()

	t.Run("parses covered line", func(t *testing.T) {
		report := &CoverageReport{}
		// Real coverage line format: "lineNum count (branchInfo)"
		ca.parseCoverageLine("3 1 0", report)
		if report.Lines != 1 {
			t.Errorf("expected 1 line, got %d", report.Lines)
		}
		if report.CoveredLine != 1 {
			t.Errorf("expected 1 covered line, got %d", report.CoveredLine)
		}
	})

	t.Run("parses uncovered line", func(t *testing.T) {
		report := &CoverageReport{}
		ca.parseCoverageLine("3 0 0", report)
		ca.parseCoverageLine("5 0 0", report)
		if report.Lines != 2 {
			t.Errorf("expected 2 total lines, got %d", report.Lines)
		}
		if report.CoveredLine != 0 {
			t.Errorf("expected 0 covered lines, got %d", report.CoveredLine)
		}
	})

	t.Run("ignores malformed line", func(t *testing.T) {
		report := &CoverageReport{}
		before := report.Lines
		ca.parseCoverageLine("malformed", report)
		if report.Lines != before {
			t.Error("expected malformed line to be ignored")
		}
	})

	t.Run("parses branch info", func(t *testing.T) {
		report := &CoverageReport{}
		// Format: "lineNum count extra (branches)" - needs 4+ parts for branch info
		ca.parseCoverageLine("3 1 0 (1/2)", report)
		if report.Branches != 2 {
			t.Errorf("expected 2 branches, got %d", report.Branches)
		}
		if report.CoveredBranch != 1 {
			t.Errorf("expected 1 covered branch, got %d", report.CoveredBranch)
		}
	})
}

func TestAnalyzeCoverage(t *testing.T) {
	ca := NewCoverageAnalyzer()
	tmpDir := t.TempDir()
	coverageFile := filepath.Join(tmpDir, "coverage.out")

	t.Run("parses coverage file", func(t *testing.T) {
		content := `mode: atomic
github.com/unimap-icp-hunter/project/internal/sample/sample.go:3.13,5.2 1 1
github.com/unimap-icp-hunter/project/internal/sample/sample.go:7.13,9.2 1 0
`
		if err := os.WriteFile(coverageFile, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write coverage file: %v", err)
		}

		reports, err := ca.analyzeCoverage(coverageFile)
		if err != nil {
			t.Fatalf("analyzeCoverage failed: %v", err)
		}
		if len(reports) < 1 {
			t.Errorf("expected at least 1 report, got %d", len(reports))
		}
	})

	t.Run("invalid file path", func(t *testing.T) {
		_, err := ca.analyzeCoverage(filepath.Join(tmpDir, "nonexistent.out"))
		if err == nil {
			t.Error("expected error for nonexistent file")
		}
	})
}
