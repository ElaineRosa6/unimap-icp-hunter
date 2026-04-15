package analyzer

import (
	"fmt"
	"regexp"
	"strings"
)

// ScriptAnalyzer JavaScript脚本分析器
type ScriptAnalyzer struct {
	patterns *Patterns
}

// Patterns 检测模式
type Patterns struct {
	// 函数模式
	FunctionPattern      *regexp.Regexp
	// DOM操作模式
	DOMOperations        []*regexp.Regexp
	// 事件绑定模式
	EventBindings        []*regexp.Regexp
	// 动态代码执行模式
	DynamicExecution     []*regexp.Regexp
	// 网络请求模式
	NetworkRequests      []*regexp.Regexp
	// 危险操作模式
	DangerousOperations []*regexp.Regexp
}

// AnalysisResult 分析结果
type AnalysisResult struct {
	Functions            []FunctionInfo
	DOMOperations        []OperationInfo
	EventBindings        []BindingInfo
	DynamicExecution     []ExecutionInfo
	NetworkRequests      []RequestInfo
	DangerousOperations []OperationInfo
	SuspiciousSignals    []string
}

// FunctionInfo 函数信息
type FunctionInfo struct {
	Name        string
	StartLine   int
	EndLine     int
	Parameters  []string
	HasDynamicCode bool
	HasDOMManipulation bool
}

// OperationInfo DOM操作信息
type OperationInfo struct {
	Type        string
	Target      string
	Line        int
	Pattern     string
}

// BindingInfo 事件绑定信息
type BindingInfo struct {
	Element     string
	Event       string
	Handler     string
	Line        int
}

// ExecutionInfo 动态执行信息
type ExecutionInfo struct {
	Type        string
	Code        string
	Line        int
}

// RequestInfo 网络请求信息
type RequestInfo struct {
	Type        string
	URL         string
	Line        int
}

// NewScriptAnalyzer 创建脚本分析器
func NewScriptAnalyzer() *ScriptAnalyzer {
	return &ScriptAnalyzer{
		patterns: &Patterns{
			FunctionPattern: regexp.MustCompile(`function\s+(\w+)\s*\(([^)]*)\)`),
			DOMOperations: []*regexp.Regexp{
				regexp.MustCompile(`document\.write\s*\(`),
				regexp.MustCompile(`element\.innerHTML\s*=`),
				regexp.MustCompile(`document\.createElement\s*\(`),
				regexp.MustCompile(`element\.appendChild\s*\(`),
				regexp.MustCompile(`element\.setAttribute\s*\(`),
				regexp.MustCompile(`document\.body\.appendChild\s*\(`),
				regexp.MustCompile(`document\.head\.appendChild\s*\(`),
			},
			EventBindings: []*regexp.Regexp{
				regexp.MustCompile(`addEventListener\s*\(\s*['"]([^'"]+)['"]`),
				regexp.MustCompile(`on(\w+)\s*=`),
				regexp.MustCompile(`element\.on(\w+)\s*=`),
			},
			DynamicExecution: []*regexp.Regexp{
				regexp.MustCompile(`eval\s*\(`),
				regexp.MustCompile(`Function\s*\(`),
				regexp.MustCompile(`setTimeout\s*\([^,]+,\s*['"]`),
				regexp.MustCompile(`setInterval\s*\([^,]+,\s*['"]`),
			},
			NetworkRequests: []*regexp.Regexp{
				regexp.MustCompile(`XMLHttpRequest`),
				regexp.MustCompile(`fetch\s*\(`),
				regexp.MustCompile(`new\s+Image\s*\(\)`),
				regexp.MustCompile(`document\.write\s*\(['"]<script`),
			},
			DangerousOperations: []*regexp.Regexp{
				regexp.MustCompile(`document\.cookie\s*=`),
				regexp.MustCompile(`localStorage\.setItem\s*\(`),
				regexp.MustCompile(`sessionStorage\.setItem\s*\(`),
				regexp.MustCompile(`window\.location\s*=`),
				regexp.MustCompile(`document\.location\s*=`),
			},
		},
	}
}

