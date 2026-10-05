package rest_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

type fakeAuth struct {
	clientIP string
	err      error
}

func (f *fakeAuth) RequestCode(_ context.Context, _, clientIP string) error {
	f.clientIP = clientIP
	return f.err
}

func (f *fakeAuth) VerifyCode(_ context.Context, email, code string) (model.Tokens, error) {
	if code != "123456" {
		return model.Tokens{}, failure.New(failure.Unauthenticated, "invalid or expired code")
	}
	return model.Tokens{AccessToken: "access-" + email, RefreshToken: "refresh"}, nil
}

func (f *fakeAuth) RefreshToken(context.Context, string) (model.Tokens, error) {
	return model.Tokens{AccessToken: "a2", RefreshToken: "r2"}, f.err
}

func (f *fakeAuth) Logout(context.Context, string) error { return f.err }

type fakeUsers struct {
	token, statsToken, followed, unfollowed, listed string
	profileIDs                                      []string
	avatarID, query, cursor                         string
	profile                                         model.Profile
	err                                             error
}

func (f *fakeUsers) user() model.User {
	return model.User{ID: "u1", Email: "ann@example.com", Profile: f.profile, ProfileCompleted: f.profile.Nickname != ""}
}

func (f *fakeUsers) GetMe(_ context.Context, token string) (model.User, error) {
	f.token = token
	return f.user(), f.err
}

func (f *fakeUsers) UpdateProfile(_ context.Context, token string, p model.Profile) (model.User, error) {
	f.token, f.profile = token, p
	return f.user(), f.err
}

func (f *fakeUsers) UpdatePrivacy(_ context.Context, token string, p model.Privacy) (model.User, error) {
	f.token = token
	u := f.user()
	u.Privacy = p
	return u, f.err
}

func (f *fakeUsers) GetProfile(_ context.Context, nickname string) (model.PublicProfile, error) {
	switch nickname {
	case "ann":
		return model.PublicProfile{ID: "u1", Profile: model.Profile{Name: "Ann", Nickname: "ann", Worldview: "buddhism"}}, nil
	case "bob":
		return model.PublicProfile{ID: "u2", Profile: model.Profile{Name: "Bob", Nickname: "bob"}}, nil
	default:
		return model.PublicProfile{}, failure.New(failure.NotFound, "user not found")
	}
}

func (f *fakeUsers) GetProfiles(_ context.Context, ids []string) ([]model.PublicProfile, error) {
	f.profileIDs = ids
	var found []model.PublicProfile
	for _, id := range ids {
		if id == "u2" {
			found = append(found, model.PublicProfile{ID: "u2", Profile: model.Profile{Name: "Bob", Nickname: "bob", Gender: "male"}})
		}
		if id == "u1" {
			found = append(found, model.PublicProfile{ID: "u1", Profile: model.Profile{Name: "Ann", Nickname: "ann"}})
		}
	}
	return found, nil
}

func (f *fakeUsers) Follow(_ context.Context, token, userID string) error {
	f.token, f.followed = token, userID
	return f.err
}

func (f *fakeUsers) Unfollow(_ context.Context, token, userID string) error {
	f.token, f.unfollowed = token, userID
	return f.err
}

func (f *fakeUsers) SetAvatar(_ context.Context, token, avatarID string) (model.User, error) {
	f.token, f.avatarID = token, avatarID
	u := f.user()
	u.AvatarID = avatarID
	return u, f.err
}

func (f *fakeUsers) Search(_ context.Context, query, cursor string, _ int) (model.UserPage, error) {
	f.query, f.cursor = query, cursor
	if query == "" {
		return model.UserPage{}, failure.New(failure.InvalidArgument, "search query is empty")
	}
	return model.UserPage{
		Users:      []model.PublicProfile{{ID: "u1", AvatarID: "a1", Profile: model.Profile{Name: "Ann", Nickname: "ann"}}},
		NextCursor: "u1",
	}, nil
}

func (f *fakeUsers) FollowingIDs(_ context.Context, token string) ([]string, error) {
	f.token = token
	return []string{"u2", "gone"}, nil
}

func (f *fakeUsers) FollowStats(_ context.Context, token, _ string) (model.FollowStats, error) {
	f.statsToken = token
	return model.FollowStats{Followers: 3, Following: 1, IsFollowing: token != ""}, nil
}

