package utils

import (
	"strings"
	"testing"
)

// ===== ValidateMinLength =====

func TestValidateMinLength(t *testing.T) {
	v := NewValidator()
	v.ValidateMinLength("name", "ab", 3, "Name")
	if !v.HasErrors() {
		t.Fatal("expected error for short string")
	}
	if !strings.Contains(v.Errors()[0].Message, "at least 3") {
		t.Errorf("unexpected message: %s", v.Errors()[0].Message)
	}

	v2 := NewValidator()
	v2.ValidateMinLength("name", "abcd", 3, "Name")
	if v2.HasErrors() {
		t.Error("expected no error for valid length")
	}

	// Unicode: 2 runes, should fail min 3
	v3 := NewValidator()
	v3.ValidateMinLength("name", "中文", 3, "Name")
	if !v3.HasErrors() {
		t.Error("expected error for short unicode string")
	}
}

// ===== ValidateMaxLength =====

func TestValidateMaxLength(t *testing.T) {
	v := NewValidator()
	v.ValidateMaxLength("name", "abcdef", 3, "Name")
	if !v.HasErrors() {
		t.Fatal("expected error for long string")
	}
	if !strings.Contains(v.Errors()[0].Message, "not exceed 3") {
		t.Errorf("unexpected message: %s", v.Errors()[0].Message)
	}

	v2 := NewValidator()
	v2.ValidateMaxLength("name", "ab", 3, "Name")
	if v2.HasErrors() {
		t.Error("expected no error for short string")
	}
}

// ===== ValidateEmail =====

func TestValidateEmail(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"empty", "", false},
		{"valid", "user@example.com", false},
		{"valid with dots", "user.name@example.com", false},
		{"missing @", "userexample.com", true},
		{"missing domain", "user@", true},
		{"missing local", "@example.com", true},
		{"no tld", "user@example", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator()
			v.ValidateEmail("email", tt.input)
			if tt.wantErr != v.HasErrors() {
				t.Errorf("ValidateEmail(%q) hasErrors=%v, want %v", tt.input, v.HasErrors(), tt.wantErr)
			}
		})
	}
}

// ===== ValidateURL =====

func TestValidateURL(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"empty", "", false},
		{"valid https", "https://example.com/path", false},
		{"valid http", "http://localhost:8080", false},
		{"invalid", "not-a-url", true},
		{"relative", "/path/to/resource", false}, // ParseRequestURI accepts relative paths
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator()
			v.ValidateURL("url", tt.input)
			if tt.wantErr != v.HasErrors() {
				t.Errorf("ValidateURL(%q) hasErrors=%v, want %v", tt.input, v.HasErrors(), tt.wantErr)
			}
		})
	}
}

// ===== ValidatePattern =====

func TestValidatePattern(t *testing.T) {
	v := NewValidator()
	v.ValidatePattern("phone", "123", `^\d{4,}$`, "Phone")
	if !v.HasErrors() {
		t.Fatal("expected error for pattern mismatch")
	}

	v2 := NewValidator()
	v2.ValidatePattern("phone", "12345", `^\d{4,}$`, "Phone")
	if v2.HasErrors() {
		t.Error("expected no error for matching pattern")
	}

	// empty value should pass
	v3 := NewValidator()
	v3.ValidatePattern("phone", "", `^\d{4,}$`, "Phone")
	if v3.HasErrors() {
		t.Error("expected no error for empty value")
	}
}

// ===== ValidateRange =====

func TestValidateRange(t *testing.T) {
	v := NewValidator()
	v.ValidateRange("age", 0, 1, 120, "Age")
	if !v.HasErrors() {
		t.Fatal("expected error for out of range")
	}

	v2 := NewValidator()
	v2.ValidateRange("age", 121, 1, 120, "Age")
	if !v2.HasErrors() {
		t.Fatal("expected error for above range")
	}

	v3 := NewValidator()
	v3.ValidateRange("age", 25, 1, 120, "Age")
	if v3.HasErrors() {
		t.Error("expected no error for in-range value")
	}
}

// ===== ValidateHTTPMethod =====

func TestValidateHTTPMethod(t *testing.T) {
	validMethods := []string{"GET", "POST", "PUT", "DELETE", "PATCH"}
	for _, m := range validMethods {
		v := NewValidator()
		v.ValidateHTTPMethod(m)
		if v.HasErrors() {
			t.Errorf("expected %s to be valid", m)
		}
	}

	v := NewValidator()
	v.ValidateHTTPMethod("INVALID")
	if !v.HasErrors() {
		t.Error("expected error for invalid method")
	}
}

