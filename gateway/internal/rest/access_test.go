package rest_test

import (
	"net/http"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/rest"
)

func TestPagePrivacy(t *testing.T) {
	t.Parallel()

	// "Кому в интернете видно мою страницу": the page is a stub, and none
	// of its parts are shown.
	h := newHarness(rest.Config{})
	h.users.hiddenPage = true

	w := h.do("GET", "/api/v1/users/bob", "", "Authorization", "Bearer tok")
	got := decode[struct {
		Name       string          `json:"name"`
		PageHidden bool            `json:"pageHidden"`
		Access     map[string]bool `json:"access"`
	}](t, w)
	if w.Code != http.StatusOK || !got.PageHidden || got.Name != "Bob" || got.Access["photos"] || h.users.profileToken != "tok" {
		t.Fatalf("%d %+v, token %q", w.Code, got, h.users.profileToken)
	}

	for _, path := range []string{"posts", "followers", "following", "clubs", "albums"} {
		if w = h.do("GET", "/api/v1/users/bob/"+path, ""); w.Code != http.StatusForbidden || decode[errorBody](t, w).Error.Message != "this page is hidden" {
			t.Fatalf("%s of a hidden page: %d %s", path, w.Code, w.Body)
		}
	}
	if w = h.do("GET", "/api/v1/albums/a1", ""); w.Code != http.StatusNotFound {
		t.Fatalf("an album of a hidden page: %d", w.Code)
	}
}

func TestPagePartsPrivacy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		hide    func(h *harness)
		path    string
		refusal string
	}{
		{"photos", func(h *harness) { h.users.hiddenPhotos = true }, "albums", "this user's photos are hidden"},
		{"followers", func(h *harness) { h.users.hiddenFriends = true }, "followers", "this user's follows are hidden"},
		{"following", func(h *harness) { h.users.hiddenFriends = true }, "following", "this user's follows are hidden"},
		{"clubs", func(h *harness) { h.users.hiddenClubs = true }, "clubs", "this user's clubs are hidden"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(rest.Config{})
			if w := h.do("GET", "/api/v1/users/ann/"+tt.path, ""); w.Code != http.StatusOK {
				t.Fatalf("open: %d %s", w.Code, w.Body)
			}

			tt.hide(&h)
			w := h.do("GET", "/api/v1/users/ann/"+tt.path, "", "Authorization", "Bearer tok")
			if w.Code != http.StatusForbidden || decode[errorBody](t, w).Error.Message != tt.refusal || h.users.accessAsked != "u1" || h.users.token != "tok" {
				t.Fatalf("hidden: %d %s, asked about %q", w.Code, w.Body, h.users.accessAsked)
			}

			// The wall is not a part of these settings.
			if w = h.do("GET", "/api/v1/users/ann/posts", ""); w.Code != http.StatusOK {
				t.Fatalf("the wall: %d", w.Code)
			}
		})
	}
}

func TestProfilePresence(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	type presence struct {
		LastSeenAt *string `json:"lastSeenAt"`
		Online     bool    `json:"online"`
	}

	if got := decode[presence](t, h.do("GET", "/api/v1/users/bob", "")); got.LastSeenAt == nil || *got.LastSeenAt != "2026-10-09T12:00:00Z" || !got.Online {
		t.Fatalf("bob: %+v", got)
	}
	if got := decode[presence](t, h.do("GET", "/api/v1/users/ann", "")); got.LastSeenAt != nil || got.Online {
		t.Fatalf("never seen: %+v", got)
	}
}

func TestProfileAccess(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	h.users.hiddenInfo, h.users.hiddenFriends = true, true

	w := h.do("GET", "/api/v1/users/dina", "")
	got := decode[struct {
		InfoHidden bool            `json:"infoHidden"`
		Contacts   map[string]any  `json:"contacts"`
		Access     map[string]bool `json:"access"`
	}](t, w)
	if w.Code != http.StatusOK || !got.InfoHidden || got.Contacts["city"] != "" ||
		!got.Access["photos"] || got.Access["friends"] || !got.Access["clubs"] || h.users.accessAsked != "u4" {
		t.Fatalf("%d %+v", w.Code, got)
	}

	if w = h.do("GET", "/api/v1/albums/a1", ""); w.Code != http.StatusOK {
		t.Fatalf("an album with the photos open: %d", w.Code)
	}
	h.users.hiddenPhotos = true
	if w = h.do("GET", "/api/v1/albums/a1", ""); w.Code != http.StatusNotFound {
		t.Fatalf("an album with the photos hidden: %d", w.Code)
	}
}