func (f *fakeUsers) ListFollowers(_ context.Context, userID, cursor string, _ int) (model.UserPage, error) {
	f.listed = "followers:" + userID + ":" + cursor
	return model.UserPage{Users: []model.PublicProfile{{ID: "u2", Profile: model.Profile{Nickname: "bob"}}}, Total: 1}, nil
}

func (f *fakeUsers) ListFollowing(_ context.Context, userID, cursor string, _ int) (model.UserPage, error) {
	f.listed = "following:" + userID + ":" + cursor
	return model.UserPage{NextCursor: "c2", Total: 5}, nil
}

type fakePosts struct {
	authors                []string
	token, cursor, deleted string
	limit                  int
	created                string
	liked                  map[string]bool
	err                    error
}

func (f *fakePosts) Create(_ context.Context, token, text string) (model.Post, error) {
	f.token, f.created = token, text
	return model.Post{ID: "p9", Text: text, CreatedAt: time.Date(2026, 10, 4, 15, 30, 0, 0, time.UTC)}, f.err
}

func (f *fakePosts) Delete(_ context.Context, token, id string) error {
	f.token, f.deleted = token, id
	return f.err
}

// Get knows p1 by u1 and p2 by an author who deleted the account.
func (f *fakePosts) Get(_ context.Context, token, id string) (model.Post, error) {
	f.token = token
	switch id {
	case "p1":
		return model.Post{ID: "p1", Author: model.Author{ID: "u1"}, Text: "первая", Likes: 3, Comments: 2, Liked: token != ""}, nil
	case "p2":
		return model.Post{ID: "p2", Author: model.Author{ID: "gone"}, Text: "сирота"}, nil
	default:
		return model.Post{}, failure.New(failure.NotFound, "post not found")
	}
}

// GetMany knows p1 only.
func (f *fakePosts) GetMany(_ context.Context, ids []string) ([]model.Post, error) {
	var found []model.Post
	for _, id := range ids {
		if id == "p1" {
			found = append(found, model.Post{ID: "p1", Author: model.Author{ID: "u1"}, CreatedAt: time.Date(2026, 10, 4, 15, 30, 0, 0, time.UTC)})
		}
	}
	return found, nil
}

// CommentsByID knows c1 only.
func (f *fakePosts) CommentsByID(_ context.Context, ids []string) ([]model.Comment, error) {
	var found []model.Comment
	for _, id := range ids {
		if id == "c1" {
			found = append(found, model.Comment{ID: "c1", PostID: "p1", Author: model.Author{ID: "u2"}, Text: "согласен"})
		}
	}
	return found, nil
}

func (f *fakePosts) Feed(_ context.Context, token string, authorIDs []string, cursor string, _ int) (model.PostPage, error) {
	f.token, f.authors, f.cursor = token, authorIDs, cursor
	return model.PostPage{
		Posts: []model.Post{
			{ID: "p3", Author: model.Author{ID: "u2"}, Text: "от боба", Likes: 1, Liked: true},
			{ID: "p2", Author: model.Author{ID: "gone"}, Text: "автор удалился"},
			{ID: "p1", Author: model.Author{ID: "u1"}, Text: "моя"},
		},
		NextCursor: "p1",
	}, nil
}

func (f *fakePosts) List(_ context.Context, token, authorID, cursor string, limit int) (model.PostPage, error) {
	f.token, f.cursor, f.limit = token, cursor, limit
	if authorID != "u1" {
		return model.PostPage{}, errors.New("wrong author id")
	}
	return model.PostPage{
		Posts:      []model.Post{{ID: "p2", Author: model.Author{ID: "u1"}, Text: "вторая", Comments: 4}, {ID: "p1", Author: model.Author{ID: "u1"}, Text: "первая", Likes: 2, Liked: true}},
		NextCursor: "p1",
		Total:      7,
	}, f.err
}

func (f *fakePosts) Like(_ context.Context, token, postID string) (int, error) {
	return f.setLike(token, postID, true)
}

func (f *fakePosts) Unlike(_ context.Context, token, postID string) (int, error) {
	return f.setLike(token, postID, false)
}

func (f *fakePosts) setLike(token, postID string, liked bool) (int, error) {
	if f.err != nil {
		return 0, f.err
	}
	if f.liked == nil {
		f.liked = make(map[string]bool)
	}
	f.token, f.liked[postID] = token, liked

	likes := 0
	for _, l := range f.liked {
		if l {
			likes++
		}
	}
	return likes, nil
}

