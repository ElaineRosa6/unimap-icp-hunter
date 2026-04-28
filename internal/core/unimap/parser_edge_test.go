package unimap

import (
	"testing"

	"github.com/unimap-icp-hunter/project/internal/model"
)

func TestTokenizeSimpleQuery(t *testing.T) {
	parser := NewUQLParser()
	tokens := parser.tokenize(`domain="example.com"`)
	if len(tokens) != 3 {
		t.Errorf("expected 3 tokens, got %d: %v", len(tokens), tokens)
	}
}

func TestTokenizeWithWhitespace(t *testing.T) {
	parser := NewUQLParser()
	tokens := parser.tokenize("  domain   =   \"test\"  ")
	if len(tokens) != 3 {
		t.Errorf("expected 3 tokens, got %d: %v", len(tokens), tokens)
	}
}

func TestTokenizeWithEscapedQuote(t *testing.T) {
	parser := NewUQLParser()
	tokens := parser.tokenize(`title="test \"quoted\""`)
	if len(tokens) != 3 {
		t.Errorf("expected 3 tokens, got %d: %v", len(tokens), tokens)
	}
}

func TestTokenizeWithParentheses(t *testing.T) {
	parser := NewUQLParser()
	tokens := parser.tokenize(`(domain="example.com")`)
	expected := []string{"(", "domain", "=", `"example.com"`, ")"}
	if len(tokens) != len(expected) {
		t.Errorf("expected %d tokens, got %d: %v", len(expected), len(tokens), tokens)
	}
}

func TestTokenizeINOperator(t *testing.T) {
	parser := NewUQLParser()
	tokens := parser.tokenize(`port IN ["80", "443"]`)
	expected := []string{"port", "IN", "[", `"80"`, `"443"`, "]"}
	if len(tokens) != len(expected) {
		t.Errorf("expected %d tokens, got %d: %v", len(expected), len(tokens), tokens)
	}
}

