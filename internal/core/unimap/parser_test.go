package unimap

import "testing"

func TestParseLogicalAndIn(t *testing.T) {
	parser := NewUQLParser()

	ast, err := parser.Parse(`country="CN" && port IN ["80", "443"]`)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if ast == nil || ast.Root == nil {
		t.Fatalf("expected non-nil AST root")
	}
	if ast.Root.Type != "logical" || ast.Root.Value != "AND" {
		t.Fatalf("expected root logical AND, got type=%q value=%q", ast.Root.Type, ast.Root.Value)
	}
	if len(ast.Root.Children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(ast.Root.Children))
	}
}

func TestParseUTF8Condition(t *testing.T) {
	parser := NewUQLParser()

	ast, err := parser.Parse(`title~="管理后台"`)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if ast == nil || ast.Root == nil {
		t.Fatalf("expected non-nil AST root")
	}
	if ast.Root.Value != "title" {
		t.Fatalf("expected field title, got %q", ast.Root.Value)
	}
}

func TestParseErrors(t *testing.T) {
	parser := NewUQLParser()

	if _, err := parser.Parse(""); err == nil {
		t.Fatalf("expected error for empty query")
	}

	if _, err := parser.Parse(`port IN ["80", "443"`); err == nil {
		t.Fatalf("expected error for missing closing bracket")
	}
}