func (f *fakePosts) CreateComment(_ context.Context, token, postID, text string) (model.Comment, error) {
	f.token, f.created = token, text
	return model.Comment{ID: "c9", PostID: postID, Text: text, CreatedAt: time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)}, f.err
}

func (f *fakePosts) DeleteComment(_ context.Context, token, id string) error {
	f.token, f.deleted = token, id
	return f.err
}

func (f *fakePosts) Comments(_ context.Context, postID, cursor string, limit int) (model.CommentPage, error) {
	f.cursor, f.limit = cursor, limit
	if postID != "p1" {
		return model.CommentPage{}, failure.New(failure.NotFound, "post not found")
	}
	return model.CommentPage{
		Comments: []model.Comment{
			{ID: "c1", PostID: "p1", Author: model.Author{ID: "u2"}, Text: "первый"},
			{ID: "c2", PostID: "p1", Author: model.Author{ID: "gone"}, Text: "автор удалился"},
			{ID: "c3", PostID: "p1", Author: model.Author{ID: "u1"}, Text: "ответ"},
		},
		NextCursor: "c3",
		Total:      5,
	}, nil
}

type fakeMedia struct {
	uploaded []byte
	kept     string
	deleted  bool
	err      error
}

func (f *fakeMedia) UploadAvatar(_ context.Context, _ string, image []byte) (string, error) {
	f.uploaded = image
	return "a2", f.err
}

func (f *fakeMedia) DeleteAvatars(_ context.Context, _, keep string) error {
	f.kept, f.deleted = keep, true
	return nil
}

func (f *fakeMedia) Avatar(_ context.Context, userID, avatarID string, small bool) ([]byte, error) {
	if userID != "u1" || avatarID != "a1" {
		return nil, failure.New(failure.NotFound, "avatar not found")
	}
	if small {
		return []byte("small-jpeg"), nil
	}
	return []byte("large-jpeg"), nil
}

// fakeNotifications has a like and a comment by bob (u2), a follow by an
// account that is gone and a like of a deleted post.
type fakeNotifications struct {
	token, cursor, markedUpTo string
	unread                    bool
	limit                     int
	err                       error
}

func (f *fakeNotifications) List(_ context.Context, token string, unread bool, cursor string, limit int) (model.NotificationPage, error) {
	f.token, f.unread, f.cursor, f.limit = token, unread, cursor, limit
	at := time.Date(2026, 10, 5, 18, 0, 0, 0, time.UTC)
	return model.NotificationPage{
		Notifications: []model.Notification{
			{ID: "n4", Kind: "comment", ActorID: "u2", PostID: "p1", CommentID: "c1", CreatedAt: at},
			{ID: "n3", Kind: "follow", ActorID: "gone", CreatedAt: at},
			{ID: "n2", Kind: "like", ActorID: "u2", PostID: "p-deleted", CreatedAt: at},
			{ID: "n1", Kind: "like", ActorID: "u2", PostID: "p1", CreatedAt: at},
			{ID: "n0", Kind: "follow", ActorID: "u2", CreatedAt: at},
		},
		NextCursor: "n0",
	}, f.err
}

func (f *fakeNotifications) CountUnread(_ context.Context, token string) (int, error) {
	f.token = token
	return 3, f.err
}

func (f *fakeNotifications) MarkRead(_ context.Context, token, id string) error {
	f.token, f.markedUpTo = token, id
	return f.err
}

type harness struct {
	handler       http.Handler
	auth          *fakeAuth
	users         *fakeUsers
	posts         *fakePosts
	media         *fakeMedia
	notifications *fakeNotifications
}

func newHarness(cfg rest.Config) harness {
	if cfg.RequestTimeout == 0 {
		cfg.RequestTimeout = time.Second
	}

	h := harness{auth: &fakeAuth{}, users: &fakeUsers{}, posts: &fakePosts{}, media: &fakeMedia{}, notifications: &fakeNotifications{}}
	h.handler = rest.NewServer(cfg, rest.Backends{
		Auth:          h.auth,
		Users:         h.users,
		Posts:         h.posts,
		Media:         h.media,
		Notifications: h.notifications,
	}, slog.New(slog.NewTextHandler(io.Discard, nil))).Handler()

	return h
}

func (h harness) do(method, path, body string, headers ...string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.RemoteAddr = "203.0.113.9:51000"
	for i := 0; i+1 < len(headers); i += 2 {
		r.Header.Set(headers[i], headers[i+1])
	}

	w := httptest.NewRecorder()
	h.handler.ServeHTTP(w, r)

	return w
}

