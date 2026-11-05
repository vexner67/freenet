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

	t.Run("an invitation follows the inviter", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		h.users.profile = model.Profile{Name: "Bob", Nickname: "bob"}

		w := h.do("POST", "/api/v1/users/ann/invitation", "", "Authorization", "Bearer tok")
		if w.Code != http.StatusOK || h.users.invitedBy != "u1" || h.users.followed != "" {
			t.Fatalf("invitation: %d, invited by %q, followed %q", w.Code, h.users.invitedBy, h.users.followed)
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

func TestFriendLists(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})

	w := h.do("GET", "/api/v1/users/ann/friends?cursor=c1", "")
	page := decode[map[string]any](t, w)
	if w.Code != http.StatusOK || h.users.listed != "all:u1:c1" || page["total"] != float64(1) || page["nextCursor"] != "c2" || h.users.token != "" {
		t.Fatalf("friends: %d %v (%s)", w.Code, page, h.users.listed)
	}
	if users, _ := page["users"].([]any); len(users) != 1 {
		t.Fatalf("users = %v", page["users"])
	}

	// The requests go with the session, which the user service checks.
	w = h.do("GET", "/api/v1/users/ann/friends?section=requests", "", "Authorization", "Bearer tok")
	if w.Code != http.StatusOK || h.users.listed != "requests:u1:" || h.users.token != "tok" {
		t.Fatalf("requests: %d (%s)", w.Code, h.users.listed)
	}

	if w = h.do("GET", "/api/v1/users/nobody/friends", ""); w.Code != http.StatusNotFound {
		t.Fatalf("unknown nickname: %d", w.Code)
	}
}

// "Оставить в подписчиках" answers with the caller's own counters.
func TestDeclineFriend(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})

	if w := h.do("POST", "/api/v1/users/bob/decline", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", w.Code)
	}

	w := h.do("POST", "/api/v1/users/bob/decline", "", "Authorization", "Bearer tok")
	stats := decode[map[string]any](t, w)
	if w.Code != http.StatusOK || h.users.listed != "declined:u2" || stats["requestsCount"] != float64(1) || stats["friendsCount"] != float64(2) {
		t.Fatalf("decline: %d %v (%s)", w.Code, stats, h.users.listed)
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
	// Whom the reader follows the post service knows by the token.
	if h.posts.everyone || h.posts.cursor != "p9" {
		t.Fatalf("everyone %v, cursor %q", h.posts.everyone, h.posts.cursor)
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
	if !h.posts.everyone {
		t.Fatalf("sources: everyone %v", h.posts.everyone)
	}

	if w = h.do("GET", "/api/v1/feed?type=mine", "", "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest {
		t.Fatalf("an unknown type: %d", w.Code)
	}
}

// "Поиск записей" of openvk: every wall, without the blacklisted.
func TestSearchPosts(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	h.users.blockedIDs = []string{"x9"}

	w := h.do("GET", "/api/v1/posts?q=%23%D0%BF%D0%BE%D1%85%D0%BE%D0%B4&cursor=p9", "", "Authorization", "Bearer tok")
	got := decode[wall](t, w)

	if w.Code != http.StatusOK || got.Total != 7 || got.NextCursor != "p2" {
		t.Fatalf("status = %d, %+v", w.Code, got)
	}
	if h.posts.searched != "#поход" || h.posts.cursor != "p9" || !slices.Equal(h.posts.hidden, []string{"x9"}) {
		t.Fatalf("searched %q from %q without %v", h.posts.searched, h.posts.cursor, h.posts.hidden)
	}
	// The post of a deleted page is left out, as in the news.
	if len(got.Posts) != 1 || got.Posts[0].Author["nickname"] != "bob" {
		t.Fatalf("posts with authors: %+v", got.Posts)
	}

	if w = newHarness(rest.Config{}).do("GET", "/api/v1/posts?q=x", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("without token: %d", w.Code)
	}
}
