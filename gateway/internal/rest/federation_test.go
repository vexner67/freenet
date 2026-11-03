package rest_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

type fakeFederation struct {
	token    string
	open     bool
	resource string
	read     string
	received model.InboxRequest
	err      error

	address string
	removed string
	news    *model.FediverseNews
	remote  []model.FediversePost
	newsErr error
	picture model.Picture
	likes   int
	liked   string
	cursor  string
	written string
	deleted string
}

func (f *fakeFederation) status() model.Federation {
	return model.Federation{Open: f.open, Followers: 3, Domain: "freenet.example"}
}

func (f *fakeFederation) Federation(_ context.Context, token string) (model.Federation, error) {
	f.token = token
	return f.status(), f.err
}

func (f *fakeFederation) Open(_ context.Context, token string) (model.Federation, error) {
	f.token, f.open = token, true
	return f.status(), f.err
}

func (f *fakeFederation) Close(_ context.Context, token string) (model.Federation, error) {
	f.token, f.open = token, false
	return f.status(), f.err
}

func (f *fakeFederation) Finger(_ context.Context, resource string) ([]byte, error) {
	f.resource = resource
	return []byte(`{"subject":"acct:anya@freenet.example"}`), f.err
}

func (f *fakeFederation) Actor(_ context.Context, id string) (model.FederationDocument, error) {
	f.read = "actor " + id
	return model.FederationDocument{Body: []byte(`{"type":"Person"}`), PageURL: "https://freenet.example/anya"}, f.err
}

func (f *fakeFederation) Note(_ context.Context, id string) (model.FederationDocument, error) {
	f.read = "note " + id
	return model.FederationDocument{Body: []byte(`{"type":"Note"}`), PageURL: "https://freenet.example/post/" + id}, f.err
}

func (f *fakeFederation) Followers(_ context.Context, id string) ([]byte, error) {
	f.read = "followers " + id
	return []byte(`{"totalItems":3}`), f.err
}

func (f *fakeFederation) Outbox(_ context.Context, id string) ([]byte, error) {
	f.read = "outbox " + id
	return []byte(`{"totalItems":0}`), f.err
}

func (f *fakeFederation) Receive(_ context.Context, in model.InboxRequest) error {
	f.received = in
	return f.err
}

func TestFederationSettings(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})

	if w := h.do("GET", "/api/v1/me/federation", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("without a token: %d", w.Code)
	}

	w := h.do("PUT", "/api/v1/me/federation", "", "Authorization", "Bearer tok")
	if w.Code != http.StatusOK || h.federation.token != "tok" {
		t.Fatalf("open: %d %s", w.Code, w.Body)
	}

	got := decode[struct {
		Open      bool   `json:"open"`
		Followers int    `json:"followers"`
		Domain    string `json:"domain"`
	}](t, w)
	if !got.Open || got.Followers != 3 || got.Domain != "freenet.example" {
		t.Fatalf("open = %+v", got)
	}

	if w = h.do("DELETE", "/api/v1/me/federation", "", "Authorization", "Bearer tok"); w.Code != http.StatusOK || h.federation.open {
		t.Fatalf("close: %d %s", w.Code, w.Body)
	}

	h.federation.err = failure.New(failure.Conflict, "the page is not open to everyone")
	if w = h.do("PUT", "/api/v1/me/federation", "", "Authorization", "Bearer tok"); w.Code != http.StatusConflict {
		t.Fatalf("hidden page: %d", w.Code)
	}
}

func TestWebFinger(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})

	w := h.do("GET", "/.well-known/webfinger?resource=acct:anya@freenet.example", "")
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "application/jrd+json" || h.federation.resource != "acct:anya@freenet.example" {
		t.Fatalf("%d %q %q", w.Code, w.Header().Get("Content-Type"), h.federation.resource)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "*" || !strings.Contains(w.Header().Get("Content-Security-Policy"), "sandbox") {
		t.Fatalf("headers = %v", w.Header())
	}

	if w = h.do("GET", "/.well-known/webfinger", ""); w.Code != http.StatusBadRequest {
		t.Fatalf("without a resource: %d", w.Code)
	}

	h.federation.err = failure.New(failure.NotFound, "not found")
	if w = h.do("GET", "/.well-known/webfinger?resource=acct:boris@freenet.example", ""); w.Code != http.StatusNotFound {
		t.Fatalf("unknown: %d", w.Code)
	}
}