func decode[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()

	var v T
	if err := json.NewDecoder(w.Body).Decode(&v); err != nil {
		t.Fatalf("decode %q: %v", w.Body.String(), err)
	}

	return v
}

type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func TestAuthEndpoints(t *testing.T) {
	t.Parallel()

	t.Run("request code", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		w := h.do("POST", "/api/v1/auth/code", `{"email":"ann@example.com"}`)

		if w.Code != http.StatusNoContent {
			t.Fatalf("status = %d", w.Code)
		}
		if h.auth.clientIP != "203.0.113.9" {
			t.Fatalf("client ip = %q", h.auth.clientIP)
		}
	})

	t.Run("verify code returns tokens", func(t *testing.T) {
		t.Parallel()

		w := newHarness(rest.Config{}).do("POST", "/api/v1/auth/verify", `{"email":"ann@example.com","code":"123456"}`)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d", w.Code)
		}

		tokens := decode[map[string]string](t, w)
		if tokens["accessToken"] != "access-ann@example.com" || tokens["refreshToken"] != "refresh" {
			t.Fatalf("tokens = %v", tokens)
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("token responses must not be cached")
		}
	})

	t.Run("wrong code is 401 with the service message", func(t *testing.T) {
		t.Parallel()

		w := newHarness(rest.Config{}).do("POST", "/api/v1/auth/verify", `{"email":"ann@example.com","code":"000000"}`)
		body := decode[errorBody](t, w)

		if w.Code != http.StatusUnauthorized || body.Error.Code != "unauthenticated" || body.Error.Message != "invalid or expired code" {
			t.Fatalf("%d %+v", w.Code, body)
		}
	})

	t.Run("refresh and logout", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		if w := h.do("POST", "/api/v1/auth/refresh", `{"refreshToken":"r1"}`); w.Code != http.StatusOK {
			t.Fatalf("refresh status = %d", w.Code)
		}
		if w := h.do("POST", "/api/v1/auth/logout", `{"refreshToken":"r1"}`); w.Code != http.StatusNoContent {
			t.Fatalf("logout status = %d", w.Code)
		}
	})
}

func TestErrorMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		err    error
		status int
		code   string
	}{
		{failure.New(failure.InvalidArgument, "invalid email"), 400, "invalid_argument"},
		{failure.New(failure.TooManyRequests, "too many requests, try again later"), 429, "too_many_requests"},
		{failure.New(failure.Unavailable, "service temporarily unavailable"), 503, "unavailable"},
		{failure.New(failure.Conflict, "nickname is already taken"), 409, "conflict"},
		{errors.New("dial tcp 10.0.0.5:50051: connection refused"), 500, "internal"},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			t.Parallel()

			h := newHarness(rest.Config{})
			h.auth.err = tt.err

			w := h.do("POST", "/api/v1/auth/code", `{"email":"ann@example.com"}`)
			body := decode[errorBody](t, w)

			if w.Code != tt.status || body.Error.Code != tt.code {
				t.Fatalf("got %d %+v, want %d %s", w.Code, body, tt.status, tt.code)
			}
			if tt.status == 500 && body.Error.Message != "internal error" {
				t.Fatalf("internal details leaked: %q", body.Error.Message)
			}
		})
	}
}

