package rest_test

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

type pollJSON struct {
	Title   string `json:"title"`
	Options []struct {
		Name  string `json:"name"`
		Votes int    `json:"votes"`
	} `json:"options"`
	Voters    int     `json:"voters"`
	Anonymous bool    `json:"anonymous"`
	Multiple  bool    `json:"multiple"`
	Locked    bool    `json:"locked"`
	EndsAt    *string `json:"endsAt"`
	Ended     bool    `json:"ended"`
	Voted     []int   `json:"voted"`
}

type polledPost struct {
	ID     string    `json:"id"`
	Poll   *pollJSON `json:"poll"`
	Repost *struct {
		ID   string    `json:"id"`
		Poll *pollJSON `json:"poll"`
	} `json:"repost"`
}

func TestCreatePostWithPoll(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}

	body := `{"poll":{"title":"Кошки или собаки?","options":["кошки","собаки"],"anonymous":true,"multiple":true,"locked":true,"days":3}}`
	w := h.do("POST", "/api/v1/me/posts", body, "Authorization", "Bearer tok")
	got := decode[polledPost](t, w)

	want := &model.NewPoll{Title: "Кошки или собаки?", Options: []string{"кошки", "собаки"}, Anonymous: true, Multiple: true, Locked: true, Days: 3}
	if w.Code != http.StatusCreated || !reflect.DeepEqual(h.posts.newPost.Poll, want) {
		t.Fatalf("%d %s; asked %+v", w.Code, w.Body, h.posts.newPost.Poll)
	}

	// The post comes back with its poll, as the author sees it.
	if got.Poll == nil || got.Poll.Title != "Кошки или собаки?" || len(got.Poll.Options) != 2 || got.Poll.Options[0].Votes != 2 ||
		got.Poll.Voters != 3 || !got.Poll.Locked || got.Poll.EndsAt == nil || *got.Poll.EndsAt != "2026-10-06T15:30:00Z" ||
		!reflect.DeepEqual(got.Poll.Voted, []int{1}) {
		t.Fatalf("poll = %+v", got.Poll)
	}

	// A post without a poll asks the post service for none.
	h = newHarness(rest.Config{})
	h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}

	w = h.do("POST", "/api/v1/me/posts", `{"text":"привет"}`, "Authorization", "Bearer tok")
	if got = decode[polledPost](t, w); w.Code != http.StatusCreated || got.Poll != nil || h.posts.newPost.Poll != nil || h.posts.pollsAsked != nil {
		t.Fatalf("%d %+v; polls asked %v", w.Code, got, h.posts.pollsAsked)
	}

	// On the wall of a community too.
	h = newHarness(rest.Config{})
	h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}

	w = h.do("POST", "/api/v1/clubs/cats/posts", `{"text":"а вы?","poll":{"title":"Да?","options":["да"]}}`, "Authorization", "Bearer tok")
	if got = decode[polledPost](t, w); w.Code != http.StatusCreated || got.Poll == nil || h.posts.newPost.Poll == nil || h.posts.newPost.Poll.Title != "Да?" {
		t.Fatalf("club: %d %s", w.Code, w.Body)
	}

	// An edit takes no poll.
	w = h.do("PATCH", "/api/v1/posts/p1", `{"text":"x","poll":{"title":"Да?","options":["да"]}}`, "Authorization", "Bearer tok")
	if w.Code != http.StatusBadRequest || h.posts.edited != "" {
		t.Fatalf("edit with a poll: %d %s", w.Code, w.Body)
	}
}

