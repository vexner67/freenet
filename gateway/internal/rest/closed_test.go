package rest_test

import (
	"net/http"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/rest"
)

type closedClubJSON struct {
	Closed    bool   `json:"closed"`
	Role      string `json:"role"`
	Requested bool   `json:"requested"`
	Requests  int    `json:"requests"`
}

// A closed community tells of itself to anyone and shows what is in it to
// its members only.
func TestClosedClub(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	h.users.profile = ann
	h.clubs.closed = true

	w := h.do("GET", "/api/v1/clubs/cats", "")
	if got := decode[closedClubJSON](t, w); w.Code != http.StatusOK || !got.Closed || got.Requested {
		t.Fatalf("the club: %d %+v", w.Code, got)
	}

	for _, path := range []string{"/posts", "/members", "/topics", "/albums"} {
		w = h.do("GET", "/api/v1/clubs/cats"+path, "", "Authorization", "Bearer tok")
		if w.Code != http.StatusForbidden || decode[errorBody](t, w).Error.Message != "this club is closed" {
			t.Errorf("a stranger reads %s: %d %s", path, w.Code, w.Body)
		}
	}

	// What was said in its discussions is not read by its ID either, nor
	// who liked it; a heart is not put on it, and one put before comes off.
	for _, tt := range []struct {
		method, path string
		code         int
	}{
		{"GET", "/api/v1/topic-messages/m1/text", http.StatusNotFound},
		{"GET", "/api/v1/topic-messages/m1/likes", http.StatusNotFound},
		{"PUT", "/api/v1/topic-messages/m1/like", http.StatusNotFound},
		{"DELETE", "/api/v1/topic-messages/m1/like", http.StatusOK},
	} {
		if w = h.do(tt.method, tt.path, "", "Authorization", "Bearer tok"); w.Code != tt.code {
			t.Errorf("a stranger: %s %s: %d %s, want %d", tt.method, tt.path, w.Code, w.Body, tt.code)
		}
	}

	// Joining is asking.
	w = h.do("PUT", "/api/v1/clubs/cats/membership", "", "Authorization", "Bearer tok")
	if got := decode[closedClubJSON](t, w); w.Code != http.StatusOK || got.Role != "" || !got.Requested {
		t.Fatalf("join: %d %+v", w.Code, got)
	}

	h.clubs.role = "member"
	for _, path := range []string{"/topic-messages/m1/text", "/topic-messages/m1/likes"} {
		if w = h.do("GET", "/api/v1"+path, "", "Authorization", "Bearer tok"); w.Code != http.StatusOK {
			t.Errorf("a member reads %s: %d %s", path, w.Code, w.Body.String())
		}
	}
	// What a deleted page wrote there is gone for everyone.
	for _, path := range []string{"/topic-messages/m3/text", "/topic-messages/m3/likes"} {
		if w = h.do("GET", "/api/v1"+path, "", "Authorization", "Bearer tok"); w.Code != http.StatusNotFound {
			t.Errorf("a message of a deleted page, %s: %d %s", path, w.Code, w.Body.String())
		}
	}
	for _, path := range []string{"/posts", "/members", "/topics", "/albums"} {
		if w = h.do("GET", "/api/v1/clubs/cats"+path, "", "Authorization", "Bearer tok"); w.Code != http.StatusOK {
			t.Errorf("a member reads %s: %d %s", path, w.Code, w.Body.String())
		}
	}

	w = h.do("PUT", "/api/v1/clubs/cats", `{"name":"Коты","wall":"open","adminsDisplay":"all","closed":true}`, "Authorization", "Bearer tok")
	if w.Code != http.StatusOK || !h.clubs.changes.Closed {
		t.Fatalf("update: %d, changes %+v", w.Code, h.clubs.changes)
	}
}

func TestJoinRequests(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	h.users.profile = ann
	h.clubs.closed, h.clubs.role = true, "admin"
	// u3 has deleted the page: counted and not shown.
	h.clubs.requests = []string{"u2", "u3"}

	if w := h.do("GET", "/api/v1/clubs/cats/requests", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("without a token: %d", w.Code)
	}

	w := h.do("GET", "/api/v1/clubs/cats/requests", "", "Authorization", "Bearer tok")
	got := decode[struct {
		People []struct {
			Nickname string `json:"nickname"`
		} `json:"people"`
		NextCursor string `json:"nextCursor"`
		Total      int    `json:"total"`
	}](t, w)
	if w.Code != http.StatusOK || got.Total != 2 || len(got.People) != 1 || got.People[0].Nickname != "bob" || got.NextCursor != "r1" {
		t.Fatalf("requests: %d %+v", w.Code, got)
	}

	w = h.do("GET", "/api/v1/clubs/cats", "", "Authorization", "Bearer tok")
	if c := decode[closedClubJSON](t, w); c.Requests != 2 {
		t.Fatalf("the club of its admin: %+v", c)
	}

	if w = h.do("PUT", "/api/v1/clubs/cats/requests/bob", "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent {
		t.Fatalf("accept: %d %s", w.Code, w.Body.String())
	}
	if w = h.do("DELETE", "/api/v1/clubs/cats/requests/bob", "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent {
		t.Fatalf("reject: %d %s", w.Code, w.Body.String())
	}
	if d := h.clubs.decided; len(d) != 2 || d[0] != "+u2" || d[1] != "-u2" {
		t.Fatalf("decided = %v", d)
	}

	if w = h.do("PUT", "/api/v1/clubs/cats/requests/nobody", "", "Authorization", "Bearer tok"); w.Code != http.StatusNotFound {
		t.Fatalf("accepting nobody: %d", w.Code)
	}
	if w = h.do("PUT", "/api/v1/clubs/dogs/requests/bob", "", "Authorization", "Bearer tok"); w.Code != http.StatusNotFound {
		t.Fatalf("an unknown club: %d", w.Code)
	}
	if w = h.do("PUT", "/api/v1/clubs/cats/requests/bob", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("accepting without a token: %d", w.Code)
	}
}

// A closed page tells so with the names, and its owner sets the type.
func TestClosedProfile(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	h.users.hiddenPage, h.users.closedPage = true, true

	w := h.do("GET", "/api/v1/users/bob", "")
	got := decode[struct {
		PageHidden bool `json:"pageHidden"`
		Closed     bool `json:"closed"`
	}](t, w)
	if w.Code != http.StatusOK || !got.PageHidden || !got.Closed {
		t.Fatalf("the closed page: %d %+v", w.Code, got)
	}

	w = h.do("GET", "/api/v1/users/bob/posts", "")
	if w.Code != http.StatusForbidden || decode[errorBody](t, w).Error.Message != "this page is hidden" {
		t.Fatalf("the wall of the closed page: %d %s", w.Code, w.Body)
	}

	// The owner sets "Тип профиля".
	w = h.do("PUT", "/api/v1/me/privacy", `{"closed":true}`, "Authorization", "Bearer tok")
	u := decode[map[string]any](t, w)
	if privacy, _ := u["privacy"].(map[string]any); w.Code != http.StatusOK || privacy["closed"] != true {
		t.Fatalf("privacy: %d %v", w.Code, u["privacy"])
	}
}
