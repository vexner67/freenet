package rest_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/vexner67/freenet/gateway/internal/rest"
)

type postponedBody struct {
	ID        string              `json:"id"`
	Author    map[string]any      `json:"author"`
	Text      string              `json:"text"`
	TextCut   bool                `json:"textCut"`
	Photos    []map[string]string `json:"photos"`
	NSFW      bool                `json:"nsfw"`
	FromClub  bool                `json:"fromClub"`
	Signed    bool                `json:"signed"`
	PublishAt string              `json:"publishAt"`
}

func TestPostponeMyPost(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	h.users.profile = ann

	body := `{"text":"Завтра в 10","photos":["ph1"],"nsfw":true,"signed":true,"publishAt":"2026-10-20T09:00:00+03:00"}`
	w := h.do("POST", "/api/v1/me/postponed", body, "Authorization", "Bearer tok")
	got := decode[postponedBody](t, w)
	if w.Code != http.StatusCreated || got.ID != "pp1" || got.Author["nickname"] != "ann" || got.Text != "Завтра в 10" || !got.NSFW ||
		got.FromClub || got.Signed || got.PublishAt != "2026-10-20T06:00:00Z" || len(got.Photos) != 1 {
		t.Fatalf("postpone: %d %+v", w.Code, got)
	}
	if in := h.posts.postponed; in.WallID != "" || in.FromWall || in.Signed || !in.PublishAt.Equal(time.Date(2026, 10, 20, 6, 0, 0, 0, time.UTC)) {
		t.Fatalf("postponed = %+v", in)
	}

	for _, bad := range []string{`{"text":"x"}`, `{"text":"x","publishAt":"завтра"}`} {
		if w = h.do("POST", "/api/v1/me/postponed", bad, "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest {
			t.Fatalf("postpone %s: %d", bad, w.Code)
		}
	}
	if w = h.do("POST", "/api/v1/me/postponed", body); w.Code != http.StatusUnauthorized {
		t.Fatalf("postpone without a token: %d", w.Code)
	}

	w = h.do("GET", "/api/v1/me/postponed", "", "Authorization", "Bearer tok")
	list := decode[struct {
		Postponed []postponedBody `json:"postponed"`
	}](t, w)
	// Cat's page is deleted: her post is left out.
	if w.Code != http.StatusOK || len(list.Postponed) != 1 || list.Postponed[0].Author["nickname"] != "bob" || !list.Postponed[0].TextCut ||
		h.posts.postponedOf != "u1" {
		t.Fatalf("list: %d %+v, of %q", w.Code, list, h.posts.postponedOf)
	}
}

func TestPostponeClubPost(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	h.users.profile = ann
	h.clubs.role = "member"

	body := `{"text":"Встреча","signed":true,"publishAt":"2026-10-20T09:00:00Z"}`
	if w := h.do("POST", "/api/v1/clubs/cats/postponed", body, "Authorization", "Bearer tok"); w.Code != http.StatusForbidden {
		t.Fatalf("a member postpones for the club: %d", w.Code)
	}

	h.clubs.role = "admin"
	w := h.do("POST", "/api/v1/clubs/cats/postponed", body, "Authorization", "Bearer tok")
	got := decode[postponedBody](t, w)
	if w.Code != http.StatusCreated || !got.FromClub || !got.Signed || h.posts.postponed.WallID != "k1" || !h.clubs.fromClub {
		t.Fatalf("an admin postpones: %d %+v", w.Code, got)
	}

	if w = h.do("GET", "/api/v1/clubs/cats/postponed", "", "Authorization", "Bearer tok"); w.Code != http.StatusOK || h.posts.postponedOf != "k1" {
		t.Fatalf("the club's list: %d, of %q", w.Code, h.posts.postponedOf)
	}
}

func TestPostponed(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	h.users.profile = ann

	w := h.do("GET", "/api/v1/postponed/pp2", "", "Authorization", "Bearer tok")
	if got := decode[postponedBody](t, w); w.Code != http.StatusOK || got.Text != "от Петра целиком" || got.Author["nickname"] != "bob" {
		t.Fatalf("get: %d %+v", w.Code, got)
	}
	if w = h.do("GET", "/api/v1/postponed/pp9", "", "Authorization", "Bearer tok"); w.Code != http.StatusNotFound {
		t.Fatalf("get unknown: %d", w.Code)
	}

	w = h.do("PATCH", "/api/v1/postponed/pp2", `{"text":"Послезавтра","nsfw":true,"publishAt":"2026-10-21T09:00:00Z"}`, "Authorization", "Bearer tok")
	got := decode[postponedBody](t, w)
	if w.Code != http.StatusOK || got.Text != "Послезавтра" || !got.NSFW || got.PublishAt != "2026-10-21T09:00:00Z" ||
		h.posts.postponedEdit.Text != "Послезавтра" {
		t.Fatalf("edit: %d %+v", w.Code, got)
	}
	if w = h.do("PATCH", "/api/v1/postponed/pp2", `{"text":"x"}`, "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest {
		t.Fatalf("edit without a time: %d", w.Code)
	}

	w = h.do("POST", "/api/v1/postponed/pp2/publish", "", "Authorization", "Bearer tok")
	published := decode[struct {
		ID     string         `json:"id"`
		Author map[string]any `json:"author"`
	}](t, w)
	if w.Code != http.StatusCreated || published.ID != "p8" || published.Author["nickname"] != "bob" || h.posts.publishedPP != "pp2" {
		t.Fatalf("publish: %d %+v", w.Code, published)
	}

	if w = h.do("DELETE", "/api/v1/postponed/pp2", "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent || h.posts.deletedPP != "pp2" {
		t.Fatalf("delete: %d", w.Code)
	}
}
