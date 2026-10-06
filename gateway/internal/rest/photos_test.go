package rest_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

func TestUploadPhoto(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("POST", "/api/v1/me/photos", "jpeg-bytes", "Authorization", "Bearer tok", "Content-Type", "image/jpeg")
	got := decode[map[string]string](t, w)

	if w.Code != http.StatusCreated || string(h.media.uploaded) != "jpeg-bytes" {
		t.Fatalf("%d %v", w.Code, got)
	}
	if got["id"] != "ph9" || got["large"] != "/media/photos/u1/ph9/large.jpg" || got["small"] != "/media/photos/u1/ph9/small.jpg" {
		t.Fatalf("photo = %v", got)
	}

	big := newHarness(rest.Config{})
	if w = big.do("POST", "/api/v1/me/photos", strings.Repeat("x", 8<<20+1), "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest {
		t.Fatalf("larger than 8 MB: %d", w.Code)
	}

	if w = newHarness(rest.Config{}).do("POST", "/api/v1/me/photos", "x"); w.Code != http.StatusUnauthorized {
		t.Fatalf("without token: %d", w.Code)
	}
}

func TestCreatePostWithPhotos(t *testing.T) {
	t.Parallel()

	t.Run("own photos", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}

		w := h.do("POST", "/api/v1/me/posts", `{"text":"","photos":["ph2","ph1"]}`, "Authorization", "Bearer tok")
		got := decode[struct {
			Photos []map[string]string `json:"photos"`
		}](t, w)

		if w.Code != http.StatusCreated || len(h.posts.photos) != 2 {
			t.Fatalf("%d %+v", w.Code, got)
		}
		if len(got.Photos) != 2 || got.Photos[0]["id"] != "ph2" || got.Photos[0]["small"] != "/media/photos/u1/ph2/small.jpg" {
			t.Fatalf("photos = %+v; in the order attached, under the author", got.Photos)
		}
	})

	t.Run("someone else's photo", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}

		w := h.do("POST", "/api/v1/me/posts", `{"text":"смотрите","photos":["ph1","stolen"]}`, "Authorization", "Bearer tok")
		if got := decode[errorBody](t, w); w.Code != http.StatusBadRequest || got.Error.Message != "photo not found" || h.posts.created != "" {
			t.Fatalf("%d %+v", w.Code, got)
		}
	})
}

func TestServePhoto(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})

	w := h.do("GET", "/media/photos/u1/ph1/small.jpg", "")
	if w.Code != http.StatusOK || w.Body.String() != "small-photo" || w.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("%d %q", w.Code, w.Body.String())
	}
	if cc := w.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Fatalf("Cache-Control = %q", cc)
	}

	for _, path := range []string{"/media/photos/u1/ph1/huge.jpg", "/media/photos/u2/ph1/large.jpg"} {
		if w = h.do("GET", path, ""); w.Code != http.StatusNotFound {
			t.Fatalf("%s: %d", path, w.Code)
		}
	}
}
