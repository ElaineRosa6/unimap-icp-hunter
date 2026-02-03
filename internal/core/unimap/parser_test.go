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
