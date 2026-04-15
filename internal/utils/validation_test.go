package utils

import (
	"strings"
	"testing"
)

func TestSanitizeInput_StripsHTML(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple script tag",
			input: `<script>alert("xss")</script>hello`,
			want:  `hello`,
		},
		{
			name:  "anchor tag",
			input: `<a href="http://evil.com">click</a>`,
			want:  `click`,
		},
		{
			name:  "nested tags",
			input: `<div><p>text</p></div>`,
			want:  `text`,
		},
		{
			name:  "plain text",
			input: `hello world`,
			want:  `hello world`,
		},
		{
			name:  "empty string",
			input: ``,
			want:  ``,
		},
		{
			name:  "event handler attribute",
			input: `<img src=x onerror="alert(1)">`,
			want:  ``,
		},
		{
			name:  "encoded script",
			input: `<script>alert(1)</script>safe text<b>bold</b>`,
			want:  `safe textbold`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeInput(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeInput(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSanitizeInput_NoScriptRemains(t *testing.T) {
	malicious := `<script>document.cookie</script>`
	result := SanitizeInput(malicious)
	if strings.Contains(strings.ToLower(result), "script") {
		t.Errorf("script tag content should be stripped, got: %q", result)
	}
}

func TestSanitizeQuery_RemovesDangerousChars(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "single quotes removed",
			input: "test'value",
			want:  "testvalue",
		},
		{
			name:  "semicolon removed",
			input: "DROP TABLE users;--",
			want:  "DROP TABLE users",
		},
		{
			name:  "comment syntax removed",
			input: "SELECT /* comment */ * FROM users",
			want:  "SELECT  comment  * FROM users",
		},
		{
			name:  "leading/trailing whitespace trimmed",
			input: "  query  ",
			want:  "query",
		},
		{
			name:  "normal query unchanged",
			input: "SELECT * FROM users WHERE id = 1",
			want:  "SELECT * FROM users WHERE id = 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeQuery(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeQuery(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestValidator_Basic(t *testing.T) {
	v := NewValidator()
	v.ValidateRequired("name", "", "Name")

	if !v.HasErrors() {
		t.Fatal("expected validation error")
	}

	errs := v.Errors()
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}

	if errs[0].Code != 6001 {
		t.Errorf("expected code 6001, got %d", errs[0].Code)
	}
}

func TestValidator_NoErrors(t *testing.T) {
	v := NewValidator()
	v.ValidateRequired("name", "John", "Name")

	if v.HasErrors() {
		t.Fatal("expected no errors")
	}

	if v.ErrorMessage() != "" {
		t.Fatal("expected empty error message")
	}
}
