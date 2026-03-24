package main

import (
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"strings"
	"testing"
)

func TestSplitCSVText(t *testing.T) {
	got := splitCSVText(" https://a.com,https://b.com\nhttps://c.com ,, ")
	if len(got) != 3 {
		t.Fatalf("expected 3 items, got %d", len(got))
	}
	if got[0] != "https://a.com" || got[1] != "https://b.com" || got[2] != "https://c.com" {
		t.Fatalf("unexpected values: %+v", got)
	}
}

func TestMaxInt(t *testing.T) {
	if maxInt(1, 2) != 2 {
		t.Fatalf("expected 2")
	}
	if maxInt(7, 3) != 7 {
		t.Fatalf("expected 7")
	}
}

func TestDoJSONRequestSuccessAndFailure(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ok" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"hello":"world"}`))
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`bad request`))
	}))
	defer ts.Close()

	var okResp map[string]string
	if err := doJSONRequest(ts.URL, "/ok", 5, map[string]string{"a": "b"}, &okResp); err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if okResp["hello"] != "world" {
		t.Fatalf("unexpected response: %+v", okResp)
	}

	var failResp map[string]string
	err := doJSONRequest(ts.URL, "/fail", 5, map[string]string{"a": "b"}, &failResp)
	if err == nil {
		t.Fatalf("expected non-2xx error")
	}
	if !strings.Contains(err.Error(), "status=400") {
		t.Fatalf("expected status in error, got: %v", err)
	}
}

func TestDoFormRequestSuccessAndFailure(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ok" {
			_ = r.ParseForm()
			if r.FormValue("query") != "app=\"nginx\"" {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`invalid form`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"query":"app=\"nginx\""}`))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`boom`))
	}))
	defer ts.Close()

	values := neturl.Values{}
	values.Set("query", "app=\"nginx\"")

	var okResp map[string]string
	if err := doFormRequest(ts.URL, "/ok", 5, values, &okResp); err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if okResp["query"] != "app=\"nginx\"" {
		t.Fatalf("unexpected response: %+v", okResp)
	}

	var failResp map[string]string
	err := doFormRequest(ts.URL, "/fail", 5, values, &failResp)
	if err == nil {
		t.Fatalf("expected non-2xx error")
	}
	if !strings.Contains(err.Error(), "status=500") {
		t.Fatalf("expected status in error, got: %v", err)
	}
}
