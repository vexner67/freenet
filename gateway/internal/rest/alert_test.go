package rest_test

import (
	"net/http"
	"slices"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

func TestAlert(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	if w := h.do("PUT", "/api/v1/users/ann/alert", `{"alert":"Страница под наблюдением"}`, "Authorization", "Bearer tok"); w.Code != http.StatusNoContent {
		t.Fatalf("PUT: %d %s", w.Code, w.Body)
	}
	if w := h.do("DELETE", "/api/v1/users/ann/alert", "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent {
		t.Fatalf("DELETE: %d %s", w.Code, w.Body)
	}
	if want := []string{"tok:u1:Страница под наблюдением", "tok:u1:"}; !slices.Equal(h.users.alerts, want) {
		t.Fatalf("asked %v, want %v", h.users.alerts, want)
	}

	if w := h.do("PUT", "/api/v1/users/ann/alert", `{"alert":`, "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest {
		t.Fatalf("a broken body: %d", w.Code)
	}
	if w := h.do("PUT", "/api/v1/users/nobody/alert", `{"alert":"x"}`, "Authorization", "Bearer tok"); w.Code != http.StatusNotFound {
		t.Fatalf("an unknown page: %d", w.Code)
	}
	if w := h.do("PUT", "/api/v1/users/ann/alert", `{"alert":"x"}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("without a token: %d", w.Code)
	}

	// Only the administrators put it.
	h = newHarness(rest.Config{})
	h.users.err = failure.New(failure.PermissionDenied, "permission denied")
	if w := h.do("PUT", "/api/v1/users/ann/alert", `{"alert":"x"}`, "Authorization", "Bearer tok"); w.Code != http.StatusForbidden {
		t.Fatalf("by a stranger: %d", w.Code)
	}
}

func TestAlert_Shown(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	if got := decode[map[string]any](t, h.do("GET", "/api/v1/users/vera", "")); got["alert"] != "Метка" {
		t.Fatalf("the profile: %v", got["alert"])
	}
	if got := decode[map[string]any](t, h.do("GET", "/api/v1/users/ann", "")); got["alert"] != "" {
		t.Fatalf("a page without the label: %v", got["alert"])
	}
}
