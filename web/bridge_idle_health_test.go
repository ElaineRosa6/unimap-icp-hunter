package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/unimap/project/internal/screenshot"
)

func TestBridgeIdlePollRefreshesHealthOnlyAfterValidAuth(t *testing.T) {
	for _, tc := range []struct {
		name, token string
		remote      bool
		status      int
	}{
		{"valid idle poll", "tok-test", false, http.StatusOK},
		{"invalid token", "invalid-fixture", false, http.StatusUnauthorized},
		{"nonloopback", "tok-test", true, http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newBridgeTestServer(false)
			old := time.Now().Add(-10 * time.Minute).Unix()
			s.bridge.LastTaskPullAt = old
			s.bridge.LastSeen = map[string]int64{"tok-test": time.Now().Unix()}
			svc := screenshot.NewBridgeService(s.bridge.Mock, 1, time.Second)
			svc.Start(context.Background())
			defer svc.Stop()
			checker := &screenshot.ExtensionHealthChecker{BridgeService: svc, LiveClient: func() bool { return s.activeBridgeLiveTokens() > 0 }, LastActivity: func() int64 { return s.bridge.LastTaskPullAt }, RecentActivityCutoff: 5 * time.Minute}
			if healthy, err := checker.Check(context.Background()); err != nil || healthy {
				t.Fatalf("stale activity healthy=%v err=%v", healthy, err)
			}
			req := httptest.NewRequest(http.MethodGet, "/api/v1/screenshot/bridge/tasks/next", nil)
			setLoopbackBridgeRequest(req)
			if tc.remote {
				req.RemoteAddr = "203.0.113.8:12345"
				req.Host = "example.test"
			}
			req.Header.Set("Authorization", "Bearer "+tc.token)
			rec := httptest.NewRecorder()
			s.handleScreenshotBridgeTaskNext(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("status=%d want=%d", rec.Code, tc.status)
			}
			healthy, err := checker.Check(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			want := tc.status == http.StatusOK
			if healthy != want {
				t.Fatalf("after idle poll healthy=%v want=%v; activity=%d", healthy, want, s.bridge.LastTaskPullAt)
			}
			if !want && s.bridge.LastTaskPullAt != old {
				t.Fatal("rejected request refreshed activity")
			}
		})
	}
}