func TestFederationDocuments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		path     string
		accept   string
		code     int
		read     string
		location string
	}{
		{"actor", "/ap/users/u1", "application/activity+json", http.StatusOK, "actor u1", ""},
		{"actor for a browser", "/ap/users/u1", "text/html,application/xhtml+xml", http.StatusFound, "actor u1", "https://freenet.example/anya"},
		{"note asked by Mastodon", "/ap/posts/p1", `application/ld+json; profile="https://www.w3.org/ns/activitystreams", text/html`, http.StatusOK, "note p1", ""},
		{"note for a browser", "/ap/posts/p1", "text/html", http.StatusFound, "note p1", "https://freenet.example/post/p1"},
		{"followers", "/ap/users/u1/followers", "text/html", http.StatusOK, "followers u1", ""},
		{"outbox", "/ap/users/u1/outbox", "", http.StatusOK, "outbox u1", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(rest.Config{})

			w := h.do("GET", tt.path, "", "Accept", tt.accept)
			if w.Code != tt.code || h.federation.read != tt.read || w.Header().Get("Location") != tt.location {
				t.Fatalf("%d, read %q, location %q", w.Code, h.federation.read, w.Header().Get("Location"))
			}
			if tt.code == http.StatusOK && w.Header().Get("Content-Type") != "application/activity+json" {
				t.Fatalf("Content-Type = %q", w.Header().Get("Content-Type"))
			}
		})
	}
}

