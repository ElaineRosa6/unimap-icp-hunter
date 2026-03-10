package unimap

import (
	"testing"
)

func TestUQLParser_Parse(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		wantErr bool
	}{
		{
			name:    "Empty query",
			query:   "",
			wantErr: true,
		},
		{
			name:    "Simple equality",
			query:   `country="CN"`,
			wantErr: false,
		},
		{
			name:    "Contains operator",
			query:   `title~="登录"`,
			wantErr: false,
		},
		{
			name:    "IN operator",
			query:   `port IN ["80", "443"]`,
			wantErr: false,
		},
		{
			name:    "Not equal operator",
			query:   `country!="CN"`,
			wantErr: false,
		},
		{
			name:    "Greater or equal operator",
			query:   `status_code>=200`,
			wantErr: false,
		},
		{
			name:    "Less or equal operator",
			query:   `status_code<="399"`,
			wantErr: false,
		},
		{
			name:    "CONTAINS keyword",
			query:   `title CONTAINS "admin"`,
			wantErr: false,
		},
	}

	parser := NewUQLParser()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ast, err := parser.Parse(tt.query)
			if (err != nil) != tt.wantErr {
				t.Errorf("UQLParser.Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && ast == nil {
				t.Errorf("UQLParser.Parse() returned nil AST for valid query")
			}
		})
	}
}

func TestUQLParser_Parse_WithParentheses(t *testing.T) {
	parser := NewUQLParser()
	query := `(country="CN" && (title="admin" || title="login"))`

	ast, err := parser.Parse(query)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if ast == nil || ast.Root == nil {
		t.Fatal("Parse() returned nil AST")
	}
	if ast.Root.Type != "logical" || ast.Root.Value != "AND" {
		t.Fatalf("root = %v/%v, want logical/AND", ast.Root.Type, ast.Root.Value)
	}
	if len(ast.Root.Children) != 2 {
		t.Fatalf("root children = %d, want 2", len(ast.Root.Children))
	}
	if ast.Root.Children[0].Type != "condition" || ast.Root.Children[0].Value != "country" {
		t.Fatalf("left child = %v/%v, want condition/country", ast.Root.Children[0].Type, ast.Root.Children[0].Value)
	}
	if ast.Root.Children[1].Type != "logical" || ast.Root.Children[1].Value != "OR" {
		t.Fatalf("right child = %v/%v, want logical/OR", ast.Root.Children[1].Type, ast.Root.Children[1].Value)
	}
}

func TestUQLParser_Parse_Precedence(t *testing.T) {
	parser := NewUQLParser()
	query := `app="redis" || port="6379" && country="CN"`

	ast, err := parser.Parse(query)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if ast == nil || ast.Root == nil {
		t.Fatal("Parse() returned nil AST")
	}
	if ast.Root.Type != "logical" || ast.Root.Value != "OR" {
		t.Fatalf("root = %v/%v, want logical/OR", ast.Root.Type, ast.Root.Value)
	}
	if len(ast.Root.Children) != 2 {
		t.Fatalf("root children = %d, want 2", len(ast.Root.Children))
	}
	if ast.Root.Children[1].Type != "logical" || ast.Root.Children[1].Value != "AND" {
		t.Fatalf("right child = %v/%v, want logical/AND", ast.Root.Children[1].Type, ast.Root.Children[1].Value)
	}
}

func TestUQLParser_Parse_IN_AST(t *testing.T) {
	parser := NewUQLParser()
	query := `port IN ["80", "443"]`

	ast, err := parser.Parse(query)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if ast == nil || ast.Root == nil {
		t.Fatal("Parse() returned nil AST")
	}
	if ast.Root.Type != "condition" || ast.Root.Value != "port" {
		t.Fatalf("root = %v/%v, want condition/port", ast.Root.Type, ast.Root.Value)
	}
	if len(ast.Root.Children) != 2 {
		t.Fatalf("root children = %d, want 2", len(ast.Root.Children))
	}
	if ast.Root.Children[0].Value != "IN" {
		t.Fatalf("operator = %v, want IN", ast.Root.Children[0].Value)
	}
	if ast.Root.Children[1].Value != "80,443" {
		t.Fatalf("array value = %v, want 80,443", ast.Root.Children[1].Value)
	}
}

func TestUQLParser_Tokenize(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  int // Expected minimum number of tokens
	}{
		{
			name:  "Simple query",
			query: `country="CN"`,
			want:  1,
		},
		{
			name:  "Query with quotes",
			query: `title="管理后台"`,
			want:  1,
		},
	}

	parser := NewUQLParser()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := parser.tokenize(tt.query)
			if len(tokens) < tt.want {
				t.Errorf("tokenize() got %d tokens, want at least %d", len(tokens), tt.want)
			}
		})
	}
}
