package rest_test

import (
	"net/http"
	"slices"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

const bearer = "Bearer tok"

// openvk's "Игнорировать пользователя" and "Игнорировать группу".
func TestIgnoreSources(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})

	if w := h.do("PUT", "/api/v1/me/feed/ignored/users/bob", "", "Authorization", bearer); w.Code != http.StatusNoContent || h.users.ignored != "u2" {
		t.Fatalf("ignore a user: %d, ignored %q", w.Code, h.users.ignored)
	}
	if w := h.do("PUT", "/api/v1/me/feed/ignored/clubs/cats", "", "Authorization", bearer); w.Code != http.StatusNoContent || h.users.ignored != "k1" {
		t.Fatalf("ignore a club: %d, ignored %q", w.Code, h.users.ignored)
	}

	h.do("DELETE", "/api/v1/me/feed/ignored/users/bob", "", "Authorization", bearer)
	w := h.do("DELETE", "/api/v1/me/feed/ignored/clubs/k1", "", "Authorization", bearer)
	if w.Code != http.StatusNoContent || !slices.EqualFunc(h.users.unignored, [][]string{{"u2"}, {"k1"}}, slices.Equal) {
		t.Fatalf("unignore: %d, unignored %v", w.Code, h.users.unignored)
	}

	h.users.unignored = nil
	if w = h.do("DELETE", "/api/v1/me/feed/ignored?id=u2&id=k1&id=u2", "", "Authorization", bearer); w.Code != http.StatusNoContent ||
		!slices.EqualFunc(h.users.unignored, [][]string{{"u2", "k1"}}, slices.Equal) {
		t.Fatalf("unignore several: %d, unignored %v", w.Code, h.users.unignored)
	}

	tests := []struct {
		name, method, path string
		headers            []string
		want               int
	}{
		{"an unknown nickname", "PUT", "/api/v1/me/feed/ignored/users/nobody", []string{"Authorization", bearer}, http.StatusNotFound},
		{"an unknown club", "PUT", "/api/v1/me/feed/ignored/clubs/dogs", []string{"Authorization", bearer}, http.StatusNotFound},
		{"nothing to unignore", "DELETE", "/api/v1/me/feed/ignored", []string{"Authorization", bearer}, http.StatusBadRequest},
		{
			"too many to unignore", "DELETE",
			"/api/v1/me/feed/ignored?id=1&id=2&id=3&id=4&id=5&id=6&id=7&id=8&id=9&id=10&id=11",
			[]string{"Authorization", bearer}, http.StatusBadRequest,
		},
		{"ignore without a token", "PUT", "/api/v1/me/feed/ignored/users/bob", nil, http.StatusUnauthorized},
		{"the list without a token", "GET", "/api/v1/me/feed/ignored", nil, http.StatusUnauthorized},
		{"unignore without a token", "DELETE", "/api/v1/me/feed/ignored?id=u2", nil, http.StatusUnauthorized},
	}

	for _, tt := range tests {
		if w = h.do(tt.method, tt.path, "", tt.headers...); w.Code != tt.want {
			t.Errorf("%s: %d, want %d", tt.name, w.Code, tt.want)
		}
	}

	// The 51st source: the user service says so.
	h.users.err = failure.New(failure.Conflict, "too many ignored sources")
	if w = h.do("PUT", "/api/v1/me/feed/ignored/clubs/cats", "", "Authorization", bearer); w.Code != http.StatusConflict {
		t.Fatalf("over the limit: %d", w.Code)
	}
}

