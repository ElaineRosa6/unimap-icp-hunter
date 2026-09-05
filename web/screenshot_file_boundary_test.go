package web

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/unimap/project/internal/config"
)

func screenshotFileFixture(t *testing.T) (*Server, string, string) {
	t.Helper()
	root := t.TempDir()
	base, outside := filepath.Join(root, "screenshots"), filepath.Join(root, "outside")
	for _, dir := range []string{base, outside} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	cfg := &config.Config{}
	cfg.Screenshot.BaseDir = base
	return &Server{config: cfg}, base, outside
}

func screenshotFileRequest(s *Server, path string, headers map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, "/screenshots/"+path, nil)
	r.Host = "localhost:8448"
	r.Header.Set("Origin", "http://localhost:8448")
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	s.handleScreenshotFile(w, r)
	return w
}

func TestScreenshotFileRejectsOutsideLinks(t *testing.T) {
	for _, kind := range []string{"file-absolute", "file-relative", "directory-absolute", "directory-relative"} {
		t.Run(kind, func(t *testing.T) {
			s, base, outside := screenshotFileFixture(t)
			marker := []byte("outside fixture must not be served")
			target := filepath.Join(outside, "private.png")
			if err := os.WriteFile(target, marker, 0600); err != nil {
				t.Fatal(err)
			}
			link, requestPath := filepath.Join(base, "linked.png"), "linked.png"
			switch kind {
			case "file-relative":
				target = filepath.Join("..", "outside", "private.png")
			case "directory-absolute":
				target, link, requestPath = outside, filepath.Join(base, "linked"), "linked/private.png"
			case "directory-relative":
				target, link, requestPath = filepath.Join("..", "outside"), filepath.Join(base, "linked"), "linked/private.png"
			}
			if err := os.Symlink(target, link); err != nil {
				t.Fatal(err)
			}
			w := screenshotFileRequest(s, requestPath, nil)
			if w.Code != http.StatusNotFound || bytes.Contains(w.Body.Bytes(), marker) {
				t.Fatalf("outside file exposed: status=%d body=%q", w.Code, w.Body.String())
			}
		})
	}
}

func TestScreenshotFileRegularRangeAndConditional(t *testing.T) {
	s, base, _ := screenshotFileFixture(t)
	data := []byte("\x89PNG\r\n\x1a\nfixture image data")
	if err := os.WriteFile(filepath.Join(base, "image.png"), data, 0600); err != nil {
		t.Fatal(err)
	}
	w := screenshotFileRequest(s, "image.png", nil)
	if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), data) || w.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("regular file: %d %q", w.Code, w.Body.String())
	}
	if w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("Cache-Control") != "private, max-age=300" {
		t.Fatal("missing response protections")
	}
	part := screenshotFileRequest(s, "image.png", map[string]string{"Range": "bytes=0-3"})
	if part.Code != http.StatusPartialContent || !bytes.Equal(part.Body.Bytes(), data[:4]) {
		t.Fatalf("range: %d %q", part.Code, part.Body.String())
	}
	since := w.Header().Get("Last-Modified")
	if since == "" {
		t.Fatal("missing Last-Modified")
	}
	cached := screenshotFileRequest(s, "image.png", map[string]string{"If-Modified-Since": since})
	if cached.Code != http.StatusNotModified || cached.Body.Len() != 0 {
		t.Fatalf("conditional: %d", cached.Code)
	}
	if err := os.Symlink("image.png", filepath.Join(base, "internal.png")); err != nil {
		t.Fatal(err)
	}
	linked := screenshotFileRequest(s, "internal.png", nil)
	if linked.Code != 200 || !bytes.Equal(linked.Body.Bytes(), data) {
		t.Fatalf("in-root link: %d", linked.Code)
	}
}

func TestScreenshotFileRejectsDirectoryWithImageSuffix(t *testing.T) {
	s, base, _ := screenshotFileFixture(t)
	if err := os.Mkdir(filepath.Join(base, "directory.png"), 0700); err != nil {
		t.Fatal(err)
	}
	w := screenshotFileRequest(s, "directory.png", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("directory accepted: %d", w.Code)
	}
}
