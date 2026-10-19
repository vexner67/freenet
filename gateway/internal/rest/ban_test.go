package rest_test

import (
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

func TestBan(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	auth := []string{"Authorization", "Bearer tok"}
	if w := h.do("PUT", "/api/v1/users/ann/ban", `{"reason":"spam","until":"2026-11-19T12:00:00+03:00"}`, auth...); w.Code != http.StatusNoContent {
		t.Fatalf("PUT for a while: %d %s", w.Code, w.Body)
	}
	if w := h.do("PUT", "/api/v1/users/ann/ban", `{"reason":"spam"}`, auth...); w.Code != http.StatusNoContent {
		t.Fatalf("PUT for good: %d %s", w.Code, w.Body)
	}
	if w := h.do("DELETE", "/api/v1/users/ann/ban", "", auth...); w.Code != http.StatusNoContent {
		t.Fatalf("DELETE: %d %s", w.Code, w.Body)
	}
	if want := []string{"tok:u1:spam:2026-11-19T09:00:00Z", "tok:u1:spam:", "tok:u1:-"}; !slices.Equal(h.users.bans, want) {
		t.Fatalf("asked %v, want %v", h.users.bans, want)
	}

	if w := h.do("PUT", "/api/v1/users/ann/ban", `{"reason":"spam","until":"tomorrow"}`, auth...); w.Code != http.StatusBadRequest {
		t.Fatalf("a bad time: %d", w.Code)
	}
	if w := h.do("PUT", "/api/v1/users/nobody/ban", `{"reason":"spam"}`, auth...); w.Code != http.StatusNotFound {
		t.Fatalf("an unknown page: %d", w.Code)
	}
	if w := h.do("DELETE", "/api/v1/users/ann/ban", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("without a token: %d", w.Code)
	}

	h = newHarness(rest.Config{})
	h.users.err = failure.New(failure.PermissionDenied, "permission denied")
	if w := h.do("PUT", "/api/v1/users/ann/ban", `{"reason":"spam"}`, auth...); w.Code != http.StatusForbidden {
		t.Fatalf("by a stranger: %d", w.Code)
	}
}

func TestBan_Shown(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})

	got := decode[map[string]any](t, h.do("GET", "/api/v1/users/boris", ""))
	ban, ok := got["ban"].(map[string]any)
	if !ok || ban["reason"] != "spam" || ban["bannedAt"] != "2026-10-19T12:00:00Z" || ban["until"] != nil {
		t.Fatalf("the blocked profile: %v", got)
	}
	if got = decode[map[string]any](t, h.do("GET", "/api/v1/users/ann", "")); got["ban"] != nil {
		t.Fatalf("a profile that is not blocked: %v", got["ban"])
	}

	if w := h.do("GET", "/api/v1/users?banned=true", ""); w.Code != http.StatusOK || h.users.search != (model.UserQuery{Banned: true}) {
		t.Fatalf("the blocked pages: %d, asked %+v", w.Code, h.users.search)
	}

	// The blocked owner reads why from their account.
	h.users.ban = &model.Ban{Reason: "spam", At: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC), Until: time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)}
	me := decode[map[string]any](t, h.do("GET", "/api/v1/me", "", "Authorization", "Bearer tok"))
	if ban, ok = me["ban"].(map[string]any); !ok || ban["until"] != "2026-11-19T12:00:00Z" || me["pinnedClubs"] != nil {
		t.Fatalf("the account: %v", me)
	}
}