func TestUserEndpoints(t *testing.T) {
	t.Parallel()

	t.Run("me requires a token", func(t *testing.T) {
		t.Parallel()

		w := newHarness(rest.Config{}).do("GET", "/api/v1/me", "")
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d", w.Code)
		}
	})

	t.Run("token is forwarded", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		w := h.do("GET", "/api/v1/me", "", "Authorization", "Bearer tok")

		if w.Code != http.StatusOK || h.users.token != "tok" {
			t.Fatalf("status = %d, token = %q", w.Code, h.users.token)
		}
		if u := decode[map[string]any](t, w); u["birthday"] != nil || u["profileCompleted"] != false {
			t.Fatalf("fresh user: %v", u)
		}
	})

	t.Run("update profile", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		w := h.do("PUT", "/api/v1/me/profile",
			`{"name":"Ann","nickname":"ann","gender":"female","birthday":"1990-05-17"}`,
			"Authorization", "Bearer tok")

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d: %s", w.Code, w.Body)
		}
		if got := h.users.profile.Birthday; !got.Equal(time.Date(1990, 5, 17, 0, 0, 0, 0, time.UTC)) {
			t.Fatalf("birthday = %v", got)
		}

		u := decode[map[string]any](t, w)
		if u["birthday"] != "1990-05-17" || u["gender"] != "female" || u["profileCompleted"] != true {
			t.Fatalf("response: %v", u)
		}
	})

	t.Run("bad birthday", func(t *testing.T) {
		t.Parallel()

		w := newHarness(rest.Config{}).do("PUT", "/api/v1/me/profile",
			`{"name":"Ann","nickname":"ann","birthday":"17.05.1990"}`, "Authorization", "Bearer tok")
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d", w.Code)
		}
	})

	t.Run("update privacy", func(t *testing.T) {
		t.Parallel()

		w := newHarness(rest.Config{}).do("PUT", "/api/v1/me/privacy", `{"showWorldview":true}`, "Authorization", "Bearer tok")
		u := decode[map[string]any](t, w)

		if privacy, _ := u["privacy"].(map[string]any); privacy["showWorldview"] != true || privacy["showBirthday"] != false {
			t.Fatalf("privacy: %v", u["privacy"])
		}
	})

	t.Run("public profile", func(t *testing.T) {
		t.Parallel()

		h := newHarness(rest.Config{})
		w := h.do("GET", "/api/v1/users/ann", "")
		p := decode[map[string]any](t, w)

		if w.Code != http.StatusOK || p["worldview"] != "buddhism" {
			t.Fatalf("%d %v", w.Code, p)
		}
		if _, ok := p["email"]; ok {
			t.Fatal("public profile must not have an email field")
		}

		if w = h.do("GET", "/api/v1/users/nobody", ""); w.Code != http.StatusNotFound {
			t.Fatalf("unknown nickname: %d", w.Code)
		}
	})
}

func TestRequestValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		method string
		path   string
		body   string
		status int
	}{
		{"malformed json", "POST", "/api/v1/auth/code", `{"email":`, 400},
		{"unknown field", "POST", "/api/v1/auth/code", `{"mail":"a@b.c"}`, 400},
		{"wrong type", "POST", "/api/v1/auth/code", `{"email":5}`, 400},
		{"trailing data", "POST", "/api/v1/auth/code", `{"email":"a@b.c"}{}`, 400},
		{"too large", "POST", "/api/v1/auth/code", `{"email":"` + strings.Repeat("a", 70<<10) + `"}`, 400},
		{"unknown endpoint", "GET", "/api/v1/nope", "", 404},
		{"wrong method", "GET", "/api/v1/auth/code", "", 405},
		{"wrong method on a path with a parameter", "DELETE", "/api/v1/users/ann", "", 405},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if w := newHarness(rest.Config{}).do(tt.method, tt.path, tt.body); w.Code != tt.status {
				t.Fatalf("status = %d, want %d: %s", w.Code, tt.status, w.Body)
			}
		})
	}
}

func TestCORS(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{AllowedOrigins: []string{"http://localhost:5173"}})

	t.Run("preflight from an allowed origin", func(t *testing.T) {
		t.Parallel()

		w := h.do("OPTIONS", "/api/v1/me", "",
			"Origin", "http://localhost:5173", "Access-Control-Request-Method", "PUT")

		if w.Code != http.StatusNoContent ||
			w.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" ||
			!strings.Contains(w.Header().Get("Access-Control-Allow-Headers"), "Authorization") {
			t.Fatalf("%d %v", w.Code, w.Header())
		}
	})

	t.Run("other origins get no CORS headers", func(t *testing.T) {
		t.Parallel()

		w := h.do("GET", "/api/v1/users/ann", "", "Origin", "https://evil.example")
		if w.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Fatal("foreign origin must not be allowed")
		}
	})
}

func TestClientIP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		trust bool
		xff   string
		want  string
	}{
		{"connection address by default", false, "198.51.100.1", "203.0.113.9"},
		{"rightmost forwarded address when trusted", true, "1.2.3.4, 198.51.100.1", "198.51.100.1"},
		{"garbage falls back to connection", true, "nonsense", "203.0.113.9"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(rest.Config{TrustForwardedFor: tt.trust})
			h.do("POST", "/api/v1/auth/code", `{"email":"ann@example.com"}`, "X-Forwarded-For", tt.xff)

			if h.auth.clientIP != tt.want {
				t.Fatalf("client ip = %q, want %q", h.auth.clientIP, tt.want)
			}
		})
	}
}

func TestHealthz(t *testing.T) {
	t.Parallel()

	if w := newHarness(rest.Config{}).do("GET", "/healthz", ""); w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
}
