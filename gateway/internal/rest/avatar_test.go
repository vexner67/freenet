package rest_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

func TestUploadAvatar(t *testing.T) {
	t.Parallel()

	t.Run("upload, point the profile, clean up the rest", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		w := h.do("PUT", "/api/v1/me/avatar", "\x89PNG...", "Authorization", "Bearer tok", "Content-Type", "image/png")
		me := decode[map[string]any](t, w)

		if w.Code != http.StatusOK || string(h.media.uploaded) != "\x89PNG..." || h.users.avatarID != "a2" {
			t.Fatalf("%d, uploaded %q, avatar %q", w.Code, h.media.uploaded, h.users.avatarID)
		}
		if !h.media.deleted || h.media.kept != "a2" {
			t.Fatalf("old avatars must be deleted keeping the new one: kept %q", h.media.kept)
		}
		// A photo of its own goes into "Фотографии со страницы".
		if !slices.Equal(h.albums.avatarPhotos, []string{"/ph9"}) {
			t.Fatalf("avatar album got %v", h.albums.avatarPhotos)
		}
		avatar, _ := me["avatar"].(map[string]any)
		if avatar["large"] != "/media/avatars/u1/a2/large.jpg" || avatar["small"] != "/media/avatars/u1/a2/small.jpg" {
			t.Fatalf("avatar = %v", me["avatar"])
		}
	})

	t.Run("posted on the wall", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		h.users.profile = model.Profile{Name: "Анна", Nickname: "ann"}
		w := h.do("PUT", "/api/v1/me/avatar?onWall=true", "\x89PNG...", "Authorization", "Bearer tok")

		np := h.posts.newPost
		if w.Code != http.StatusOK || h.users.avatarID != "a2" || !np.AvatarUpdate || np.WallID != "" || np.FromWall || len(np.PhotoIDs) != 1 || np.PhotoIDs[0] != "ph9" {
			t.Fatalf("%d, avatar %q, post %+v", w.Code, h.users.avatarID, np)
		}

		// Without a profile there is no one to post as; without onWall
		// nothing is asked.
		for _, path := range []string{"/api/v1/me/avatar?onWall=true", "/api/v1/me/avatar"} {
			h := newHarness(rest.Config{})
			if path == "/api/v1/me/avatar" {
				h.users.profile = model.Profile{Name: "Анна", Nickname: "ann"}
			}
			if w := h.do("PUT", path, "\x89PNG...", "Authorization", "Bearer tok"); w.Code != http.StatusOK || h.posts.newPost.AvatarUpdate {
				t.Fatalf("%s: %d, post %+v", path, w.Code, h.posts.newPost)
			}
		}
	})

	t.Run("bad image is not set", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		h.media.err = failure.New(failure.InvalidArgument, "not a JPEG, PNG, GIF or WebP image")

		w := h.do("PUT", "/api/v1/me/avatar", "text", "Authorization", "Bearer tok")
		if w.Code != http.StatusBadRequest || h.users.avatarID != "" || h.media.deleted {
			t.Fatalf("%d, avatar %q, deleted %v", w.Code, h.users.avatarID, h.media.deleted)
		}
	})

	t.Run("too large", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		w := h.do("PUT", "/api/v1/me/avatar", strings.Repeat("x", 8<<20+1), "Authorization", "Bearer tok")
		if w.Code != http.StatusBadRequest || h.media.uploaded != nil {
			t.Fatalf("status = %d", w.Code)
		}
	})

	t.Run("delete", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		w := h.do("DELETE", "/api/v1/me/avatar", "", "Authorization", "Bearer tok")
		me := decode[map[string]any](t, w)

		if w.Code != http.StatusOK || me["avatar"] != nil || !h.media.deleted || h.media.kept != "" {
			t.Fatalf("%d %v, kept %q", w.Code, me["avatar"], h.media.kept)
		}
	})

	t.Run("needs a token", func(t *testing.T) {
		t.Parallel()

		if w := newHarness(rest.Config{}).do("PUT", "/api/v1/me/avatar", "x"); w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d", w.Code)
		}
	})
}

func TestServeAvatar(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})

	w := h.do("GET", "/media/avatars/u1/a1/small.jpg", "")
	if w.Code != http.StatusOK || w.Body.String() != "small-jpeg" || w.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("%d %q %v", w.Code, w.Body.String(), w.Header())
	}
	if cc := w.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Fatalf("avatars must be cached: %q", cc)
	}

	for _, path := range []string{"/media/avatars/u1/a1/huge.jpg", "/media/avatars/u1/nope/large.jpg"} {
		if w = h.do("GET", path, ""); w.Code != http.StatusNotFound {
			t.Fatalf("%s: %d", path, w.Code)
		}
	}
}

func TestSearchUsers(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("GET", "/api/v1/users?q=%D0%B0%D0%BD%D0%BD%D0%B0&cursor=c1", "")
	page := decode[map[string]any](t, w)

	if w.Code != http.StatusOK || h.users.query != "анна" || h.users.cursor != "c1" || page["nextCursor"] != "u1" {
		t.Fatalf("%d %v (query %q)", w.Code, page, h.users.query)
	}
	users, _ := page["users"].([]any)
	first, _ := users[0].(map[string]any)
	if avatar, _ := first["avatar"].(map[string]any); avatar["small"] != "/media/avatars/u1/a1/small.jpg" {
		t.Fatalf("results carry avatars: %v", first)
	}

	if w = h.do("GET", "/api/v1/users", ""); w.Code != http.StatusBadRequest {
		t.Fatalf("empty query: %d", w.Code)
	}
}