func TestTokenizeMultipleOperators(t *testing.T) {
	parser := NewUQLParser()
	testCases := []struct {
		query string
		op    string
	}{
		{`port != "80"`, "!="},
		{`port <> "80"`, "<>"},
		{`port >= "80"`, ">="},
		{`port <= "80"`, "<="},
		{`title ~= "test"`, "~="},
		{`port > "80"`, ">"},
		{`port < "80"`, "<"},
	}

	for _, tc := range testCases {
		t.Run(tc.op, func(t *testing.T) {
			tokens := parser.tokenize(tc.query)
			found := false
			for _, token := range tokens {
				if token == tc.op {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected operator %s in tokens: %v", tc.op, tokens)
			}
		})
	}
}

func TestTokenizeLogicalOperators(t *testing.T) {
	parser := NewUQLParser()
	testCases := []struct {
		query string
		op    string
	}{
		{`a="1" && b="2"`, "&&"},
		{`a="1" || b="2"`, "||"},
		{`a="1" AND b="2"`, "AND"},
		{`a="1" OR b="2"`, "OR"},
	}

	for _, tc := range testCases {
		t.Run(tc.op, func(t *testing.T) {
			tokens := parser.tokenize(tc.query)
			found := false
			for _, token := range tokens {
				if token == tc.op {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected operator %s in tokens: %v", tc.op, tokens)
			}
		})
	}
}

func TestBuildASTSimpleCondition(t *testing.T) {
	parser := NewUQLParser()
	tokens := []string{"domain", "=", `"example.com"`}
	node, err := parser.buildAST(tokens)
	if err != nil {
		t.Fatalf("buildAST failed: %v", err)
	}
	if node.Type != "condition" {
		t.Errorf("expected condition node, got %s", node.Type)
	}
	if node.Value != "domain" {
		t.Errorf("expected field=domain, got %s", node.Value)
	}
}

func TestBuildASTWithParentheses(t *testing.T) {
	parser := NewUQLParser()
	tokens := []string{"(", "domain", "=", `"example.com"`, ")"}
	node, err := parser.buildAST(tokens)
	if err != nil {
		t.Fatalf("buildAST failed: %v", err)
	}
	if node.Type != "condition" {
		t.Errorf("expected condition node, got %s", node.Type)
	}
}

func TestBuildASTINOperator(t *testing.T) {
	parser := NewUQLParser()
	tokens := []string{"port", "IN", "[", `"80"`, `"443"`, "]"}
	node, err := parser.buildAST(tokens)
	if err != nil {
		t.Fatalf("buildAST failed: %v", err)
	}
	if node.Type != "condition" {
		t.Errorf("expected condition node, got %s", node.Type)
	}
}

func TestBuildASTLogicalAND(t *testing.T) {
	parser := NewUQLParser()
	tokens := []string{"a", "=", `"1"`, "&&", "b", "=", `"2"`}
	node, err := parser.buildAST(tokens)
	if err != nil {
		t.Fatalf("buildAST failed: %v", err)
	}
	if node.Type != "logical" || node.Value != "AND" {
		t.Errorf("expected logical AND node, got type=%s value=%s", node.Type, node.Value)
	}
}

func TestBuildASTLogicalOR(t *testing.T) {
	parser := NewUQLParser()
	tokens := []string{"a", "=", `"1"`, "||", "b", "=", `"2"`}
	node, err := parser.buildAST(tokens)
	if err != nil {
		t.Fatalf("buildAST failed: %v", err)
	}
	if node.Type != "logical" || node.Value != "OR" {
		t.Errorf("expected logical OR node, got type=%s value=%s", node.Type, node.Value)
	}
}

func TestBuildASTErrors(t *testing.T) {
	parser := NewUQLParser()

	testCases := []struct {
		name   string
		tokens []string
	}{
		{"empty tokens", []string{}},
		{"missing closing parenthesis", []string{"(", "a", "=", `"1"`}},
		{"missing [ after IN", []string{"port", "IN"}},
		{"missing ] after IN", []string{"port", "IN", "[", `"80"`}},
		{"incomplete condition", []string{"a", "="}},
		{"unsupported operator", []string{"a", "UNKNOWN", `"1"`}},
		{"unexpected token", []string{"a", "=", `"1"`, "extra"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parser.buildAST(tc.tokens)
			if err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestExtractConditions(t *testing.T) {
	parser := NewUQLParser()

	ast, err := parser.Parse(`domain="example.com" && port="80"`)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	conditions := parser.ExtractConditions(ast)
	if len(conditions) != 2 {
		t.Errorf("expected 2 conditions, got %d", len(conditions))
	}
}

func TestExtractConditionsIN(t *testing.T) {
	parser := NewUQLParser()

	ast, err := parser.Parse(`port IN ["80", "443"]`)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	conditions := parser.ExtractConditions(ast)
	if len(conditions) != 1 {
		t.Errorf("expected 1 condition, got %d", len(conditions))
	}
}

func TestExtractConditionsNilAST(t *testing.T) {
	parser := NewUQLParser()
	conditions := parser.ExtractConditions(nil)
	if len(conditions) != 0 {
		t.Errorf("expected 0 conditions for nil AST, got %d", len(conditions))
	}

	conditions = parser.ExtractConditions(&model.UQLAST{Root: nil})
	if len(conditions) != 0 {
		t.Errorf("expected 0 conditions for nil root, got %d", len(conditions))
	}
}

func TestValidate(t *testing.T) {
	parser := NewUQLParser()

	testCases := []struct {
		name    string
		query   string
		wantErr bool
	}{
		{"valid query with =", `domain="example.com"`, false},
		{"valid query with IN", `port IN ["80"]`, false},
		{"empty query", "", true},
		{"whitespace only", "   ", true},
		{"no operator", "just text", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := parser.Validate(tc.query)
			if (err != nil) != tc.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestSimplify(t *testing.T) {
	parser := NewUQLParser()

	testCases := []struct {
		name  string
		input string
		want  string
	}{
		{"trim whitespace", "  domain=\"test\"  ", "domain=\"test\""},
		{"multiple spaces", "domain  =  \"test\"", "domain = \"test\""},
		{"tabs", "domain\t=\t\"test\"", "domain = \"test\""},
		{"newlines", "domain\n=\n\"test\"", "domain = \"test\""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := parser.Simplify(tc.input)
			if got != tc.want {
				t.Errorf("Simplify() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseEdgeCases(t *testing.T) {
	parser := NewUQLParser()

	testCases := []struct {
		name  string
		query string
	}{
		{"nested parentheses", `((domain="test"))`},
		{"multiple AND", `a="1" && b="2" && c="3"`},
		{"AND OR combination", `a="1" && (b="2" || c="3")`},
		{"uppercase IN", `port IN ["80"]`},
		{"contains operator", `title CONTAINS "test"`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ast, err := parser.Parse(tc.query)
			if err != nil {
				t.Fatalf("Parse failed for %q: %v", tc.query, err)
			}
			if ast == nil || ast.Root == nil {
				t.Error("expected non-nil AST")
			}
		})
	}
}

func TestNewUQLParser(t *testing.T) {
	parser := NewUQLParser()
	if parser == nil {
		t.Fatal("expected non-nil parser")
	}
}

func TestParseNoValidTokens(t *testing.T) {
	parser := NewUQLParser()
	_, err := parser.Parse("   ")
	if err == nil {
		t.Error("expected error for whitespace-only query")
	}
}
