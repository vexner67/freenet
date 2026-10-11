package rest_test

import (
	"net/http"
	"slices"
	"strings"
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

	t.Run("in an edited post", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})

		if w := h.do("PATCH", "/api/v1/posts/p1", text, "Authorization", "Bearer tok"); w.Code != http.StatusOK {
			t.Fatalf("edit: %d %s", w.Code, w.Body)
		}
		if !slices.Equal(h.users.named, asked) || !slices.Equal(h.posts.mentioned, want) {
			t.Fatalf("asked about %v, passed %v", h.users.named, h.posts.mentioned)
		}
	})

	t.Run("in a repost", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}

		if w := h.do("POST", "/api/v1/posts/p1/reposts", text, "Authorization", "Bearer tok"); w.Code != http.StatusCreated {
			t.Fatalf("repost: %d %s", w.Code, w.Body)
		}
		if !slices.Equal(h.users.named, asked) || !slices.Equal(h.posts.repost.Mentions, want) {
			t.Fatalf("asked about %v, passed %v", h.users.named, h.posts.repost.Mentions)
		}
	})

	t.Run("in a comment under a photo", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}

		if w := h.do("POST", "/api/v1/album-photos/ph6/comments", text, "Authorization", "Bearer tok"); w.Code != http.StatusCreated {
			t.Fatalf("comment: %d %s", w.Code, w.Body)
		}
		if !slices.Equal(h.users.named, asked) || !slices.Equal(h.albums.mentioned, want) {
			t.Fatalf("asked about %v, passed %v", h.users.named, h.albums.mentioned)
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

// Which of the words after "@" are pages anyone may ask.
func TestKnownNicknames(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})

	w := h.do("GET", "/api/v1/nicknames?nickname=Ann&nickname=nobody&nickname=bob", "")
	got := decode[struct {
		Nicknames []string `json:"nicknames"`
	}](t, w)
	if w.Code != http.StatusOK || !slices.Equal(got.Nicknames, []string{"ann", "bob"}) {
		t.Fatalf("%d %s", w.Code, w.Body)
	}

	if w = h.do("GET", "/api/v1/nicknames", ""); w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != `{"nicknames":[]}` {
		t.Fatalf("no nicknames: %d %s", w.Code, w.Body)
	}

	if w = h.do("GET", "/api/v1/nicknames?"+strings.Repeat("nickname=a&", 101), ""); w.Code != http.StatusBadRequest {
		t.Fatalf("too many: %d %s", w.Code, w.Body)
	}
}