func TestInbox(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})

	w := h.do("POST", "/ap/users/u1/inbox?x=1", `{"type":"Follow"}`,
		"Content-Type", "application/activity+json", "Signature", `keyId="https://m.example/u#k"`, "Date", "Mon, 02 Nov 2026 12:00:00 GMT")
	if w.Code != http.StatusAccepted {
		t.Fatalf("%d %s", w.Code, w.Body)
	}

	got := h.federation.received
	if got.Method != "POST" || got.Path != "/ap/users/u1/inbox?x=1" || got.UserID != "u1" || string(got.Body) != `{"type":"Follow"}` {
		t.Fatalf("received = %+v", got)
	}
	if got.Headers["signature"] != `keyId="https://m.example/u#k"` || got.Headers["host"] != "example.com" || got.Headers["content-type"] != "application/activity+json" {
		t.Fatalf("headers = %v", got.Headers)
	}

	if w = h.do("POST", "/ap/inbox", `{}`); w.Code != http.StatusAccepted || h.federation.received.UserID != "" {
		t.Fatalf("shared inbox: %d %+v", w.Code, h.federation.received)
	}

	if w = h.do("POST", "/ap/inbox", strings.Repeat("x", 300<<10)); w.Code != http.StatusBadRequest {
		t.Fatalf("too large: %d", w.Code)
	}

	h.federation.err = failure.New(failure.Unauthenticated, "bad signature")
	if w = h.do("POST", "/ap/inbox", `{}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("bad signature: %d", w.Code)
	}
}

func TestFederationOff(t *testing.T) {
	t.Parallel()

	handler := mustServer(rest.Config{RequestTimeout: time.Second}, rest.Backends{}, prometheus.NewRegistry()).Handler()

	for _, path := range []string{"/api/v1/me/federation", "/.well-known/webfinger?resource=acct:anya@freenet.example", "/ap/users/u1"} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", path, nil))

		if w.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, w.Code)
		}
	}
}

// The news from other servers.

func (f *fakeFederation) SiteActor(context.Context) ([]byte, error) {
	return []byte(`{"type":"Application"}`), f.err
}

func (f *fakeFederation) Lookup(_ context.Context, token, address string) (model.FediverseLookup, error) {
	f.token, f.address = token, address
	if address == "t.me/meduzalive" {
		return model.FediverseLookup{Account: meduza}, f.err
	}

	return model.FediverseLookup{Account: anna, Subscribed: true}, f.err
}

func (f *fakeFederation) Subscriptions(_ context.Context, token string) ([]model.FediverseAccount, error) {
	f.token = token
	return []model.FediverseAccount{anna}, f.err
}

func (f *fakeFederation) Subscribe(_ context.Context, token, address string) (model.FediverseAccount, error) {
	f.token, f.address = token, address
	return anna, f.err
}

func (f *fakeFederation) Unsubscribe(_ context.Context, token, id string) error {
	f.token, f.removed = token, id
	return f.err
}

func (f *fakeFederation) News(_ context.Context, token string, q model.FediverseNews) ([]model.FediversePost, error) {
	f.token, f.news = token, &q
	return f.remote, f.newsErr
}

func (f *fakeFederation) Featured(context.Context) ([]model.FediverseAccount, error) {
	return []model.FediverseAccount{anna}, f.err
}

func (f *fakeFederation) Feature(_ context.Context, address string) (model.FediverseAccount, error) {
	f.address = address
	return anna, f.err
}

func (f *fakeFederation) Unfeature(_ context.Context, id string) error {
	f.removed = id
	return f.err
}

func (f *fakeFederation) VideoCover(_ context.Context, postID string, index int) (model.Picture, error) {
	f.read = "cover " + postID + " " + strconv.Itoa(index)
	return f.picture, f.err
}

func (f *fakeFederation) PostPicture(_ context.Context, postID string, index int) (model.Picture, error) {
	f.read = "picture " + postID + " " + strconv.Itoa(index)
	return f.picture, f.err
}

func (f *fakeFederation) LinkPicture(_ context.Context, postID string) (model.Picture, error) {
	f.read = "card " + postID
	return f.picture, f.err
}

func (f *fakeFederation) LikeNews(_ context.Context, token, postID string) (int, error) {
	f.token, f.liked = token, "like "+postID
	f.likes++
	return f.likes, f.err
}

func (f *fakeFederation) UnlikeNews(_ context.Context, token, postID string) (int, error) {
	f.token, f.liked = token, "unlike "+postID
	f.likes--
	return f.likes, f.err
}

func (f *fakeFederation) NewsLikers(_ context.Context, token, postID, cursor string, limit int) (model.LikerPage, error) {
	f.token, f.liked, f.cursor = token, "likers "+postID, cursor+"/"+strconv.Itoa(limit)
	return model.LikerPage{UserIDs: []string{"x1", "x2"}, NextCursor: "c2", Total: 5}, f.err
}

func (f *fakeFederation) NewsPost(_ context.Context, token, postID string) (model.FediversePost, error) {
	f.token = token
	return model.FediversePost{ID: postID, Author: meduza, Text: "news", Comments: 2}, f.err
}

func (f *fakeFederation) CommentNews(_ context.Context, token, postID, text string) (model.FediverseComment, error) {
	f.token, f.written = token, postID+" "+text
	return model.FediverseComment{ID: "c9", PostID: postID, AuthorID: "u1", Text: text, CreatedAt: time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)}, f.err
}

func (f *fakeFederation) NewsComments(_ context.Context, token, postID, cursor string, limit int) (model.FediverseCommentPage, error) {
	f.token, f.cursor = token, postID+" "+cursor+"/"+strconv.Itoa(limit)
	return model.FediverseCommentPage{
		Comments: []model.FediverseComment{
			{ID: "c1", PostID: postID, AuthorID: "u1", Text: "мой"},
			{ID: "c2", PostID: postID, AuthorID: "x1", Text: "чужой"},
			{ID: "c3", PostID: postID, AuthorID: "gone", Text: "удалённого"},
		},
		NextCursor: "c3",
		Total:      7,
	}, f.err
}

func (f *fakeFederation) DeleteNewsComment(_ context.Context, token, id string) error {
	f.token, f.deleted = token, id
	return f.err
}

func (f *fakeFederation) AccountPicture(_ context.Context, id string) (model.Picture, error) {
	f.read = "avatar " + id
	return f.picture, f.err
}

var meduza = model.FediverseAccount{
	ID: "https://t.me/meduzalive", Handle: "@meduzalive", Name: "Медуза", PageURL: "https://t.me/meduzalive", Accepted: true, Telegram: true,
}

var anna = model.FediverseAccount{
	ID: "https://m.example/users/anna", Handle: "@anna@m.example", Name: "Anna", PageURL: "https://m.example/@anna", HasAvatar: true,
}

type fediverseAccount struct {
	ID       string `json:"id"`
	Handle   string `json:"handle"`
	URL      string `json:"url"`
	Avatar   string `json:"avatar"`
	Accepted bool   `json:"accepted"`
	Telegram bool   `json:"telegram"`
}

type fediverseLookupResponse struct {
	Account    fediverseAccount `json:"account"`
	Subscribed bool             `json:"subscribed"`
}

type fediversePosts struct {
	Posts []struct {
		ID    string `json:"id"`
		Text  string `json:"text"`
		NSFW  bool   `json:"nsfw"`
		Likes int    `json:"likes"`
		Liked bool   `json:"liked"`
		Link  *struct {
			URL   string `json:"url"`
			Title string `json:"title"`
			Site  string `json:"site"`
			Photo *struct {
				Small string `json:"small"`
			} `json:"photo"`
		} `json:"link"`
		Fediverse *struct {
			Author  fediverseAccount `json:"author"`
			URL     string           `json:"url"`
			Summary string           `json:"summary"`
			Images  []struct {
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"images"`
			Videos []struct {
				Cover    string `json:"cover"`
				Blurhash string `json:"blurhash"`
				Width    int    `json:"width"`
				Height   int    `json:"height"`
				Duration int    `json:"duration"`
			} `json:"videos"`
		} `json:"fediverse"`
	} `json:"posts"`
	NextCursor string `json:"nextCursor"`
}

