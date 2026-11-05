package rest_test

import (
	"net/http"
	"slices"
	"testing"
	"time"

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

// The numbers are kept for a while: the services are not asked to count
// their tables for every visitor of the page.
func TestAbout_Kept(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{StatisticsTTL: time.Hour})

	// A count that failed keeps nothing.
	h.clubs.err = failure.New(failure.Unavailable, "service unavailable")
	if w := h.do("GET", "/api/v1/about", ""); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("a service down: %d", w.Code)
	}

	h.clubs.err = nil
	if w := h.do("GET", "/api/v1/about", ""); w.Code != http.StatusOK {
		t.Fatalf("counted: %d %s", w.Code, w.Body)
	}

	// The second answer is the first: a service that is down by now is
	// not asked.
	h.clubs.err = failure.New(failure.Unavailable, "service unavailable")
	w := h.do("GET", "/api/v1/about", "")
	if got := decode[map[string]int](t, w); w.Code != http.StatusOK || got["clubs"] != 5 || got["posts"] != 40 {
		t.Fatalf("kept: %d %s", w.Code, w.Body)
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
			Theme       string   `json:"theme"`
			HideRating  bool     `json:"hideRating"`
			NSFW        string   `json:"nsfw"`
			PostView    string   `json:"postView"`
			Timezone    *int     `json:"timezone"`
		} `json:"interface"`
	}

	body := `{"avatarStyle":"round","mainPage":"feed","hiddenMenu":["events"],"theme":"dark","hideRating":true,"nsfw":"show","postView":"microblog","timezone":180}`
	w := h.do("PUT", "/api/v1/me/interface", body, "Authorization", "Bearer tok")
	got := decode[meJSON](t, w).Interface
	if w.Code != http.StatusOK || got.AvatarStyle != "round" || got.MainPage != "feed" ||
		len(got.HiddenMenu) != 1 || got.HiddenMenu[0] != "events" || h.users.token != "tok" ||
		got.Theme != "dark" || !got.HideRating || got.NSFW != "show" || got.PostView != "microblog" || got.Timezone == nil || *got.Timezone != 180 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}

	// A whole menu is an empty list, never null; the browser's clock is.
	w = h.do("PUT", "/api/v1/me/interface", `{"avatarStyle":"square","mainPage":"page","timezone":null}`, "Authorization", "Bearer tok")
	if got = decode[meJSON](t, w).Interface; w.Code != http.StatusOK || got.HiddenMenu == nil || got.Timezone != nil {
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

// The mark of an adult post goes to the post service and comes back; the
// news leave such posts out for whoever asked.
func TestNSFW(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	h.users.profile = ann

	w := h.do("POST", "/api/v1/me/posts", `{"text":"для взрослых","nsfw":true}`, "Authorization", "Bearer tok")
	if got := decode[struct {
		NSFW bool `json:"nsfw"`
	}](t, w); w.Code != http.StatusCreated || !got.NSFW || !h.posts.newPost.NSFW {
		t.Fatalf("a marked post: %d %s", w.Code, w.Body)
	}

	if w = h.do("POST", "/api/v1/me/posts", `{"text":"котики"}`, "Authorization", "Bearer tok"); w.Code != http.StatusCreated || h.posts.newPost.NSFW {
		t.Fatalf("a plain post: %d %s", w.Code, w.Body)
	}

	if w = h.do("GET", "/api/v1/feed", "", "Authorization", "Bearer tok"); w.Code != http.StatusOK || h.posts.hideNSFW {
		t.Fatalf("the news by default: %d, hidden %v", w.Code, h.posts.hideNSFW)
	}

	h.users.ui.NSFW = "hide"
	if w = h.do("GET", "/api/v1/feed", "", "Authorization", "Bearer tok"); w.Code != http.StatusOK || !h.posts.hideNSFW {
		t.Fatalf("the news of whoever hides adult posts: %d, hidden %v", w.Code, h.posts.hideNSFW)
	}

	// The news, of everyone too, leave out whom the reader blacklisted.
	h.users.blockedIDs = []string{"u9"}
	for _, path := range []string{"/api/v1/feed", "/api/v1/feed?type=all"} {
		if w = h.do("GET", path, "", "Authorization", "Bearer tok"); w.Code != http.StatusOK || !slices.Equal(h.posts.hidden, []string{"u9"}) {
			t.Fatalf("%s with a blacklist: %d, hidden %v", path, w.Code, h.posts.hidden)
		}
	}
}
