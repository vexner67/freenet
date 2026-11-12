package rest_test

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

// The sandbox runs the HTML of people in an opaque origin, with no
// network, framed by the site alone.
func TestSandbox(t *testing.T) {
	t.Parallel()

	handler := mustServer(
		rest.Config{RequestTimeout: time.Second, WebDir: webDir(t)},
		rest.Backends{Auth: &fakeAuth{}, Users: &fakeUsers{}, Posts: &fakePosts{}, Media: &fakeMedia{}, Notifications: &fakeNotifications{}, Messages: &fakeMessages{}, Clubs: &fakeClubs{}},
		prometheus.NewRegistry(),
	).Handler()

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/sandbox", nil))

	if w.Code != 200 || !strings.Contains(w.Body.String(), "freenet-sandbox-ready") {
		t.Fatalf("GET /sandbox = %d %q", w.Code, w.Body.String())
	}
	csp := w.Header().Get("Content-Security-Policy")
	for _, want := range []string{"sandbox allow-scripts allow-pointer-lock;", "default-src 'none'", "frame-ancestors 'self'", "form-action 'none'"} {
		if !strings.Contains(csp, want) {
			t.Errorf("CSP %q lacks %q", csp, want)
		}
	}
	for _, never := range []string{"allow-same-origin", "allow-top-navigation", "allow-popups", "allow-forms", "connect-src"} {
		if strings.Contains(csp, never) {
			t.Errorf("CSP %q has %q", csp, never)
		}
	}
	if got := w.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
}