func TestFeed_Fediverse(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}
	h.federation.remote = []model.FediversePost{{
		ID: "p2", URL: "https://m.example/notes/1", Author: anna, Text: "from afar", Summary: "cw", Sensitive: true,
		ImageDescriptions: []string{"a cat"}, PublishedAt: time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC),
		Videos: []model.FediverseVideo{{Blurhash: "LEHV6nWB2yk8pyo0adR*.7kCMdnj", Width: 1280, Height: 720, Duration: 84 * time.Second}},
		Likes:  4, Liked: true,
		Link: &model.FediverseLink{URL: "https://meduza.io/1", Site: "Meduza", Title: "News", HasImage: true},
	}}

	w := h.do("GET", "/api/v1/feed?type=all&cursor=p9", "", "Authorization", "Bearer tok")
	got := decode[fediversePosts](t, w)

	if w.Code != http.StatusOK || len(got.Posts) != 3 || got.Posts[1].ID != "p2" || got.NextCursor != "p1" {
		t.Fatalf("status %d, news %+v; want the remote post between p3 and p1", w.Code, got)
	}

	q := h.federation.news
	if q == nil || !q.Featured || q.Before != "p9" || q.Limit != 20 || q.HideSensitive {
		t.Fatalf("asked for %+v", q)
	}

	p := got.Posts[1]
	if p.Fediverse == nil || p.Text != "from afar" || !p.NSFW || p.Fediverse.Summary != "cw" ||
		p.Fediverse.URL != "https://m.example/notes/1" || p.Fediverse.Author.Handle != "@anna@m.example" ||
		p.Fediverse.Author.Avatar != "/media/fediverse/accounts?id=https%3A%2F%2Fm.example%2Fusers%2Fanna" ||
		len(p.Fediverse.Images) != 1 || p.Fediverse.Images[0].URL != "/media/fediverse/posts/p2/0" {
		t.Fatalf("remote post %+v", p)
	}

	if p.Likes != 4 || !p.Liked || p.Link == nil || p.Link.URL != "https://meduza.io/1" || p.Link.Site != "Meduza" ||
		p.Link.Photo == nil || p.Link.Photo.Small != "/media/fediverse/posts/p2/link" {
		t.Fatalf("likes %d %v, link %+v", p.Likes, p.Liked, p.Link)
	}

	if v := p.Fediverse.Videos; len(v) != 1 || v[0].Cover != "/media/fediverse/posts/p2/videos/0" ||
		v[0].Blurhash != "LEHV6nWB2yk8pyo0adR*.7kCMdnj" || v[0].Width != 1280 || v[0].Height != 720 || v[0].Duration != 84 {
		t.Fatalf("videos %+v", v)
	}

	// A page as long as the limit ends with the remote post.
	w = h.do("GET", "/api/v1/feed?limit=2", "", "Authorization", "Bearer tok")
	if got = decode[fediversePosts](t, w); len(got.Posts) != 2 || got.NextCursor != "p2" || h.federation.news.Featured {
		t.Fatalf("news of 2: %+v", got)
	}

	// A failing federation leaves the news of the site.
	h.federation.newsErr = failure.New(failure.Unavailable, "down")
	if w = h.do("GET", "/api/v1/feed", "", "Authorization", "Bearer tok"); w.Code != http.StatusOK || len(decode[fediversePosts](t, w).Posts) != 2 {
		t.Fatalf("news without federation: %d", w.Code)
	}
}

