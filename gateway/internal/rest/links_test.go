package rest_test

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

// fakePreviews knows the pages in pages; others tell nothing.
type fakePreviews struct {
	pages map[string]model.LinkPreview
	read  []string
}

func (f *fakePreviews) Read(_ context.Context, url string) (model.LinkPreview, error) {
	f.read = append(f.read, url)

	p, ok := f.pages[url]
	if !ok {
		return model.LinkPreview{}, errors.New("no preview")
	}

	return p, nil
}

func TestLinkCards(t *testing.T) {
	t.Parallel()

	newAnn := func() harness {
		h := newHarness(rest.Config{})
		h.users.profile = model.Profile{Name: "Ann", Nickname: "ann", Gender: "female"}
		h.previews.pages = map[string]model.LinkPreview{
			"https://cats.example/a": {URL: "https://cats.example/a", Title: "Кошки", Description: "Всё о кошках", Site: "cats.example", Image: []byte("jpeg")},
			"https://plain.example":  {URL: "https://plain.example", Title: "Plain", Site: "plain.example"},
		}
		return h
	}

	t.Run("the first link of a post gets a card with its picture", func(t *testing.T) {
		t.Parallel()

		h := newAnn()
		w := h.do("POST", "/api/v1/me/posts", `{"text":"Смотрите: https://cats.example/a, и https://plain.example"}`, "Authorization", "Bearer tok")
		got := decode[map[string]any](t, w)

		link, _ := got["link"].(map[string]any)
		photo, _ := link["photo"].(map[string]any)
		if w.Code != http.StatusCreated || link["url"] != "https://cats.example/a" || link["title"] != "Кошки" ||
			link["description"] != "Всё о кошках" || link["site"] != "cats.example" || photo["large"] != "/media/photos/u1/ph9/large.jpg" {
			t.Fatalf("%d %v", w.Code, got)
		}

		// The picture is uploaded, taken and kept for good, as the post
		// will delete it with itself.
		if string(h.media.uploaded) != "jpeg" || !slices.Equal(h.media.claimed, []string{"ph9"}) ||
			!slices.Equal(h.media.published, []string{"u1/ph9"}) || h.posts.newPost.Link.PhotoID != "ph9" {
			t.Fatalf("media %+v, link %+v", h.media, h.posts.newPost.Link)
		}
		if !slices.Equal(h.previews.read, []string{"https://cats.example/a"}) {
			t.Fatalf("read = %v", h.previews.read)
		}
	})

	t.Run("a card without a picture", func(t *testing.T) {
		t.Parallel()

		h := newAnn()
		w := h.do("POST", "/api/v1/me/posts", `{"text":"https://plain.example"}`, "Authorization", "Bearer tok")
		got := decode[map[string]any](t, w)

		if link, _ := got["link"].(map[string]any); w.Code != http.StatusCreated || link["title"] != "Plain" || link["photo"] != nil || h.media.uploaded != nil {
			t.Fatalf("%d %v", w.Code, got)
		}
	})

	t.Run("a page that tells nothing makes no card", func(t *testing.T) {
		t.Parallel()

		h := newAnn()
		w := h.do("POST", "/api/v1/me/posts", `{"text":"https://nothing.example"}`, "Authorization", "Bearer tok")
		got := decode[map[string]any](t, w)

		if w.Code != http.StatusCreated || got["link"] != nil || h.posts.newPost.Link != nil {
			t.Fatalf("%d %v", w.Code, got)
		}
	})

	t.Run("no link, no reading", func(t *testing.T) {
		t.Parallel()

		h := newAnn()
		if w := h.do("POST", "/api/v1/me/posts", `{"text":"привет"}`, "Authorization", "Bearer tok"); w.Code != http.StatusCreated || len(h.previews.read) != 0 {
			t.Fatalf("%d, read %v", w.Code, h.previews.read)
		}
	})

	t.Run("a refused post deletes the picture of its card", func(t *testing.T) {
		t.Parallel()

		h := newAnn()
		h.posts.err = failure.New(failure.TooManyRequests, "too many requests, try again later")

		if w := h.do("POST", "/api/v1/me/posts", `{"text":"https://cats.example/a"}`, "Authorization", "Bearer tok"); w.Code != http.StatusTooManyRequests {
			t.Fatalf("status = %d", w.Code)
		}
		if !slices.Equal(h.media.unpublished, []string{"u1/ph9"}) {
			t.Fatalf("unpublished = %v", h.media.unpublished)
		}
	})

	t.Run("on the wall of a community", func(t *testing.T) {
		t.Parallel()

		h := newAnn()
		w := h.do("POST", "/api/v1/clubs/cats/posts", `{"text":"https://plain.example"}`, "Authorization", "Bearer tok")
		got := decode[map[string]any](t, w)

		if link, _ := got["link"].(map[string]any); w.Code != http.StatusCreated || link["title"] != "Plain" {
			t.Fatalf("%d %v", w.Code, got)
		}
	})
}
