package unimap

import (
	"fmt"
	"strings"
	"text/scanner"

	"github.com/unimap-icp-hunter/project/internal/model"
)

// UQLParser UQL查询语言解析器
type UQLParser struct {
	scanner *scanner.Scanner
	current rune
}

// NewUQLParser 创建解析器
func NewUQLParser() *UQLParser {
	return &UQLParser{}
}

// Parse 解析UQL查询字符串为AST
func (p *UQLParser) Parse(query string) (*model.UQLAST, error) {
	if query == "" {
		return nil, fmt.Errorf("query cannot be empty")
	}

	// 简单的词法分析
	tokens := p.tokenize(query)
	if len(tokens) == 0 {
		return nil, fmt.Errorf("no valid tokens found")
	}

	// 构建AST
	root, err := p.buildAST(tokens)
	if err != nil {
		return nil, err
	}

	return &model.UQLAST{Root: root}, nil
}

// tokenize 将查询字符串分解为token
func (p *UQLParser) tokenize(query string) []string {
	// 简单的tokenize实现
	// 支持: field="value", field="value" && field="value", field IN ["a", "b"]

	tokens := []string{}
	current := ""
	inQuotes := false
	inBrackets := false
	escape := false

	for _, ch := range query {
		if escape {
			current += string(ch)
			escape = false
			continue
		}

		if ch == '\\' {
			escape = true
			continue
		}

		if ch == '"' {
			inQuotes = !inQuotes
			current += string(ch)
			continue
		}

		if ch == '[' {
			inBrackets = true
			current += string(ch)
			continue
		}

		if ch == ']' {
			inBrackets = false
			current += string(ch)
			continue
		}

		if !inQuotes && !inBrackets && (ch == ' ' || ch == '\t' || ch == '\n') {
			if current != "" {
				tokens = append(tokens, current)
				current = ""
			}
			continue
		}

		if !inQuotes && !inBrackets && (ch == '&' || ch == '|' || ch == '=' || ch == '!' || ch == '>' || ch == '<') {
			if current != "" {
				tokens = append(tokens, current)
				current = ""
			}
			// 处理多字符操作符
			if ch == '&' && p.peekNext(query, ch) == '&' {
				tokens = append(tokens, "&&")
				current = ""
				// 跳过下一个&
				continue
			}
			if ch == '|' && p.peekNext(query, ch) == '|' {
				tokens = append(tokens, "||")
				current = ""
				continue
			}
			tokens = append(tokens, string(ch))
			continue
		}

		current += string(ch)
	}

	if current != "" {
		tokens = append(tokens, current)
	}

	return tokens
}

func (p *UQLParser) peekNext(query string, current rune) rune {
	// 简化实现，实际应该维护索引
	return ' ' // 占位
}

