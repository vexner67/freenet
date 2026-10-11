package rest_test

import (
	"net/http"
	"slices"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

func TestSessions(t *testing.T) {
	t.Parallel()

	t.Run("a sign-in and a refresh say where from", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		want := model.Client{IP: "203.0.113.9", UserAgent: "Mozilla/5.0 (test)"}

		if w := h.do("POST", "/api/v1/auth/verify", `{"email":"ann@example.com","code":"123456"}`, "User-Agent", want.UserAgent); w.Code != http.StatusOK {
			t.Fatalf("verify: status = %d", w.Code)
		}
		if h.auth.client != want {
			t.Fatalf("the client of a sign-in = %+v", h.auth.client)
		}

		h.auth.client = model.Client{}
		if w := h.do("POST", "/api/v1/auth/refresh", `{"refreshToken":"r1"}`, "User-Agent", want.UserAgent); w.Code != http.StatusOK {
			t.Fatalf("refresh: status = %d", w.Code)
		}
		if h.auth.client != want {
			t.Fatalf("the client of a refresh = %+v", h.auth.client)
		}
	})

	t.Run("the list marks the session that asks", func(t *testing.T) {
		t.Parallel()

		w := newHarness(rest.Config{}).do("GET", "/api/v1/me/sessions", "", "Authorization", "Bearer tok")
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", w.Code, w.Body)
		}

		type session struct {
			ID         string `json:"id"`
			CreatedAt  string `json:"createdAt"`
			LastUsedAt string `json:"lastUsedAt"`
			IP         string `json:"ip"`
			UserAgent  string `json:"userAgent"`
			Current    bool   `json:"current"`
		}
		got := decode[struct {
			Sessions []session `json:"sessions"`
		}](t, w).Sessions

		want := []session{
			{ID: "s-laptop", CreatedAt: "2026-10-01T12:00:00Z", LastUsedAt: "2026-10-01T13:00:00Z", IP: "203.0.113.9", UserAgent: "Mozilla/5.0 (laptop)", Current: true},
			{ID: "s-phone", CreatedAt: "2026-10-01T11:00:00Z", LastUsedAt: "2026-10-01T12:00:00Z"},
		}
		if !slices.Equal(got, want) {
			t.Fatalf("sessions = %+v", got)
		}
	})

	t.Run("one session is ended", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		if w := h.do("DELETE", "/api/v1/me/sessions/s-phone", "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent {
			t.Fatalf("status = %d: %s", w.Code, w.Body)
		}
		if !slices.Equal(h.auth.ended, []string{"s-phone"}) {
			t.Fatalf("ended = %v", h.auth.ended)
		}

		if w := h.do("DELETE", "/api/v1/me/sessions/s-other", "", "Authorization", "Bearer tok"); w.Code != http.StatusNotFound {
			t.Fatalf("someone else's session: status = %d", w.Code)
		}
	})

	t.Run("all the others are ended", func(t *testing.T) {
		t.Parallel()

		w := newHarness(rest.Config{}).do("DELETE", "/api/v1/me/sessions", "", "Authorization", "Bearer tok")
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", w.Code, w.Body)
		}
		if got := decode[map[string]int](t, w); got["ended"] != 1 {
			t.Fatalf("ended = %v", got)
		}
	})

	t.Run("sessions need a token", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		for _, req := range [][2]string{{"GET", "/api/v1/me/sessions"}, {"DELETE", "/api/v1/me/sessions"}, {"DELETE", "/api/v1/me/sessions/s-phone"}} {
			if w := h.do(req[0], req[1], ""); w.Code != http.StatusUnauthorized {
				t.Fatalf("%s %s without a token: status = %d", req[0], req[1], w.Code)
			}
		}
	})
}
