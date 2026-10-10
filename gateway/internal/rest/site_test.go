package rest_test

import (
	"net/http"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

func TestAbout(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})

	// Anyone may ask, without a token.
	w := h.do("GET", "/api/v1/about", "")
	got := decode[map[string]int](t, w)
	if w.Code != http.StatusOK || got["users"] != 12 || got["online"] != 3 || got["clubs"] != 5 || got["posts"] != 40 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}

	h.clubs.err = failure.New(failure.Unavailable, "service unavailable")
	if w = h.do("GET", "/api/v1/about", ""); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("a service down: %d", w.Code)
	}
}

func TestUpdateInterface(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	h.users.profile = ann

	type meJSON struct {
		Interface struct {
			AvatarStyle string   `json:"avatarStyle"`
			MainPage    string   `json:"mainPage"`
			HiddenMenu  []string `json:"hiddenMenu"`
		} `json:"interface"`
	}

	w := h.do("PUT", "/api/v1/me/interface", `{"avatarStyle":"round","mainPage":"feed","hiddenMenu":["events"]}`, "Authorization", "Bearer tok")
	got := decode[meJSON](t, w)
	if w.Code != http.StatusOK || got.Interface.AvatarStyle != "round" || got.Interface.MainPage != "feed" ||
		len(got.Interface.HiddenMenu) != 1 || got.Interface.HiddenMenu[0] != "events" || h.users.token != "tok" {
		t.Fatalf("%d %s", w.Code, w.Body)
	}

	// A whole menu is an empty list, never null.
	w = h.do("PUT", "/api/v1/me/interface", `{"avatarStyle":"square","mainPage":"page"}`, "Authorization", "Bearer tok")
	if got = decode[meJSON](t, w); w.Code != http.StatusOK || got.Interface.HiddenMenu == nil {
		t.Fatalf("a whole menu: %d %s", w.Code, w.Body)
	}

	if w = h.do("PUT", "/api/v1/me/interface", `{"avatarStyle":"round"}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("without a token: %d", w.Code)
	}

	h.users.err = failure.New(failure.InvalidArgument, "invalid avatar style")
	if w = h.do("PUT", "/api/v1/me/interface", `{"avatarStyle":"oval","mainPage":"page"}`, "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest {
		t.Fatalf("an unknown style: %d", w.Code)
	}
}