// buildAST 从token构建AST
func (p *UQLParser) buildAST(tokens []string) (*model.UQLNode, error) {
	if len(tokens) == 0 {
		return nil, fmt.Errorf("no tokens")
	}

	// 简单的递归下降解析
	// 支持: field="value", field IN [values], (condition), condition && condition

	var parseExpr func(int) (*model.UQLNode, int, error)

	parseExpr = func(start int) (*model.UQLNode, int, error) {
		if start >= len(tokens) {
			return nil, start, fmt.Errorf("unexpected end of expression")
		}

		// 处理括号
		if tokens[start] == "(" {
			node, next, err := parseExpr(start + 1)
			if err != nil {
				return nil, start, err
			}
			if next >= len(tokens) || tokens[next] != ")" {
				return nil, start, fmt.Errorf("missing closing parenthesis")
			}
			return node, next + 1, nil
		}

		// 基础条件: field operator value
		if start+2 >= len(tokens) {
			return nil, start, fmt.Errorf("incomplete condition")
		}

		field := tokens[start]
		operator := tokens[start+1]

		// 处理 IN 操作符
		if strings.ToUpper(operator) == "IN" {
			if start+3 >= len(tokens) || tokens[start+2] != "[" {
				return nil, start, fmt.Errorf("expected [ after IN")
			}
			// 查找闭合]
			end := start + 3
			for end < len(tokens) && tokens[end] != "]" {
				end++
			}
			if end >= len(tokens) {
				return nil, start, fmt.Errorf("missing closing bracket")
			}

			// 收集数组值
			values := []string{}
			for i := start + 3; i < end; i++ {
				// 去除引号
				val := strings.Trim(tokens[i], `"`)
				values = append(values, val)
			}

			node := &model.UQLNode{
				Type:  "condition",
				Value: field,
				Children: []*model.UQLNode{
					{Type: "operator", Value: "IN"},
					{Type: "array", Value: strings.Join(values, ",")},
				},
			}
			return node, end + 1, nil
		}

		// 处理等号操作符
		if operator == "=" || operator == "==" {
			if start+2 >= len(tokens) {
				return nil, start, fmt.Errorf("missing value")
			}
			value := strings.Trim(tokens[start+2], `"`)
			node := &model.UQLNode{
				Type:  "condition",
				Value: field,
				Children: []*model.UQLNode{
					{Type: "operator", Value: operator},
					{Type: "value", Value: value},
				},
			}
			return node, start + 3, nil
		}

		return nil, start, fmt.Errorf("unsupported operator: %s", operator)
	}

	// 构建完整AST，处理AND/OR
	var root *model.UQLNode
	index := 0

	for index < len(tokens) {
		node, next, err := parseExpr(index)
		if err != nil {
			return nil, err
		}

		if root == nil {
			root = node
		} else {
			// 需要找到前面的AND/OR
			// 这里简化处理，实际应该在parseExpr中处理
			root = &model.UQLNode{
				Type:     "logical",
				Value:    "AND",
				Children: []*model.UQLNode{root, node},
			}
		}

		index = next

		// 检查是否有AND/OR
		if index < len(tokens) {
			if tokens[index] == "&&" || strings.ToUpper(tokens[index]) == "AND" {
				index++
			} else if tokens[index] == "||" || strings.ToUpper(tokens[index]) == "OR" {
				// OR操作
				root.Type = "logical"
				root.Value = "OR"
				index++
			}
		}
	}

	return root, nil
}

// ExtractConditions 提取查询条件
func (p *UQLParser) ExtractConditions(ast *model.UQLAST) map[string]interface{} {
	conditions := make(map[string]interface{})
	if ast == nil || ast.Root == nil {
		return conditions
	}

	var traverse func(*model.UQLNode)
	traverse = func(node *model.UQLNode) {
		if node == nil {
			return
		}

		if node.Type == "condition" && len(node.Children) >= 2 {
			field := node.Value
			op := node.Children[0].Value
			val := node.Children[1].Value

			if op == "IN" {
				// 解析数组
				values := strings.Split(val, ",")
				conditions[field] = map[string]interface{}{
					"operator": "IN",
					"value":    values,
				}
			} else {
				conditions[field] = map[string]interface{}{
					"operator": op,
					"value":    val,
				}
			}
		}

		// 递归子节点
		for _, child := range node.Children {
			traverse(child)
		}
	}

	traverse(ast.Root)
	return conditions
}

// Validate 验证UQL查询的有效性
func (p *UQLParser) Validate(query string) error {
	if strings.TrimSpace(query) == "" {
		return fmt.Errorf("query cannot be empty")
	}

	// 检查基本结构
	if !strings.Contains(query, "=") && !strings.Contains(strings.ToUpper(query), "IN") {
		return fmt.Errorf("query must contain at least one condition with = or IN")
	}

	return nil
}

// Simplify 简化UQL查询（移除多余空格等）
func (p *UQLParser) Simplify(query string) string {
	query = strings.TrimSpace(query)
	// 标准化空格
	query = strings.ReplaceAll(query, "  ", " ")
	query = strings.ReplaceAll(query, "\t", " ")
	query = strings.ReplaceAll(query, "\n", " ")
	return strings.TrimSpace(query)
}
