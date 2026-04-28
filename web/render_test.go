package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"text/template"
)

func TestRenderTemplate_TemplateNotFound(t *testing.T) {
	tmpl, err := template.New("").Parse("hello")
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{templates: tmpl}
	rec := httptest.NewRecorder()
	if s.renderTemplate(rec, http.StatusInternalServerError, "nonexistent", nil) {
		t.Fatal("expected false for nonexistent template")
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestRenderTemplate_Success(t *testing.T) {
	tmpl, err := template.New("index").Parse("hello {{.}}")
	if err != nil {
		t.Fatal(err)
	}
	tmplSet := template.Must(template.New("").AddParseTree("index", tmpl.Tree))
	s := &Server{templates: tmplSet}
	rec := httptest.NewRecorder()
	if !s.renderTemplate(rec, http.StatusOK, "index", "world") {
		t.Fatal("expected true for valid template")
	}
	if rec.Body.String() != "hello world" {
		t.Fatalf("expected 'hello world', got %q", rec.Body.String())
	}
}
