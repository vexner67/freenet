package rest_test

import (
	"net/http"
	"slices"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/rest"
)

func TestUpdateStatus(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	h.users.profile.Nickname = "ann"

	w := h.do("PUT", "/api/v1/me/status", `{"status":"Привет"}`, "Authorization", "Bearer tok")
	got := decode[struct {
		Status       string   `json:"status"`
		Completeness int      `json:"completeness"`
		Unfilled     []string `json:"unfilled"`
	}](t, w)
	if w.Code != http.StatusOK || got.Status != "Привет" || got.Completeness != 85 || !slices.Equal(got.Unfilled, []string{"telegram"}) {
		t.Fatalf("%d %+v", w.Code, got)
	}

	if w = h.do("PUT", "/api/v1/me/status", `{"status":"x"}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("without a token: %d", w.Code)
	}
}

func TestWallTabs(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	for _, tab := range []string{"owner", "others"} {
		if w := h.do("GET", "/api/v1/users/ann/posts?type="+tab, ""); w.Code != http.StatusOK || h.posts.wallFilter != tab {
			t.Fatalf("%s: %d, asked for %q", tab, w.Code, h.posts.wallFilter)
		}
	}
	if w := h.do("GET", "/api/v1/clubs/cats/posts?type=others", ""); w.Code != http.StatusOK || h.posts.wallFilter != "others" {
		t.Fatalf("the club's: %d, asked for %q", w.Code, h.posts.wallFilter)
	}
	if w := h.do("GET", "/api/v1/users/ann/posts", ""); w.Code != http.StatusOK || h.posts.wallFilter != "" {
		t.Fatalf("all: %d, asked for %q", w.Code, h.posts.wallFilter)
	}
}
