package rest_test

import (
	"net/http"
	"slices"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

// The gateway reads the nicknames a text names, asks the user service who
// of them hears of it and passes their IDs on with the text.
func TestMentions(t *testing.T) {
	t.Parallel()

	const text = `{"text":"@bob, [ann|Аня] и @nobody"}`
	asked := []string{"bob", "ann", "nobody"}
	want := []string{"u2", "u1"}

	t.Run("in a post", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}

		if w := h.do("POST", "/api/v1/me/posts", text, "Authorization", "Bearer tok"); w.Code != http.StatusCreated {
			t.Fatalf("post: %d %s", w.Code, w.Body)
		}
		if !slices.Equal(h.users.named, asked) || !slices.Equal(h.posts.newPost.Mentions, want) {
			t.Fatalf("asked about %v, passed %v", h.users.named, h.posts.newPost.Mentions)
		}
	})

	t.Run("in a comment", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}

		if w := h.do("POST", "/api/v1/posts/p1/comments", text, "Authorization", "Bearer tok"); w.Code != http.StatusCreated {
			t.Fatalf("comment: %d %s", w.Code, w.Body)
		}
		if !slices.Equal(h.users.named, asked) || !slices.Equal(h.posts.newComment.Mentions, want) {
			t.Fatalf("asked about %v, passed %v", h.users.named, h.posts.newComment.Mentions)
		}
	})

	t.Run("in a message of a topic", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}

		if w := h.do("POST", "/api/v1/topics/t1/messages", text, "Authorization", "Bearer tok"); w.Code != http.StatusCreated {
			t.Fatalf("message: %d %s", w.Code, w.Body)
		}
		if !slices.Equal(h.users.named, asked) || !slices.Equal(h.topics.written.Mentions, want) {
			t.Fatalf("asked about %v, passed %v", h.users.named, h.topics.written.Mentions)
		}
	})

	t.Run("a text that names nobody asks nothing", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}

		if w := h.do("POST", "/api/v1/me/posts", `{"text":"mail@example.com"}`, "Authorization", "Bearer tok"); w.Code != http.StatusCreated {
			t.Fatalf("post: %d %s", w.Code, w.Body)
		}
		if h.users.named != nil || h.posts.newPost.Mentions != nil {
			t.Fatalf("asked about %v, passed %v", h.users.named, h.posts.newPost.Mentions)
		}
	})
}
