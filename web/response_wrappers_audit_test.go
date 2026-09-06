package web

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type auditDeadlineWriter struct {
	*httptest.ResponseRecorder
	deadline time.Time
}

func TestResponseWrapperFlushTracking(t *testing.T) {
	base := httptest.NewRecorder()
	audit := &auditResponseWriter{ResponseWriter: base, statusCode: 200}
	metrics := &statusRecorder{ResponseWriter: audit, statusCode: 200}
	if err := http.NewResponseController(metrics).Flush(); err != nil {
		t.Fatal(err)
	}
	metrics.WriteHeader(500)
	if !base.Flushed || base.Code != 200 || metrics.statusCode != 200 || audit.statusCode != 200 {
		t.Fatal("flush committed state lost")
	}
	// An unsupported deadline must still report its capability error.
	if err := http.NewResponseController(metrics).SetWriteDeadline(time.Now()); !errors.Is(err, http.ErrNotSupported) {
		t.Fatalf("unsupported deadline error=%v", err)
	}
}

func TestResponseWrapperInformationalThenFinal(t *testing.T) {
	states := make(chan [2]int, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		audit := &auditResponseWriter{ResponseWriter: w, statusCode: 200}
		metrics := &statusRecorder{ResponseWriter: audit, statusCode: 200}
		metrics.WriteHeader(103)
		metrics.WriteHeader(201)
		metrics.WriteHeader(500)
		_, _ = metrics.Write([]byte("fixture"))
		states <- [2]int{metrics.statusCode, audit.statusCode}
	}))
	defer server.Close()
	response, err := server.Client().Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != 201 || string(body) != "fixture" {
		t.Fatalf("response=%d body=%q err=%v", response.StatusCode, body, err)
	}
	if got := <-states; got != [2]int{201, 201} {
		t.Fatalf("recorded states=%v", got)
	}
}

func TestResponseWrapperSwitchingProtocolsIsFinal(t *testing.T) {
	base := httptest.NewRecorder()
	w := &statusRecorder{ResponseWriter: base, statusCode: 200}
	w.WriteHeader(101)
	w.WriteHeader(500)
	if base.Code != 101 || w.statusCode != 101 {
		t.Fatal("101 did not remain final")
	}
}

func (w *auditDeadlineWriter) SetWriteDeadline(deadline time.Time) error {
	w.deadline = deadline
	return nil
}

func TestAuditResponseWrapperDeadline(t *testing.T) {
	for _, layer := range []string{"direct-control", "metrics", "audit", "both"} {
		t.Run(layer, func(t *testing.T) {
			base := &auditDeadlineWriter{ResponseRecorder: httptest.NewRecorder()}
			var writer http.ResponseWriter = base
			if layer == "audit" || layer == "both" {
				writer = &auditResponseWriter{ResponseWriter: writer, statusCode: 200}
			}
			if layer == "metrics" || layer == "both" {
				writer = &statusRecorder{ResponseWriter: writer, statusCode: 200}
			}
			deadline := time.Now().Add(time.Minute)
			if err := http.NewResponseController(writer).SetWriteDeadline(deadline); err != nil {
				t.Fatalf("deadline capability lost: %v", err)
			}
			if !base.deadline.Equal(deadline) {
				t.Fatal("deadline not delivered to underlying writer")
			}
		})
	}
}

func TestAuditResponseWrapperCommittedStatus(t *testing.T) {
	for _, layer := range []string{"metrics", "audit"} {
		for _, implicit := range []bool{false, true} {
			name := layer + "/explicit"
			if implicit {
				name = layer + "/implicit"
			}
			t.Run(name, func(t *testing.T) {
				base := httptest.NewRecorder()
				var writer http.ResponseWriter
				var recorded func() int
				if layer == "metrics" {
					w := &statusRecorder{ResponseWriter: base, statusCode: 200}
					writer, recorded = w, func() int { return w.statusCode }
				} else {
					w := &auditResponseWriter{ResponseWriter: base, statusCode: 200}
					writer, recorded = w, func() int { return w.statusCode }
				}
				if implicit {
					_, _ = writer.Write([]byte("payload"))
				} else {
					writer.WriteHeader(404)
				}
				writer.WriteHeader(500)
				if recorded() != base.Code {
					t.Fatalf("recorded=%d actual=%d", recorded(), base.Code)
				}
			})
		}
	}
}