// Analyze 分析JavaScript代码
func (a *ScriptAnalyzer) Analyze(script string) (*AnalysisResult, error) {
	if script == "" {
		return &AnalysisResult{}, nil
	}
	
	result := &AnalysisResult{}
	
	// 分析函数
	a.analyzeFunctions(script, result)
	
	// 分析DOM操作
	a.analyzeDOMOperations(script, result)
	
	// 分析事件绑定
	a.analyzeEventBindings(script, result)
	
	// 分析动态执行
	a.analyzeDynamicExecution(script, result)
	
	// 分析网络请求
	a.analyzeNetworkRequests(script, result)
	
	// 分析危险操作
	a.analyzeDangerousOperations(script, result)
	
	// 检测可疑信号
	a.detectSuspiciousSignals(result)
	
	return result, nil
}

// analyzeFunctions 分析函数
func (a *ScriptAnalyzer) analyzeFunctions(script string, result *AnalysisResult) {
	lines := strings.Split(script, "\n")
	
	for i, line := range lines {
		matches := a.patterns.FunctionPattern.FindStringSubmatch(line)
		if matches != nil && len(matches) >= 3 {
			function := FunctionInfo{
				Name:       matches[1],
				StartLine:  i + 1,
				Parameters: strings.Split(strings.TrimSpace(matches[2]), ","),
			}
			
			// 清理参数
			for j, param := range function.Parameters {
				function.Parameters[j] = strings.TrimSpace(param)
			}
			
			result.Functions = append(result.Functions, function)
		}
	}
}

// analyzeDOMOperations 分析DOM操作
func (a *ScriptAnalyzer) analyzeDOMOperations(script string, result *AnalysisResult) {
	lines := strings.Split(script, "\n")
	
	for i, line := range lines {
		for _, pattern := range a.patterns.DOMOperations {
			if pattern.MatchString(line) {
				match := pattern.FindString(line)
				operation := OperationInfo{
					Type:    "dom",
					Target:  match,
					Line:    i + 1,
					Pattern: pattern.String(),
				}
				result.DOMOperations = append(result.DOMOperations, operation)
			}
		}
	}
}

// analyzeEventBindings 分析事件绑定
func (a *ScriptAnalyzer) analyzeEventBindings(script string, result *AnalysisResult) {
	lines := strings.Split(script, "\n")
	
	for i, line := range lines {
		// 分析addEventListener
		matches := a.patterns.EventBindings[0].FindStringSubmatch(line)
		if matches != nil && len(matches) >= 2 {
			binding := BindingInfo{
				Element: "unknown",
				Event:   matches[1],
				Handler: "function",
				Line:    i + 1,
			}
			result.EventBindings = append(result.EventBindings, binding)
			continue
		}
		
		// 分析on事件属性
		matches = a.patterns.EventBindings[1].FindStringSubmatch(line)
		if matches != nil && len(matches) >= 2 {
			binding := BindingInfo{
				Element: "element",
				Event:   matches[1],
				Handler: "inline",
				Line:    i + 1,
			}
			result.EventBindings = append(result.EventBindings, binding)
			continue
		}
		
		// 分析element.on事件
		matches = a.patterns.EventBindings[2].FindStringSubmatch(line)
		if matches != nil && len(matches) >= 2 {
			binding := BindingInfo{
				Element: "element",
				Event:   matches[1],
				Handler: "function",
				Line:    i + 1,
			}
			result.EventBindings = append(result.EventBindings, binding)
		}
	}
}

// analyzeDynamicExecution 分析动态执行
func (a *ScriptAnalyzer) analyzeDynamicExecution(script string, result *AnalysisResult) {
	lines := strings.Split(script, "\n")
	
	for i, line := range lines {
		for _, pattern := range a.patterns.DynamicExecution {
			if pattern.MatchString(line) {
				match := pattern.FindString(line)
				var execType string
				
				switch {
				case strings.Contains(match, "eval"):
					execType = "eval"
				case strings.Contains(match, "Function"):
					execType = "function_constructor"
				case strings.Contains(match, "setTimeout"):
					execType = "settimeout"
				case strings.Contains(match, "setInterval"):
					execType = "setinterval"
				default:
					execType = "dynamic"
				}
				
				execution := ExecutionInfo{
					Type: execType,
					Code: match,
					Line: i + 1,
				}
				result.DynamicExecution = append(result.DynamicExecution, execution)
			}
		}
	}
}

