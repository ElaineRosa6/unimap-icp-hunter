package requestid

import (
	"context"
	"strings"
	"testing"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty string", "", ""},
		{"whitespace only", "   ", ""},
		{"normal id", "abc-123", "abc-123"},
		{"leading/trailing spaces", "  abc  ", "abc"},
		{"long string gets truncated", strings.Repeat("a", 200), strings.Repeat("a", 128)},
		{"exactly 128 chars", strings.Repeat("x", 128), strings.Repeat("x", 128)},
		{"129 chars gets truncated to 128", strings.Repeat("y", 129), strings.Repeat("y", 128)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Normalize(tt.input)
			if got != tt.want {
				t.Errorf("Normalize(%q) = %q (len=%d), want %q (len=%d)", tt.input, got, len(got), tt.want, len(tt.want))
			}
		})
	}
}

func TestNew(t *testing.T) {
	t.Run("generates non-empty id", func(t *testing.T) {
		id := New()
		if id == "" {
			t.Fatal("expected non-empty id")
		}
	})

	t.Run("id has rid- prefix", func(t *testing.T) {
		id := New()
		if !strings.HasPrefix(id, "rid-") {
			t.Errorf("expected id to start with 'rid-', got %q", id)
		}
	})

	t.Run("ids are unique", func(t *testing.T) {
		ids := make(map[string]bool)
		for i := 0; i < 100; i++ {
			id := New()
			if ids[id] {
				t.Fatalf("duplicate id generated: %s", id)
			}
			ids[id] = true
		}
	})

	t.Run("id format contains dashes", func(t *testing.T) {
		id := New()
		parts := strings.Split(id, "-")
		if len(parts) < 3 {
			t.Errorf("expected id with at least 3 dash-separated parts, got %d: %s", len(parts), id)
		}
	})
}

func TestBase36(t *testing.T) {
	tests := []struct {
		input uint64
		want  string
	}{
		{0, "0"},
		{1, "1"},
		{10, "a"},
		{35, "z"},
		{36, "10"},
		{100, "2s"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := base36(tt.input)
			if got != tt.want {
				t.Errorf("base36(%d) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestWithContext(t *testing.T) {
	t.Run("stores custom id", func(t *testing.T) {
		ctx := context.Background()
		newCtx := WithContext(ctx, "my-custom-id")
		got := FromContext(newCtx)
		if got != "my-custom-id" {
			t.Errorf("expected 'my-custom-id', got %q", got)
		}
	})

	t.Run("generates id for empty input", func(t *testing.T) {
		ctx := context.Background()
		newCtx := WithContext(ctx, "")
		got := FromContext(newCtx)
		if got == "" {
			t.Error("expected auto-generated id, got empty")
		}
		if !strings.HasPrefix(got, "rid-") {
			t.Errorf("expected rid- prefix, got %q", got)
		}
	})

	t.Run("generates id for whitespace input", func(t *testing.T) {
		ctx := context.Background()
		newCtx := WithContext(ctx, "   ")
		got := FromContext(newCtx)
		if got == "" {
			t.Error("expected auto-generated id for whitespace input")
		}
	})

	t.Run("normalizes long id", func(t *testing.T) {
		ctx := context.Background()
		longID := strings.Repeat("a", 200)
		newCtx := WithContext(ctx, longID)
		got := FromContext(newCtx)
		if len(got) > 128 {
			t.Errorf("expected id to be truncated to 128, got length %d", len(got))
		}
	})
}

func TestFromContext(t *testing.T) {
	t.Run("returns empty for nil context", func(t *testing.T) {
		// FromContext explicitly handles nil context and returns ""
		if got := FromContext(nil); got != "" {
			t.Errorf("expected empty string for nil context, got %q", got)
		}
	})

	t.Run("returns empty for context without id", func(t *testing.T) {
		ctx := context.Background()
		if got := FromContext(ctx); got != "" {
			t.Errorf("expected empty string, got %q", got)
		}
	})

	t.Run("roundtrips id through context", func(t *testing.T) {
		ctx := context.Background()
		want := "test-request-id-123"
		newCtx := WithContext(ctx, want)
		got := FromContext(newCtx)
		if got != want {
			t.Errorf("expected %q, got %q", want, got)
		}
	})
}

func TestHeaderName(t *testing.T) {
	if HeaderName != "X-Request-Id" {
		t.Errorf("expected HeaderName 'X-Request-Id', got %q", HeaderName)
	}
}
