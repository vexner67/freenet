package rest_test

import (
	"net/http"
	"slices"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

func TestProfileStats(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("GET", "/api/v1/users/ann", "")
	p := decode[map[string]any](t, w)

	if w.Code != http.StatusOK || p["followersCount"] != float64(3) || p["followingCount"] != float64(1) || p["isFollowing"] != false {
		t.Fatalf("anonymous: %d %v", w.Code, p)
	}
	if p["nickname"] != "ann" {
		t.Fatalf("profile fields must stay: %v", p)
	}

	h = newHarness(rest.Config{})
	p = decode[map[string]any](t, h.do("GET", "/api/v1/users/ann", "", "Authorization", "Bearer tok"))
	if h.users.statsToken != "tok" || p["isFollowing"] != true {
		t.Fatalf("signed in: token %q, %v", h.users.statsToken, p)
	}
}

func TestFollow(t *testing.T) {
	t.Parallel()

	t.Run("follow and unfollow by nickname", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		h.users.profile = model.Profile{Name: "Bob", Nickname: "bob"}

		w := h.do("POST", "/api/v1/users/ann/follow", "", "Authorization", "Bearer tok")
		if w.Code != http.StatusOK || h.users.followed != "u1" {
			t.Fatalf("follow: %d, followed %q", w.Code, h.users.followed)
		}
		if stats := decode[map[string]any](t, w); stats["followersCount"] != float64(3) {
			t.Fatalf("fresh counters expected: %v", stats)
		}

		if w = h.do("DELETE", "/api/v1/users/ann/follow", "", "Authorization", "Bearer tok"); w.Code != http.StatusOK || h.users.unfollowed != "u1" {
			t.Fatalf("unfollow: %d, unfollowed %q", w.Code, h.users.unfollowed)
		}
	})

	t.Run("needs a filled-in profile", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		if w := h.do("POST", "/api/v1/users/ann/follow", "", "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest || h.users.followed != "" {
			t.Fatalf("status = %d", w.Code)
		}
	})

	t.Run("needs a token", func(t *testing.T) {
		t.Parallel()

		if w := newHarness(rest.Config{}).do("POST", "/api/v1/users/ann/follow", ""); w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d", w.Code)
		}
	})
}

func TestFollowLists(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})

	w := h.do("GET", "/api/v1/users/ann/followers?cursor=c1", "")
	page := decode[map[string]any](t, w)
	if w.Code != http.StatusOK || h.users.listed != "followers:u1:c1" || page["total"] != float64(1) {
		t.Fatalf("followers: %d %v (%s)", w.Code, page, h.users.listed)
	}
	if users, _ := page["users"].([]any); len(users) != 1 {
		t.Fatalf("users = %v", page["users"])
	}

	w = h.do("GET", "/api/v1/users/ann/following", "")
	page = decode[map[string]any](t, w)
	if h.users.listed != "following:u1:" || page["nextCursor"] != "c2" {
		t.Fatalf("following: %v (%s)", page, h.users.listed)
	}
	if users, _ := page["users"].([]any); users == nil {
		t.Fatal("an empty list must be [], not null")
	}

	// openvk's "Общие друзья": whom the caller follows too.
	if w = h.do("GET", "/api/v1/users/ann/following?common=true", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("common, anonymous: %d", w.Code)
	}
	w = h.do("GET", "/api/v1/users/ann/following?common=true", "", "Authorization", "Bearer tok")
	if page = decode[map[string]any](t, w); h.users.listed != "common:u1:" || page["total"] != float64(1) || h.users.token != "tok" {
		t.Fatalf("common: %v (%s)", page, h.users.listed)
	}

	// openvk's "Друзья онлайн".
	w = h.do("GET", "/api/v1/users/ann/following?online=true", "")
	if page = decode[map[string]any](t, w); h.users.listed != "online:u1:" || page["total"] != float64(1) {
		t.Fatalf("online: %v (%s)", page, h.users.listed)
	}

	if w = h.do("GET", "/api/v1/users/nobody/followers", ""); w.Code != http.StatusNotFound {
		t.Fatalf("unknown nickname: %d", w.Code)
	}
}

func TestFeed(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}

	w := h.do("GET", "/api/v1/feed?cursor=p9", "", "Authorization", "Bearer tok")
	got := decode[wall](t, w)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	if !slices.Equal(h.posts.authors, []string{"u2", "gone", "u1"}) || h.posts.cursor != "p9" {
		t.Fatalf("feed sources = %v, cursor %q: followed users and me", h.posts.authors, h.posts.cursor)
	}
	if len(got.Posts) != 2 || got.Posts[0].Author["nickname"] != "bob" || got.Posts[1].Author["nickname"] != "ann" {
		t.Fatalf("posts with authors, without the vanished one: %+v", got.Posts)
	}
	if len(h.users.profileIDs) != 3 {
		t.Fatalf("authors looked up in one request: %v", h.users.profileIDs)
	}
	if h.posts.token != "tok" || !got.Posts[0].Liked || got.Posts[0].Likes != 1 {
		t.Fatalf("likes of the viewer: token %q, %+v", h.posts.token, got.Posts[0])
	}

	if w = newHarness(rest.Config{}).do("GET", "/api/v1/feed", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("without token: %d", w.Code)
	}
}

// "Все новости" ask the post service for everyone, with no follows.
func TestFeed_All(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}

	w := h.do("GET", "/api/v1/feed?type=all", "", "Authorization", "Bearer tok")
	got := decode[wall](t, w)

	if w.Code != http.StatusOK || len(got.Posts) != 2 {
		t.Fatalf("status = %d, posts %+v", w.Code, got.Posts)
	}
	if !h.posts.everyone || len(h.posts.authors) != 0 || len(h.posts.walls) != 0 {
		t.Fatalf("sources: everyone %v, authors %v, walls %v", h.posts.everyone, h.posts.authors, h.posts.walls)
	}

	if w = h.do("GET", "/api/v1/feed?type=mine", "", "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest {
		t.Fatalf("an unknown type: %d", w.Code)
	}
}
