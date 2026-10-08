package rest_test

import (
	"net/http"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

type commentPage struct {
	Comments []struct {
		ID     string            `json:"id"`
		PostID string            `json:"postId"`
		Text   string            `json:"text"`
		Author map[string]string `json:"author"`
	} `json:"comments"`
	NextCursor string `json:"nextCursor"`
	Total      int    `json:"total"`
}

func TestListComments(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("GET", "/api/v1/posts/p1/comments?cursor=c0&limit=3", "")
	got := decode[commentPage](t, w)

	if w.Code != http.StatusOK || got.Total != 5 || got.NextCursor != "c3" {
		t.Fatalf("%d %+v", w.Code, got)
	}
	if len(got.Comments) != 2 || got.Comments[0].Author["nickname"] != "bob" || got.Comments[1].Author["nickname"] != "ann" {
		t.Fatalf("comments with authors, without the vanished one: %+v", got.Comments)
	}
	if got.Comments[0].PostID != "p1" || got.Comments[0].Text != "первый" {
		t.Fatalf("comment = %+v", got.Comments[0])
	}
	if h.posts.cursor != "c0" || h.posts.limit != 3 || len(h.users.profileIDs) != 3 {
		t.Fatalf("cursor = %q, limit = %d, authors looked up in one request: %v", h.posts.cursor, h.posts.limit, h.users.profileIDs)
	}

	if w = newHarness(rest.Config{}).do("GET", "/api/v1/posts/p404/comments", ""); w.Code != http.StatusNotFound {
		t.Fatalf("missing post: status = %d", w.Code)
	}

	if w = newHarness(rest.Config{}).do("GET", "/api/v1/posts/p1/comments?limit=0", ""); w.Code != http.StatusBadRequest {
		t.Fatalf("bad limit: status = %d", w.Code)
	}
}

func TestCreateComment(t *testing.T) {
	t.Parallel()

	t.Run("with the caller as the author", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		h.users.profile = model.Profile{Name: "Ann", Nickname: "ann", Gender: "female"}

		w := h.do("POST", "/api/v1/posts/p1/comments", `{"text":"согласна"}`, "Authorization", "Bearer tok")
		got := decode[map[string]any](t, w)

		if w.Code != http.StatusCreated || h.posts.created != "согласна" || h.posts.token != "tok" {
			t.Fatalf("%d %v", w.Code, got)
		}
		if got["postId"] != "p1" || got["createdAt"] != "2026-10-05T12:00:00Z" {
			t.Fatalf("comment = %v", got)
		}
		if author, _ := got["author"].(map[string]any); author["nickname"] != "ann" || author["gender"] != "female" {
			t.Fatalf("author = %v", got["author"])
		}
	})

	t.Run("with photos, the caller's only", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}

		w := h.do("POST", "/api/v1/posts/p1/comments", `{"photos":["ph1"]}`, "Authorization", "Bearer tok")
		got := decode[map[string]any](t, w)
		photos, _ := got["photos"].([]any)
		if w.Code != http.StatusCreated || len(photos) != 1 || len(h.posts.photos) != 1 {
			t.Fatalf("%d %v", w.Code, got)
		}
		if photo, _ := photos[0].(map[string]any); photo["small"] != "/media/photos/u1/ph1/small.jpg" {
			t.Fatalf("photo = %v", photos[0])
		}

		h = newHarness(rest.Config{})
		h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}
		if w = h.do("POST", "/api/v1/posts/p1/comments", `{"photos":["ph7"]}`, "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest || h.posts.created != "" {
			t.Fatalf("someone else's photo: %d", w.Code)
		}
	})

	t.Run("profile must be filled in first", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		w := h.do("POST", "/api/v1/posts/p1/comments", `{"text":"привет"}`, "Authorization", "Bearer tok")

		if got := decode[errorBody](t, w); w.Code != http.StatusBadRequest || got.Error.Message != "fill in your profile before commenting" || h.posts.created != "" {
			t.Fatalf("status = %d, %+v, created = %q", w.Code, got, h.posts.created)
		}
	})

	t.Run("needs a token", func(t *testing.T) {
		t.Parallel()

		if w := newHarness(rest.Config{}).do("POST", "/api/v1/posts/p1/comments", `{"text":"x"}`); w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d", w.Code)
		}
	})
}

func TestDeleteComment(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	if w := h.do("DELETE", "/api/v1/comments/c1", "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent || h.posts.deleted != "c1" {
		t.Fatalf("status = %d, deleted = %q", w.Code, h.posts.deleted)
	}

	h = newHarness(rest.Config{})
	h.posts.err = failure.New(failure.PermissionDenied, "permission denied")
	if w := h.do("DELETE", "/api/v1/comments/c1", "", "Authorization", "Bearer tok"); w.Code != http.StatusForbidden {
		t.Fatalf("someone else's comment: status = %d", w.Code)
	}

	if w := newHarness(rest.Config{}).do("DELETE", "/api/v1/comments/c1", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("without token: status = %d", w.Code)
	}
}

func TestCommentRepliesAndLikes(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}

	w := h.do("POST", "/api/v1/posts/p1/comments", `{"text":"Пётр, согласна","replyTo":"c1"}`, "Authorization", "Bearer tok")
	got := decode[map[string]any](t, w)
	if w.Code != http.StatusCreated || h.posts.replyTo != "c1" || got["replyTo"] != "c1" {
		t.Fatalf("reply: %d %v, sent replyTo %q", w.Code, got, h.posts.replyTo)
	}

	if w = h.do("PUT", "/api/v1/comments/c1/like", "", "Authorization", "Bearer tok"); w.Code != http.StatusOK || h.posts.likedComment != "c1" {
		t.Fatalf("like: %d", w.Code)
	}
	if like := decode[map[string]any](t, w); like["likes"] != float64(2) || like["liked"] != true {
		t.Fatalf("like = %v", like)
	}
	if w = h.do("DELETE", "/api/v1/comments/c1/like", "", "Authorization", "Bearer tok"); w.Code != http.StatusOK || h.posts.likedComment != "-c1" {
		t.Fatalf("unlike: %d", w.Code)
	}
	if w = h.do("PUT", "/api/v1/comments/c404/like", "", "Authorization", "Bearer tok"); w.Code != http.StatusNotFound {
		t.Fatalf("like a missing comment: %d", w.Code)
	}
	if w = h.do("PUT", "/api/v1/comments/c1/like", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("like without a token: %d", w.Code)
	}

	// The list tells the caller what they like, and what answers what.
	page := decode[map[string]any](t, h.do("GET", "/api/v1/posts/p1/comments", "", "Authorization", "Bearer tok"))
	comments, _ := page["comments"].([]any)
	last, _ := comments[len(comments)-1].(map[string]any)
	first, _ := comments[0].(map[string]any)
	if last["replyTo"] != "c1" || last["liked"] != true || last["likes"] != float64(2) || first["replyTo"] != nil {
		t.Fatalf("comments = %v", comments)
	}
	anon := decode[map[string]any](t, h.do("GET", "/api/v1/posts/p1/comments", ""))
	if c, _ := anon["comments"].([]any)[len(comments)-1].(map[string]any); c["liked"] != false {
		t.Fatalf("an anonymous viewer likes %v", c)
	}
}
