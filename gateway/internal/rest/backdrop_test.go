package rest_test

import (
	"net/http"
	"slices"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

func TestClubBackdrop(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	if w := h.do("PUT", "/api/v1/clubs/cats/backdrop/left", "jpeg", "Authorization", "Bearer tok"); w.Code != http.StatusForbidden || h.media.uploaded != nil {
		t.Fatalf("a member uploads: %d", w.Code)
	}

	h.clubs.role = "admin"
	h.clubs.backdrop = [2]string{"", "kp0"}
	w := h.do("PUT", "/api/v1/clubs/cats/backdrop/left", "jpeg", "Authorization", "Bearer tok")
	got := decode[struct {
		Backdrop *struct {
			Left, Right *struct {
				Large string `json:"large"`
			}
		} `json:"backdrop"`
	}](t, w)
	if w.Code != http.StatusOK || got.Backdrop == nil || got.Backdrop.Left.Large != "/media/photos/k1/kp1/large.jpg" || got.Backdrop.Right.Large != "/media/photos/k1/kp0/large.jpg" {
		t.Fatalf("upload: %d %+v", w.Code, got.Backdrop)
	}
	if h.media.clubAvatarOf != "k1" || !slices.Equal(h.media.keptPhotos, []string{"kp1", "kp0"}) {
		t.Fatalf("clean-up kept %v", h.media.keptPhotos)
	}

	h.media.uploaded = nil
	if w = h.do("PUT", "/api/v1/clubs/cats/backdrop/top", "jpeg", "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest || h.media.uploaded != nil {
		t.Fatalf("a bad side: %d", w.Code)
	}

	w = h.do("DELETE", "/api/v1/clubs/cats/backdrop", "", "Authorization", "Bearer tok")
	if got = decode[struct {
		Backdrop *struct {
			Left, Right *struct {
				Large string `json:"large"`
			}
		} `json:"backdrop"`
	}](t, w); w.Code != http.StatusOK || got.Backdrop != nil || len(h.media.keptPhotos) != 0 {
		t.Fatalf("delete: %d %+v, kept %v", w.Code, got.Backdrop, h.media.keptPhotos)
	}
}

func TestBackdrop(t *testing.T) {
	t.Parallel()

	type backdrop struct {
		Backdrop *struct {
			Left, Right *struct {
				Large string `json:"large"`
			}
		} `json:"backdrop"`
	}

	h := newHarness(rest.Config{})
	h.users.backdrop.Left = "ph0"

	w := h.do("PUT", "/api/v1/me/backdrop/left", "jpeg", "Authorization", "Bearer tok")
	if got := decode[backdrop](t, w); w.Code != http.StatusOK || got.Backdrop == nil || got.Backdrop.Left.Large != "/media/photos/u1/ph9/large.jpg" || got.Backdrop.Right != nil {
		t.Fatalf("upload: %d %s", w.Code, w.Body)
	}
	// Kept for good before the page shows it; what it replaced goes.
	if string(h.media.uploaded) != "jpeg" || !slices.Equal(h.media.published, []string{"u1/ph9"}) || !slices.Equal(h.media.unpublished, []string{"u1/ph0"}) {
		t.Fatalf("published %v, deleted %v", h.media.published, h.media.unpublished)
	}

	h.media.uploaded = nil
	if w = h.do("PUT", "/api/v1/me/backdrop/top", "jpeg", "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest || h.media.uploaded != nil {
		t.Fatalf("a bad side: %d", w.Code)
	}
	if w = h.do("PUT", "/api/v1/me/backdrop/left", "jpeg"); w.Code != http.StatusUnauthorized {
		t.Fatalf("without a token: %d", w.Code)
	}

	h.users.backdrop.Right = "ph5"
	w = h.do("DELETE", "/api/v1/me/backdrop", "", "Authorization", "Bearer tok")
	if got := decode[backdrop](t, w); w.Code != http.StatusOK || got.Backdrop != nil || !slices.Equal(h.media.unpublished, []string{"u1/ph0", "u1/ph9,ph5"}) {
		t.Fatalf("delete: %d %s, deleted %v", w.Code, w.Body, h.media.unpublished)
	}

	// The page shows it to anyone.
	h.users.backdrop = model.Backdrop{}
	if got := decode[backdrop](t, h.do("GET", "/api/v1/users/bob", "")); got.Backdrop == nil || got.Backdrop.Right.Large != "/media/photos/u2/pb2/large.jpg" {
		t.Fatalf("bob's page: %+v", got.Backdrop)
	}
}
