package requestid

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"sync/atomic"
	"time"
)

const HeaderName = "X-Request-Id"

type contextKey struct{}

var counter uint64

// Normalize trims and limits request id length.
func Normalize(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if len(value) > 128 {
		return value[:128]
	}
	return value
}

// New generates a lightweight request id without external dependency.
func New() string {
	buf := make([]byte, 4)
	_, _ = rand.Read(buf)
	n := atomic.AddUint64(&counter, 1)
	return "rid-" + time.Now().UTC().Format("20060102T150405.000") + "-" + hex.EncodeToString(buf) + "-" + base36(n)
}

func base36(v uint64) string {
	const chars = "0123456789abcdefghijklmnopqrstuvwxyz"
	if v == 0 {
		return "0"
	}
	out := make([]byte, 0, 13)
	for v > 0 {
		out = append([]byte{chars[v%36]}, out...)
		v /= 36
	}
	return string(out)
}

func WithContext(ctx context.Context, id string) context.Context {
	id = Normalize(id)
	if id == "" {
		id = New()
	}
	return context.WithValue(ctx, contextKey{}, id)
}

func FromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v := ctx.Value(contextKey{}); v != nil {
		if id, ok := v.(string); ok {
			return Normalize(id)
		}
	}
	return ""
}
