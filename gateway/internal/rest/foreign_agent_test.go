package rest_test

import (
	"net/http"
	"slices"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

func TestForeignAgent(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	if w := h.do("PUT", "/api/v1/users/ann/foreign-agent", "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent {
		t.Fatalf("PUT: %d %s", w.Code, w.Body)
	}
	if w := h.do("DELETE", "/api/v1/users/ann/foreign-agent", "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent {
		t.Fatalf("DELETE: %d %s", w.Code, w.Body)
	}
	if want := []string{"tok:u1:true", "tok:u1:false"}; !slices.Equal(h.users.agents, want) {
		t.Fatalf("asked %v, want %v", h.users.agents, want)
	}

	if w := h.do("PUT", "/api/v1/users/nobody/foreign-agent", "", "Authorization", "Bearer tok"); w.Code != http.StatusNotFound {
		t.Fatalf("an unknown page: %d", w.Code)
	}
	if w := h.do("PUT", "/api/v1/users/ann/foreign-agent", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("without a token: %d", w.Code)
	}

	// Only the administrators put it.
	h = newHarness(rest.Config{})
	h.users.err = failure.New(failure.PermissionDenied, "permission denied")
	if w := h.do("PUT", "/api/v1/users/ann/foreign-agent", "", "Authorization", "Bearer tok"); w.Code != http.StatusForbidden {
		t.Fatalf("by a stranger: %d", w.Code)
	}
}

func TestForeignAgent_Shown(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	if got := decode[map[string]any](t, h.do("GET", "/api/v1/users/vera", "")); got["foreignAgent"] != true {
		t.Fatalf("the profile: %v", got["foreignAgent"])
	}
	if got := decode[map[string]any](t, h.do("GET", "/api/v1/users/ann", "")); got["foreignAgent"] != false {
		t.Fatalf("a page without the plaque: %v", got["foreignAgent"])
	}
}
