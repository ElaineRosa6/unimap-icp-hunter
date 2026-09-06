package utils

import (
	"crypto/md5"
	"encoding/hex"
	"testing"
)

func TestCacheKeyPreservesQueryIdentity(t *testing.T) {
	pairs := [][2]string{
		{`title="Admin"`, `title="admin"`},
		{`body="hello  world"`, `body="hello world"`},
		{"body=\"hello\tworld\"", "body=\"hello world\""},
		{`path=/Admin`, `path=/admin`},
		{` title="x"`, `title="x"`},
	}
	for _, pair := range pairs {
		t.Run(pair[0], func(t *testing.T) {
			a := GenerateCacheKey("fixture", pair[0], 1, 10)
			if a != GenerateCacheKey("fixture", pair[0], 1, 10) {
				t.Fatal("identical query is unstable")
			}
			if a == GenerateCacheKey("fixture", pair[1], 1, 10) {
				t.Fatalf("distinct query bytes share key: %q / %q", pair[0], pair[1])
			}
		})
	}
	if GenerateCacheKey("fixture:a", "b", 1, 10) == GenerateCacheKey("fixture", "a:b", 1, 10) {
		t.Fatal("engine/query boundary collision")
	}
}

func TestCacheKeyExcludesLegacyEntries(t *testing.T) {
	old := md5.Sum([]byte("fixture:title=\"admin\":1:10"))
	if GenerateCacheKey("fixture", `title="admin"`, 1, 10) == hex.EncodeToString(old[:]) {
		t.Fatal("legacy entry remains reachable")
	}
}