func TestPostsWithPolls(t *testing.T) {
	t.Parallel()

	ann, bob := model.Author{ID: "u1"}, model.Author{ID: "u2"}

	h := newHarness(rest.Config{})
	h.posts.polled = true
	h.posts.wallPosts = []model.Post{
		{ID: "p6", Author: ann, Wall: ann, Polled: true},
		{ID: "p5", Author: bob, Wall: ann, Text: "смотрите", RepostOf: "p1"},
		{ID: "p3", Author: ann, Wall: ann, Text: "без опроса"},
	}

	// The polls of a page, of its posts and of the posts they share, are
	// read in one request; a guest has chosen nothing.
	w := h.do("GET", "/api/v1/users/ann/posts", "")
	page := decode[struct {
		Posts []polledPost `json:"posts"`
	}](t, w)

	if w.Code != http.StatusOK || len(page.Posts) != 3 || len(h.posts.pollsAsked) != 1 || len(h.posts.pollsAsked[0]) != 2 {
		t.Fatalf("%d %s; polls asked %v", w.Code, w.Body, h.posts.pollsAsked)
	}

	own, repost, plain := page.Posts[0], page.Posts[1], page.Posts[2]
	if own.Poll == nil || own.Poll.Voted == nil || len(own.Poll.Voted) != 0 {
		t.Fatalf("the poll of a post for a guest = %+v", own.Poll)
	}
	if repost.Poll != nil || repost.Repost == nil || repost.Repost.Poll == nil || repost.Repost.Poll.Title == "" {
		t.Fatalf("the poll of a shared post = %+v", repost)
	}
	if plain.Poll != nil {
		t.Fatalf("a post without a poll got one: %+v", plain.Poll)
	}

	// The page of a post tells the viewer's choice.
	w = h.do("GET", "/api/v1/posts/p1", "", "Authorization", "Bearer tok")
	if got := decode[polledPost](t, w); w.Code != http.StatusOK || got.Poll == nil || !reflect.DeepEqual(got.Poll.Voted, []int{1}) {
		t.Fatalf("post: %d %s", w.Code, w.Body)
	}
}

func TestVotePoll(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})

	for _, tt := range []struct{ method, body string }{{"PUT", `{"options":[0]}`}, {"DELETE", ""}} {
		if w := h.do(tt.method, "/api/v1/posts/p1/poll/vote", tt.body); w.Code != http.StatusUnauthorized {
			t.Fatalf("%s without a token: %d", tt.method, w.Code)
		}
	}

	w := h.do("PUT", "/api/v1/posts/p1/poll/vote", `{"options":[1,0]}`, "Authorization", "Bearer tok")
	got := decode[pollJSON](t, w)
	if w.Code != http.StatusOK || h.posts.votedIn != "p1" || !reflect.DeepEqual(h.posts.voted, []int{1, 0}) || h.posts.token != "tok" ||
		!reflect.DeepEqual(got.Voted, []int{1}) || got.Voters != 3 {
		t.Fatalf("vote: %d %s; voted %v in %q", w.Code, w.Body, h.posts.voted, h.posts.votedIn)
	}

	if w = h.do("PUT", "/api/v1/posts/p1/poll/vote", `{"options":"all"}`, "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest {
		t.Fatalf("a malformed vote: %d", w.Code)
	}

	w = h.do("DELETE", "/api/v1/posts/p1/poll/vote", "", "Authorization", "Bearer tok")
	if got = decode[pollJSON](t, w); w.Code != http.StatusOK || h.posts.votedIn != "-p1" || len(got.Voted) != 0 {
		t.Fatalf("retract: %d %s", w.Code, w.Body)
	}
}

func TestPollVoters(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})

	// Anyone sees them; a deleted page is left out, counted.
	w := h.do("GET", "/api/v1/posts/p1/poll/voters?option=1&cursor=v1&limit=6", "")
	got := decode[struct {
		Users []struct {
			Nickname string `json:"nickname"`
		} `json:"users"`
		NextCursor string `json:"nextCursor"`
		Total      int    `json:"total"`
	}](t, w)

	if w.Code != http.StatusOK || len(got.Users) != 1 || got.Users[0].Nickname != "bob" || got.Total != 2 || got.NextCursor != "v2" ||
		h.posts.likersOf != "poll:p1:1" || h.posts.cursor != "v1" || h.posts.limit != 6 {
		t.Fatalf("%d %+v (asked %q)", w.Code, got, h.posts.likersOf)
	}

	for _, q := range []string{"", "?option=", "?option=first"} {
		if w = h.do("GET", "/api/v1/posts/p1/poll/voters"+q, ""); w.Code != http.StatusBadRequest {
			t.Fatalf("%q: %d", q, w.Code)
		}
	}
}