// analyzeNetworkRequests 分析网络请求
func (a *ScriptAnalyzer) analyzeNetworkRequests(script string, result *AnalysisResult) {
	lines := strings.Split(script, "\n")
	
	for i, line := range lines {
		for _, pattern := range a.patterns.NetworkRequests {
			if pattern.MatchString(line) {
				match := pattern.FindString(line)
				var requestType string
				
				switch {
				case strings.Contains(match, "XMLHttpRequest"):
					requestType = "xhr"
				case strings.Contains(match, "fetch"):
					requestType = "fetch"
				case strings.Contains(match, "Image"):
					requestType = "image"
				case strings.Contains(match, "script"):
					requestType = "script"
				default:
					requestType = "network"
				}
				
				request := RequestInfo{
					Type: requestType,
					URL:  match,
					Line: i + 1,
				}
				result.NetworkRequests = append(result.NetworkRequests, request)
			}
		}
	}
}

// analyzeDangerousOperations 分析危险操作
func (a *ScriptAnalyzer) analyzeDangerousOperations(script string, result *AnalysisResult) {
	lines := strings.Split(script, "\n")
	
	for i, line := range lines {
		for _, pattern := range a.patterns.DangerousOperations {
			if pattern.MatchString(line) {
				match := pattern.FindString(line)
				var operationType string
				
				switch {
				case strings.Contains(match, "cookie"):
					operationType = "cookie"
				case strings.Contains(match, "localStorage"):
					operationType = "localstorage"
				case strings.Contains(match, "sessionStorage"):
					operationType = "sessionstorage"
				case strings.Contains(match, "location"):
					operationType = "location"
				default:
					operationType = "dangerous"
				}
				
				operation := OperationInfo{
					Type:    operationType,
					Target:  match,
					Line:    i + 1,
					Pattern: pattern.String(),
				}
				result.DangerousOperations = append(result.DangerousOperations, operation)
			}
		}
	}
}

// detectSuspiciousSignals 检测可疑信号
func (a *ScriptAnalyzer) detectSuspiciousSignals(result *AnalysisResult) {
	// 检测动态代码执行
	if len(result.DynamicExecution) > 0 {
		result.SuspiciousSignals = append(result.SuspiciousSignals, "dynamic_code_execution")
	}
	
	// 检测可疑的DOM操作
	dangerousDOMCount := 0
	for _, op := range result.DOMOperations {
		if strings.Contains(op.Target, "write") || strings.Contains(op.Target, "innerHTML") {
			dangerousDOMCount++
		}
	}
	if dangerousDOMCount > 0 {
		result.SuspiciousSignals = append(result.SuspiciousSignals, fmt.Sprintf("dangerous_dom_operations:%d", dangerousDOMCount))
	}
	
	// 检测可疑的事件绑定
	for _, binding := range result.EventBindings {
		if binding.Event == "error" || binding.Event == "load" || binding.Event == "mouseover" {
			result.SuspiciousSignals = append(result.SuspiciousSignals, fmt.Sprintf("suspicious_event_binding:%s", binding.Event))
		}
	}
	
	// 检测网络请求
	if len(result.NetworkRequests) > 5 {
		result.SuspiciousSignals = append(result.SuspiciousSignals, "excessive_network_requests")
	}
	
	// 检测危险操作
	if len(result.DangerousOperations) > 0 {
		result.SuspiciousSignals = append(result.SuspiciousSignals, "dangerous_operations_detected")
	}
	
	// 检测函数数量异常
	if len(result.Functions) > 20 {
		result.SuspiciousSignals = append(result.SuspiciousSignals, "excessive_functions")
	}
}

// GetSuspiciousLevel 获取可疑级别
func (a *ScriptAnalyzer) GetSuspiciousLevel(result *AnalysisResult) int {
	level := 0
	
	// 根据可疑信号计算级别
	for _, signal := range result.SuspiciousSignals {
		switch {
		case signal == "dynamic_code_execution":
			level += 5
		case strings.HasPrefix(signal, "dangerous_dom_operations"):
			level += 3
		case strings.HasPrefix(signal, "suspicious_event_binding"):
			level += 2
		case signal == "excessive_network_requests":
			level += 2
		case signal == "dangerous_operations_detected":
			level += 4
		case signal == "excessive_functions":
			level += 1
		}
	}
	
	return level
}