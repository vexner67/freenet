package rest_test

import (
	"net/http"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

type wall struct {
	Posts []struct {
		ID     string            `json:"id"`
		Text   string            `json:"text"`
		Author map[string]string `json:"author"`
	} `json:"posts"`
	NextCursor string `json:"nextCursor"`
	Total      int    `json:"total"`
}

func TestListPosts(t *testing.T) {
	t.Parallel()

	t.Run("wall of a nickname with its author", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		w := h.do("GET", "/api/v1/users/ann/posts?cursor=p5&limit=2", "")
		got := decode[wall](t, w)

		if w.Code != http.StatusOK || got.Total != 7 || got.NextCursor != "p1" || len(got.Posts) != 2 {
			t.Fatalf("%d %+v", w.Code, got)
		}
		if a := got.Posts[0].Author; a["nickname"] != "ann" || a["name"] != "Ann" || a["id"] != "u1" {
			t.Fatalf("author = %v", a)
		}
		if h.posts.cursor != "p5" || h.posts.limit != 2 {
			t.Fatalf("cursor = %q, limit = %d", h.posts.cursor, h.posts.limit)
		}
	})

	t.Run("unknown nickname", func(t *testing.T) {
		t.Parallel()

		if w := newHarness(rest.Config{}).do("GET", "/api/v1/users/nobody/posts", ""); w.Code != http.StatusNotFound {
			t.Fatalf("status = %d", w.Code)
		}
	})

	for _, limit := range []string{"0", "51", "ten"} {
		t.Run("limit "+limit, func(t *testing.T) {
			t.Parallel()

			if w := newHarness(rest.Config{}).do("GET", "/api/v1/users/ann/posts?limit="+limit, ""); w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d", w.Code)
			}
		})
	}
}

func TestCreatePost(t *testing.T) {
	t.Parallel()

	t.Run("on the caller's wall", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		h.users.profile = model.Profile{Name: "Ann", Nickname: "ann", Gender: "female"}

		w := h.do("POST", "/api/v1/me/posts", `{"text":"привет"}`, "Authorization", "Bearer tok")
		got := decode[map[string]any](t, w)

		if w.Code != http.StatusCreated || h.posts.created != "привет" || h.posts.token != "tok" {
			t.Fatalf("%d %v", w.Code, got)
		}
		if author, _ := got["author"].(map[string]any); author["nickname"] != "ann" || author["gender"] != "female" {
			t.Fatalf("author = %v", got["author"])
		}
		if got["createdAt"] != "2026-10-04T15:30:00Z" {
			t.Fatalf("createdAt = %v", got["createdAt"])
		}
	})

	t.Run("profile must be filled in first", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		w := h.do("POST", "/api/v1/me/posts", `{"text":"привет"}`, "Authorization", "Bearer tok")

		if w.Code != http.StatusBadRequest || h.posts.created != "" {
			t.Fatalf("status = %d, created = %q", w.Code, h.posts.created)
		}
	})

	t.Run("needs a token", func(t *testing.T) {
		t.Parallel()

		if w := newHarness(rest.Config{}).do("POST", "/api/v1/me/posts", `{"text":"x"}`); w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d", w.Code)
		}
	})
}

func TestDeletePost(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	if w := h.do("DELETE", "/api/v1/posts/p1", "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent || h.posts.deleted != "p1" {
		t.Fatalf("status = %d, deleted = %q", w.Code, h.posts.deleted)
	}

	h = newHarness(rest.Config{})
	h.posts.err = failure.New(failure.PermissionDenied, "permission denied")
	if w := h.do("DELETE", "/api/v1/posts/p1", "", "Authorization", "Bearer tok"); w.Code != http.StatusForbidden {
		t.Fatalf("someone else's post: status = %d", w.Code)
	}

	if w := newHarness(rest.Config{}).do("DELETE", "/api/v1/posts/p1", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("without token: status = %d", w.Code)
	}
}
