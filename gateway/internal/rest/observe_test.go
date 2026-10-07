package rest_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/vexner67/freenet/gateway/internal/rest"
	"github.com/vexner67/freenet/pkg/reqid"
)

func TestRequestID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		trustProxy bool
		header     string
		want       string // "" = a new ID
	}{
		{name: "made", trustProxy: false},
		{name: "client's ignored", trustProxy: false, header: "from-client"},
		{name: "proxy's kept", trustProxy: true, header: "from-proxy", want: "from-proxy"},
		{name: "invalid replaced", trustProxy: true, header: "bad id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(rest.Config{TrustForwardedFor: tt.trustProxy})

			var headers []string
			if tt.header != "" {
				headers = []string{reqid.Header, tt.header}
			}

			w := h.do(http.MethodPost, "/api/v1/auth/code", `{"email":"ann@example.com"}`, headers...)
			if w.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want 204: %s", w.Code, w.Body)
			}

			got := w.Header().Get(reqid.Header)
			switch {
			case tt.want != "" && got != tt.want:
				t.Errorf("response ID = %q, want %q", got, tt.want)
			case tt.want == "" && (!reqid.Valid(got) || got == tt.header):
				t.Errorf("response ID = %q, want a new one", got)
			}

			if h.auth.requestID != got {
				t.Errorf("backend got ID %q, response says %q", h.auth.requestID, got)
			}
		})
	}
}

func TestCORS_ExposesRequestID(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{AllowedOrigins: []string{"https://app.example"}})

	w := h.do(http.MethodGet, "/healthz", "", "Origin", "https://app.example")
	if got := w.Header().Get("Access-Control-Expose-Headers"); got != reqid.Header {
		t.Errorf("Access-Control-Expose-Headers = %q, want %q", got, reqid.Header)
	}
}

// Requests are counted by route, never by the nickname or ID in the path.
func TestMetrics(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})

	h.do(http.MethodGet, "/api/v1/users/ann", "")
	h.do(http.MethodGet, "/api/v1/users/bob", "")
	h.do(http.MethodGet, "/no/such/thing", "")

	want := `
# HELP http_requests_total HTTP requests completed, by route and status code.
# TYPE http_requests_total counter
http_requests_total{code="200",method="GET",route="/api/v1/users/{nickname}"} 2
http_requests_total{code="404",method="GET",route="/"} 1
`
	if err := testutil.GatherAndCompare(h.metrics, strings.NewReader(want), "http_requests_total"); err != nil {
		t.Error(err)
	}

	if n, err := testutil.GatherAndCount(h.metrics, "http_request_duration_seconds"); err != nil || n != 2 {
		t.Errorf("duration series = %d (%v), want 2", n, err)
	}
}
