package rest_test

import (
	"net/http"
	"slices"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

func TestVerification(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	if w := h.do("PUT", "/api/v1/users/ann/verification", "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent {
		t.Fatalf("PUT: %d %s", w.Code, w.Body)
	}
	if w := h.do("DELETE", "/api/v1/users/ann/verification", "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent {
		t.Fatalf("DELETE: %d %s", w.Code, w.Body)
	}
	if want := []string{"tok:u1:true", "tok:u1:false"}; !slices.Equal(h.users.verified, want) {
		t.Fatalf("asked %v, want %v", h.users.verified, want)
	}

	if w := h.do("PUT", "/api/v1/users/nobody/verification", "", "Authorization", "Bearer tok"); w.Code != http.StatusNotFound {
		t.Fatalf("an unknown page: %d", w.Code)
	}
	if w := h.do("PUT", "/api/v1/users/ann/verification", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("without a token: %d", w.Code)
	}

	// Only the administrators give it.
	h = newHarness(rest.Config{})
	h.users.err = failure.New(failure.PermissionDenied, "permission denied")
	if w := h.do("PUT", "/api/v1/users/ann/verification", "", "Authorization", "Bearer tok"); w.Code != http.StatusForbidden {
		t.Fatalf("by a stranger: %d", w.Code)
	}
}

func TestVerification_Shown(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})

	w := h.do("GET", "/api/v1/users/vera", "")
	if got := decode[map[string]any](t, w); w.Code != http.StatusOK || got["verified"] != true {
		t.Fatalf("the profile: %d %s", w.Code, w.Body)
	}
	if got := decode[map[string]any](t, h.do("GET", "/api/v1/users/ann", "")); got["verified"] != false {
		t.Fatalf("an unverified profile: %v", got["verified"])
	}

	if w = h.do("GET", "/api/v1/users?verified=true", ""); w.Code != http.StatusOK || h.users.search != (model.UserQuery{Verified: true}) {
		t.Fatalf("the verified pages: %d, asked %+v", w.Code, h.users.search)
	}

	if got := decode[map[string]int](t, h.do("GET", "/api/v1/about", "")); got["verified"] != 2 {
		t.Fatalf("about: %v", got)
	}
}
