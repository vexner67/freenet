package rest_test

import (
	"net/http"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

func TestAbout(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})

	// Anyone may ask, without a token.
	w := h.do("GET", "/api/v1/about", "")
	got := decode[map[string]int](t, w)
	if w.Code != http.StatusOK || got["users"] != 12 || got["online"] != 3 || got["clubs"] != 5 || got["posts"] != 40 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}

	h.clubs.err = failure.New(failure.Unavailable, "service unavailable")
	if w = h.do("GET", "/api/v1/about", ""); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("a service down: %d", w.Code)
	}
}