// The list names people, deleted pages among them, and communities, in
// the order they were ignored; what is gone stops being ignored.
func TestListIgnored(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	h.users.ignoredIDs = []string{"k1", "erased", "u3", "u2"}

	w := h.do("GET", "/api/v1/me/feed/ignored", "", "Authorization", bearer)
	got := decode[struct {
		Sources []map[string]any `json:"sources"`
	}](t, w)

	if w.Code != http.StatusOK || len(got.Sources) != 3 {
		t.Fatalf("status = %d, sources %v", w.Code, got.Sources)
	}

	club, deleted, bob := got.Sources[0], got.Sources[1], got.Sources[2]
	if club["id"] != "k1" || club["club"] != true || club["name"] != cats.Name {
		t.Errorf("the club = %v", club)
	}
	if deleted["id"] != "u3" || deleted["deactivated"] != true {
		t.Errorf("the deleted page = %v", deleted)
	}
	if bob["nickname"] != "bob" {
		t.Errorf("bob = %v", bob)
	}

	if !slices.EqualFunc(h.users.unignored, [][]string{{"erased"}}, slices.Equal) {
		t.Fatalf("what is gone must stop being ignored: %v", h.users.unignored)
	}

	empty := newHarness(rest.Config{})
	if got := decode[map[string]any](t, empty.do("GET", "/api/v1/me/feed/ignored", "", "Authorization", bearer)); len(got["sources"].([]any)) != 0 {
		t.Fatalf("nothing ignored = %v", got)
	}
}

// "Настройки ленты": the news leave the ignored walls out and, asked to,
// what people wrote on the walls of others.
func TestFeed_Settings(t *testing.T) {
	t.Parallel()

	feed := func(query string, ignored ...string) harness {
		t.Helper()

		h := newHarness(rest.Config{})
		h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}
		h.users.ignoredIDs = ignored

		if w := h.do("GET", "/api/v1/feed"+query, "", "Authorization", bearer); w.Code != http.StatusOK {
			t.Fatalf("feed%s: %d", query, w.Code)
		}

		return h
	}

	h := feed("")
	if h.posts.ownOnly || len(h.posts.ignored) != 0 || h.posts.everyone {
		t.Fatalf("by default: own only %v, ignored %v, everyone %v", h.posts.ownOnly, h.posts.ignored, h.posts.everyone)
	}

	// The post service leaves the ignored out itself.
	h = feed("", "u2", "k1")
	if !slices.Equal(h.posts.ignored, []string{"u2", "k1"}) {
		t.Fatalf("with ignored sources: ignored %v", h.posts.ignored)
	}

	h = feed("?ignored=true&alien=false", "u2", "k1")
	if len(h.posts.ignored) != 0 || !h.posts.ownOnly {
		t.Fatalf("showing the ignored, own only: ignored %v, own only %v", h.posts.ignored, h.posts.ownOnly)
	}

	h = feed("?type=all&alien=false", "k1")
	if !h.posts.everyone || !h.posts.ownOnly || !slices.Equal(h.posts.ignored, []string{"k1"}) {
		t.Fatalf("all news: everyone %v, own only %v, ignored %v", h.posts.everyone, h.posts.ownOnly, h.posts.ignored)
	}
}

// The pages of a person and of a club tell whether the viewer ignores them.
func TestIgnoredByMe(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	h.users.ignoredIDs = []string{"u2", "k1"}

	if got := decode[map[string]any](t, h.do("GET", "/api/v1/users/bob", "", "Authorization", bearer)); got["ignoredByMe"] != true {
		t.Fatalf("an ignored person = %v", got["ignoredByMe"])
	}
	if got := decode[map[string]any](t, h.do("GET", "/api/v1/clubs/cats", "", "Authorization", bearer)); got["ignoredByMe"] != true {
		t.Fatalf("an ignored club = %v", got["ignoredByMe"])
	}
	if got := decode[map[string]any](t, h.do("GET", "/api/v1/users/ann", "", "Authorization", bearer)); got["ignoredByMe"] != false {
		t.Fatalf("a person not ignored = %v", got["ignoredByMe"])
	}

	// Nobody without a token ignores anything.
	if got := decode[map[string]any](t, h.do("GET", "/api/v1/users/bob", "")); got["ignoredByMe"] != false {
		t.Fatalf("for a guest = %v", got["ignoredByMe"])
	}
	if got := decode[map[string]any](t, h.do("GET", "/api/v1/clubs/cats", "")); got["ignoredByMe"] != nil {
		t.Fatalf("a club for a guest = %v", got["ignoredByMe"])
	}
}