func TestFediverseSubscriptions(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})

	w := h.do("GET", "/api/v1/fediverse/lookup?address=%40anna%40m.example", "", "Authorization", "Bearer tok")
	if w.Code != http.StatusOK || h.federation.address != "@anna@m.example" || h.federation.token != "tok" ||
		!strings.Contains(w.Body.String(), `"subscribed":true`) {
		t.Fatalf("lookup: %d %s", w.Code, w.Body)
	}

	w = h.do("GET", "/api/v1/fediverse/lookup?address=t.me%2Fmeduzalive", "", "Authorization", "Bearer tok")
	if acc := decode[fediverseLookupResponse](t, w).Account; w.Code != http.StatusOK || !acc.Telegram || acc.Handle != "@meduzalive" ||
		acc.URL != "https://t.me/meduzalive" || !acc.Accepted {
		t.Fatalf("lookup of a channel: %d %s", w.Code, w.Body)
	}

	w = h.do("POST", "/api/v1/me/fediverse", `{"address":"anna@m.example"}`, "Authorization", "Bearer tok")
	if acc := decode[fediverseAccount](t, w); w.Code != http.StatusOK || acc.URL != "https://m.example/@anna" || h.federation.address != "anna@m.example" {
		t.Fatalf("subscribe: %d %+v", w.Code, acc)
	}

	w = h.do("GET", "/api/v1/me/fediverse", "", "Authorization", "Bearer tok")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"handle":"@anna@m.example"`) {
		t.Fatalf("subscriptions: %d %s", w.Code, w.Body)
	}

	w = h.do("DELETE", "/api/v1/me/fediverse?id=https%3A%2F%2Fm.example%2Fusers%2Fanna", "", "Authorization", "Bearer tok")
	if w.Code != http.StatusNoContent || h.federation.removed != anna.ID {
		t.Fatalf("unsubscribe: %d, removed %q", w.Code, h.federation.removed)
	}

	if w = h.do("GET", "/api/v1/me/fediverse", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("without a token: %d", w.Code)
	}
}

func TestFediverseAdmin(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})

	for _, req := range [][2]string{{"GET", ""}, {"POST", `{"address":"@anna@m.example"}`}, {"DELETE", ""}} {
		if w := h.do(req[0], "/api/v1/admin/fediverse?id=x", req[1], "Authorization", "Bearer tok"); w.Code != http.StatusForbidden {
			t.Fatalf("%s by someone else: %d", req[0], w.Code)
		}
	}

	if h.federation.address != "" || h.federation.removed != "" {
		t.Fatal("federation was asked for someone who does not run the site")
	}

	h.users.admin = true

	if w := h.do("POST", "/api/v1/admin/fediverse", `{"address":"@anna@m.example"}`, "Authorization", "Bearer tok"); w.Code != http.StatusOK || h.federation.address != "@anna@m.example" {
		t.Fatalf("feature: %d", w.Code)
	}

	if w := h.do("GET", "/api/v1/admin/fediverse", "", "Authorization", "Bearer tok"); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), anna.Handle) {
		t.Fatalf("featured: %d %s", w.Code, w.Body)
	}

	if w := h.do("DELETE", "/api/v1/admin/fediverse?id=x", "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent || h.federation.removed != "x" {
		t.Fatalf("unfeature: %d", w.Code)
	}
}

func TestFediversePictures(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	h.federation.picture = model.Picture{Body: []byte("png"), ContentType: "image/png"}

	w := h.do("GET", "/media/fediverse/posts/p2/1", "")
	if w.Code != http.StatusOK || w.Body.String() != "png" || w.Header().Get("Content-Type") != "image/png" ||
		w.Header().Get("Cache-Control") != "public, max-age=86400" || h.federation.read != "picture p2 1" {
		t.Fatalf("post picture: %d %v, read %q", w.Code, w.Header(), h.federation.read)
	}

	h.federation.picture = model.Picture{Body: []byte("jpeg"), ContentType: "image/jpeg"}
	if w = h.do("GET", "/media/fediverse/posts/p2/videos/1", ""); w.Code != http.StatusOK || w.Body.String() != "jpeg" ||
		w.Header().Get("Cache-Control") != "public, max-age=86400" || h.federation.read != "cover p2 1" {
		t.Fatalf("video cover: %d %v, read %q", w.Code, w.Header(), h.federation.read)
	}

	if w = h.do("GET", "/media/fediverse/posts/p2/link", ""); w.Code != http.StatusOK || h.federation.read != "card p2" {
		t.Fatalf("picture of a card: %d, read %q", w.Code, h.federation.read)
	}

	if w = h.do("GET", "/media/fediverse/posts/p2/videos/x", ""); w.Code != http.StatusNotFound {
		t.Fatalf("a cover with no index: %d", w.Code)
	}

	if w = h.do("GET", "/media/fediverse/accounts?id=https%3A%2F%2Fm.example%2Fusers%2Fanna", ""); w.Code != http.StatusOK || h.federation.read != "avatar "+anna.ID {
		t.Fatalf("avatar: %d, read %q", w.Code, h.federation.read)
	}

	h.federation.picture = model.Picture{Body: []byte("<svg/>"), ContentType: "image/svg+xml"}
	if w = h.do("GET", "/media/fediverse/posts/p2/0", ""); w.Code != http.StatusNotFound {
		t.Fatalf("an SVG: %d", w.Code)
	}

	if w = h.do("GET", "/media/fediverse/posts/p2/x", ""); w.Code != http.StatusNotFound {
		t.Fatalf("no index: %d", w.Code)
	}

	w = h.do("GET", "/ap/actor", "", "Accept", "application/activity+json")
	if w.Code != http.StatusOK || w.Body.String() != `{"type":"Application"}` || w.Header().Get("Content-Type") != "application/activity+json" {
		t.Fatalf("site actor: %d %s", w.Code, w.Body)
	}
}

func TestFediverseLikes(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	auth := []string{"Authorization", "Bearer tok"}

	w := h.do("PUT", "/api/v1/fediverse/posts/p2/like", "", auth...)
	if got := decode[map[string]any](t, w); w.Code != http.StatusOK || got["likes"] != float64(1) || got["liked"] != true ||
		h.federation.liked != "like p2" || h.federation.token != "tok" {
		t.Fatalf("like: %d %v, %q", w.Code, got, h.federation.liked)
	}

	w = h.do("DELETE", "/api/v1/fediverse/posts/p2/like", "", auth...)
	if got := decode[map[string]any](t, w); w.Code != http.StatusOK || got["likes"] != float64(0) || got["liked"] != false || h.federation.liked != "unlike p2" {
		t.Fatalf("unlike: %d %v", w.Code, got)
	}

	w = h.do("GET", "/api/v1/fediverse/posts/p2/likes?cursor=c1&limit=6", "", auth...)
	got := decode[struct {
		Users      []struct{ ID string } `json:"users"`
		NextCursor string                `json:"nextCursor"`
		Total      int                   `json:"total"`
	}](t, w)
	if w.Code != http.StatusOK || len(got.Users) != 2 || got.Users[0].ID != "x1" || got.NextCursor != "c2" || got.Total != 5 ||
		h.federation.liked != "likers p2" || h.federation.cursor != "c1/6" {
		t.Fatalf("likers: %d %+v, %q %q", w.Code, got, h.federation.liked, h.federation.cursor)
	}

	for _, method := range []string{"PUT", "DELETE"} {
		if w = h.do(method, "/api/v1/fediverse/posts/p2/like", ""); w.Code != http.StatusUnauthorized {
			t.Errorf("%s without a token: %d", method, w.Code)
		}
	}
	if w = h.do("GET", "/api/v1/fediverse/posts/p2/likes", ""); w.Code != http.StatusUnauthorized {
		t.Errorf("likers without a token: %d", w.Code)
	}

	h.federation.err = failure.New(failure.TooManyRequests, "too many likes")
	if w = h.do("PUT", "/api/v1/fediverse/posts/p2/like", "", auth...); w.Code != http.StatusTooManyRequests {
		t.Errorf("too many likes: %d", w.Code)
	}
}

func TestFediverseComments(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	auth := []string{"Authorization", "Bearer tok"}

	w := h.do("GET", "/api/v1/fediverse/posts/p2", "", auth...)
	if got := decode[map[string]any](t, w); w.Code != http.StatusOK || got["id"] != "p2" || got["comments"] != float64(2) || got["fediverse"] == nil {
		t.Fatalf("post: %d %v", w.Code, got)
	}

	w = h.do("GET", "/api/v1/fediverse/posts/p2/comments?cursor=c0&limit=6", "", auth...)
	type comment struct {
		ID     string `json:"id"`
		Author struct {
			Name string `json:"name"`
		} `json:"author"`
		Text string `json:"text"`
		Mine bool   `json:"mine"`
	}
	page := decode[struct {
		Comments   []comment `json:"comments"`
		NextCursor string    `json:"nextCursor"`
		Total      int       `json:"total"`
	}](t, w)
	if w.Code != http.StatusOK || len(page.Comments) != 2 || !page.Comments[0].Mine || page.Comments[1].Mine ||
		page.Comments[1].Author.Name != "x1" || page.NextCursor != "c3" || page.Total != 7 || h.federation.cursor != "p2 c0/6" {
		t.Fatalf("comments: %d %+v, %q", w.Code, page, h.federation.cursor)
	}

	// Federation takes no page of 0: the gateway asks for the usual page.
	if w = h.do("GET", "/api/v1/fediverse/posts/p2/comments", "", auth...); w.Code != http.StatusOK || h.federation.cursor != "p2 /20" {
		t.Fatalf("comments without a limit: %d, %q", w.Code, h.federation.cursor)
	}
	if w = h.do("GET", "/api/v1/fediverse/posts/p2/likes", "", auth...); w.Code != http.StatusOK || h.federation.cursor != "/20" {
		t.Fatalf("likers without a limit: %d, %q", w.Code, h.federation.cursor)
	}

	if w = h.do("POST", "/api/v1/fediverse/posts/p2/comments", `{"text":"привет"}`, auth...); w.Code != http.StatusBadRequest {
		t.Fatalf("comment without a profile: %d", w.Code)
	}

	h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}
	w = h.do("POST", "/api/v1/fediverse/posts/p2/comments", `{"text":"привет"}`, auth...)
	if got := decode[comment](t, w); w.Code != http.StatusCreated || got.ID != "c9" || !got.Mine || got.Author.Name != "Ann" ||
		h.federation.written != "p2 привет" {
		t.Fatalf("comment: %d %+v, %q", w.Code, got, h.federation.written)
	}

	if w = h.do("DELETE", "/api/v1/fediverse/comments/c1", "", auth...); w.Code != http.StatusNoContent || h.federation.deleted != "c1" {
		t.Fatalf("delete: %d, %q", w.Code, h.federation.deleted)
	}

	for _, call := range [][2]string{
		{"GET", "/api/v1/fediverse/posts/p2"},
		{"GET", "/api/v1/fediverse/posts/p2/comments"},
		{"POST", "/api/v1/fediverse/posts/p2/comments"},
		{"DELETE", "/api/v1/fediverse/comments/c1"},
	} {
		if w = h.do(call[0], call[1], `{"text":"a"}`); w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without a token: %d", call[0], call[1], w.Code)
		}
	}

	h.federation.err = failure.New(failure.PermissionDenied, "not your comment")
	if w = h.do("DELETE", "/api/v1/fediverse/comments/c2", "", auth...); w.Code != http.StatusForbidden {
		t.Errorf("delete of another: %d", w.Code)
	}
}