// ===== ValidateAPIKey =====

func TestValidateAPIKey(t *testing.T) {
	// empty should pass
	v := NewValidator()
	v.ValidateAPIKey("key", "")
	if v.HasErrors() {
		t.Error("expected no error for empty API key")
	}

	// too short
	v2 := NewValidator()
	v2.ValidateAPIKey("key", "short")
	if !v2.HasErrors() {
		t.Error("expected error for short API key")
	}

	// valid: 32+ alphanumeric chars
	v3 := NewValidator()
	v3.ValidateAPIKey("key", "abcdefghijklmnopqrstuvwxyz123456")
	if v3.HasErrors() {
		t.Error("expected no error for valid API key")
	}

	// invalid: contains special chars
	v4 := NewValidator()
	v4.ValidateAPIKey("key", "abcdefghijklmnopqrstuvwxyz12345!")
	if !v4.HasErrors() {
		t.Error("expected error for API key with special chars")
	}
}

// ===== ValidateURLs =====

func TestValidateURLs(t *testing.T) {
	v := NewValidator()
	v.ValidateURLs("urls", []string{"https://example.com", "http://test.org"})
	if v.HasErrors() {
		t.Errorf("expected no errors, got: %v", v.Errors())
	}

	v2 := NewValidator()
	v2.ValidateURLs("urls", []string{"https://example.com", "", "not-a-url"})
	if !v2.HasErrors() {
		t.Fatal("expected errors for invalid URLs")
	}
	errs := v2.Errors()
	if len(errs) != 2 {
		t.Errorf("expected 2 errors, got %d", len(errs))
	}
}

// ===== ValidateConcurrency =====

func TestValidateConcurrency(t *testing.T) {
	v := NewValidator()
	v.ValidateConcurrency("workers", 0)
	if !v.HasErrors() {
		t.Error("expected error for concurrency < 1")
	}

	v2 := NewValidator()
	v2.ValidateConcurrency("workers", 101)
	if !v2.HasErrors() {
		t.Error("expected error for concurrency > 100")
	}

	v3 := NewValidator()
	v3.ValidateConcurrency("workers", 50)
	if v3.HasErrors() {
		t.Error("expected no error for valid concurrency")
	}

	v4 := NewValidator()
	v4.ValidateConcurrency("workers", 1)
	if v4.HasErrors() {
		t.Error("expected no error for min concurrency")
	}

	v5 := NewValidator()
	v5.ValidateConcurrency("workers", 100)
	if v5.HasErrors() {
		t.Error("expected no error for max concurrency")
	}
}

// ===== ToUnimapError =====

func TestValidator_ToUnimapError(t *testing.T) {
	v := NewValidator()
	if v.ToUnimapError() != nil {
		t.Error("expected nil for no errors")
	}

	v.ValidateRequired("name", "", "Name")
	err := v.ToUnimapError()
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if err.Code != 6000 {
		t.Errorf("expected code 6000, got %d", err.Code)
	}
}

// ===== ValidateRequestSize =====

func TestValidateRequestSize(t *testing.T) {
	tests := []struct {
		contentLength int64
		maxSizeMB     int
		want          bool
	}{
		{0, 10, true},
		{1024, 10, true},
		{10 * 1024 * 1024, 10, true},
		{11 * 1024 * 1024, 10, false},
		{100 * 1024 * 1024, 10, false},
	}
	for _, tt := range tests {
		got := ValidateRequestSize(tt.contentLength, tt.maxSizeMB)
		if got != tt.want {
			t.Errorf("ValidateRequestSize(%d, %d) = %v, want %v", tt.contentLength, tt.maxSizeMB, got, tt.want)
		}
	}
}

// ===== ValidateContentType =====

func TestValidateContentType(t *testing.T) {
	tests := []struct {
		contentType string
		expected    string
		want        bool
	}{
		{"application/json", "application/json", true},
		{"application/json; charset=utf-8", "application/json", true},
		{"APPLICATION/JSON", "application/json", true},
		{"text/html", "application/json", false},
		{"", "application/json", false},
		{"application/json", "", true},
	}
	for _, tt := range tests {
		got := ValidateContentType(tt.contentType, tt.expected)
		if got != tt.want {
			t.Errorf("ValidateContentType(%q, %q) = %v, want %v", tt.contentType, tt.expected, got, tt.want)
		}
	}
}
