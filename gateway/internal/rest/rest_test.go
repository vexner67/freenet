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
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
	"github.com/vexner67/freenet/pkg/reqid"
)

type fakeAuth struct {
	clientIP  string
	requestID string
	err       error
}

func (f *fakeAuth) RequestCode(ctx context.Context, _, clientIP string) error {
	f.clientIP = clientIP
	f.requestID = reqid.FromContext(ctx)
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
	profileCalls                                    int
	avatarID, query, cursor                         string
	profile                                         model.Profile
	wallAsked                                       string
	closedWall                                      bool
	// Messages: closed unless the peer wrote first; wroteFirst is what the
	// gateway passed on.
	closedMessages, wroteFirst bool
	// Blacklists: relation is the same with everyone.
	relation                         model.Relation
	blocked, unblocked, relationWith string
	// deactivated: the caller's page is deleted, with this reason.
	deactivated bool
	reason      string
	err         error
}

func (f *fakeUsers) user() model.User {
	u := model.User{ID: "u1", Email: "ann@example.com", Profile: f.profile, ProfileCompleted: f.profile.Nickname != ""}
	if f.deactivated {
		u.Deactivated, u.PurgeAt = true, time.Date(2027, 5, 7, 12, 0, 0, 0, time.UTC)
	}
	return u
}

func (f *fakeUsers) Deactivate(_ context.Context, token, reason string) (model.User, error) {
	f.token, f.reason, f.deactivated = token, reason, f.err == nil
	return f.user(), f.err
}

func (f *fakeUsers) Reactivate(_ context.Context, token string) (model.User, error) {
	f.token, f.deactivated = token, false
	return f.user(), f.err
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
	case "cat":
		return model.PublicProfile{ID: "u3", Profile: model.Profile{Name: "Cat", Nickname: "cat"}, Deactivated: true}, nil
	default:
		return model.PublicProfile{}, failure.New(failure.NotFound, "user not found")
	}
}

// GetProfiles knows u1, u2, u3 (a deleted page) and anyone whose ID starts
// with "x", and takes at most 100 IDs, as the user service.
func (f *fakeUsers) GetProfiles(_ context.Context, ids []string) ([]model.PublicProfile, error) {
	f.profileIDs = ids
	f.profileCalls++
	if len(ids) > 100 {
		return nil, failure.New(failure.InvalidArgument, "too many user ids")
	}
	var found []model.PublicProfile
	for _, id := range ids {
		if strings.HasPrefix(id, "x") {
			found = append(found, model.PublicProfile{ID: id, Profile: model.Profile{Name: id, Nickname: id}})
		}
		if id == "u2" {
			found = append(found, model.PublicProfile{ID: "u2", Profile: model.Profile{Name: "Bob", Nickname: "bob", Gender: "male"}})
		}
		if id == "u1" {
			found = append(found, model.PublicProfile{ID: "u1", Profile: model.Profile{Name: "Ann", Nickname: "ann"}})
		}
		if id == "u3" {
			found = append(found, model.PublicProfile{ID: "u3", Profile: model.Profile{Name: "Cat", Nickname: "cat"}, Deactivated: true})
		}
	}
	return found, nil
}

// CanPostOnWall: every wall is open but when closedWall.
func (f *fakeUsers) CanPostOnWall(_ context.Context, token, userID string) (bool, error) {
	f.token, f.wallAsked = token, userID
	return !f.closedWall, f.err
}

func (f *fakeUsers) CanMessage(_ context.Context, token, _ string, wroteFirst bool) (bool, error) {
	f.token, f.wroteFirst = token, wroteFirst
	return !f.relation.Blocked() && (!f.closedMessages || wroteFirst), f.err
}

func (f *fakeUsers) Block(_ context.Context, token, userID string) error {
	f.token, f.blocked = token, userID
	return f.err
}

func (f *fakeUsers) Unblock(_ context.Context, token, userID string) error {
	f.token, f.unblocked = token, userID
	return f.err
}

func (f *fakeUsers) Blacklist(_ context.Context, token, cursor string, _ int) (model.UserPage, error) {
	f.token, f.cursor = token, cursor
	return model.UserPage{Users: []model.PublicProfile{{ID: "u2", Profile: model.Profile{Name: "Bob", Nickname: "bob"}}}, Total: 1}, f.err
}

func (f *fakeUsers) Relation(_ context.Context, token, userID string) (model.Relation, error) {
	f.token, f.relationWith = token, userID
	return f.relation, f.err
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
	wallPosts              []model.Post
	created, wall, shared  string
	edited, pinned         string
	photos                 []string
	liked                  map[string]bool
	err                    error
}

func (f *fakePosts) Create(_ context.Context, token, wallID, text string, photoIDs []string) (model.Post, error) {
	f.token, f.wall, f.created, f.photos = token, wallID, text, photoIDs
	return model.Post{ID: "p9", Text: text, PhotoIDs: photoIDs, CreatedAt: time.Date(2026, 10, 4, 15, 30, 0, 0, time.UTC)}, f.err
}

// Edit edits the posts Get knows.
func (f *fakePosts) Edit(ctx context.Context, token, id, text string, photoIDs []string) (model.Post, error) {
	p, err := f.Get(ctx, token, id)
	if err != nil {
		return model.Post{}, err
	}

	f.edited, f.photos = text, photoIDs
	p.Text, p.PhotoIDs, p.EditedAt = text, photoIDs, time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC)

	return p, f.err
}

// Pin pins the posts Get knows; Unpin is "-" before the ID.
func (f *fakePosts) Pin(ctx context.Context, token, id string) error {
	if _, err := f.Get(ctx, token, id); err != nil {
		return err
	}

	f.pinned = id
	return f.err
}

func (f *fakePosts) Unpin(ctx context.Context, token, id string) error {
	if _, err := f.Get(ctx, token, id); err != nil {
		return err
	}

	f.pinned = "-" + id
	return f.err
}

// Repost shares p1 only, as u1's repost p7.
func (f *fakePosts) Repost(_ context.Context, token, postID, text string) (model.Post, error) {
	f.token, f.shared, f.created = token, postID, text
	if postID != "p1" {
		return model.Post{}, failure.New(failure.NotFound, "post not found")
	}
	ann := model.Author{ID: "u1"}
	return model.Post{ID: "p7", Author: ann, Wall: ann, Text: text, RepostOf: "p1", CreatedAt: time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)}, f.err
}

// Delete knows the posts Get knows.
func (f *fakePosts) Delete(ctx context.Context, token, id string) error {
	if _, err := f.Get(ctx, token, id); err != nil {
		return err
	}

	f.token, f.deleted = token, id
	return f.err
}

// Get knows p1 by u1, p2 by an author who deleted the account, p4 by u2
// on u1's wall and p8, u2's repost of a deleted post.
func (f *fakePosts) Get(_ context.Context, token, id string) (model.Post, error) {
	f.token = token
	switch id {
	case "p1":
		return model.Post{ID: "p1", Author: model.Author{ID: "u1"}, Wall: model.Author{ID: "u1"}, Text: "первая", PhotoIDs: []string{"ph1"}, Likes: 3, Comments: 2, Liked: token != ""}, nil
	case "p2":
		return model.Post{ID: "p2", Author: model.Author{ID: "gone"}, Wall: model.Author{ID: "gone"}, Text: "сирота"}, nil
	case "p4":
		return model.Post{ID: "p4", Author: model.Author{ID: "u2"}, Wall: model.Author{ID: "u1"}, Text: "в гостях"}, nil
	case "p8":
		return model.Post{ID: "p8", Author: model.Author{ID: "u2"}, Wall: model.Author{ID: "u2"}, RepostOf: "p-deleted"}, nil
	default:
		return model.Post{}, failure.New(failure.NotFound, "post not found")
	}
}

// GetMany knows p1 only.
func (f *fakePosts) GetMany(_ context.Context, ids []string) ([]model.Post, error) {
	var found []model.Post
	for _, id := range ids {
		if id == "p1" {
			found = append(found, model.Post{ID: "p1", Author: model.Author{ID: "u1"}, Wall: model.Author{ID: "u1"}, Text: "первая", PhotoIDs: []string{"ph1"}, Reposts: 1, CreatedAt: time.Date(2026, 10, 4, 15, 30, 0, 0, time.UTC)})
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
			{ID: "p3", Author: model.Author{ID: "u2"}, Wall: model.Author{ID: "u2"}, Text: "от боба", Likes: 1, Liked: true},
			{ID: "p2", Author: model.Author{ID: "gone"}, Wall: model.Author{ID: "gone"}, Text: "автор удалился"},
			{ID: "p5", Author: model.Author{ID: "u2"}, Wall: model.Author{ID: "gone"}, Text: "на исчезнувшей стене"},
			{ID: "p1", Author: model.Author{ID: "u1"}, Wall: model.Author{ID: "u1"}, Text: "моя"},
		},
		NextCursor: "p1",
	}, nil
}

// List is u1's wall: bob's post p4 on it and u1's own p2 and p1.
func (f *fakePosts) List(_ context.Context, token, wallID, cursor string, limit int) (model.PostPage, error) {
	f.token, f.cursor, f.limit = token, cursor, limit
	if wallID != "u1" {
		return model.PostPage{}, errors.New("wrong wall id")
	}
	if f.wallPosts != nil {
		return model.PostPage{Posts: f.wallPosts}, f.err
	}
	ann := model.Author{ID: "u1"}
	return model.PostPage{
		Posts: []model.Post{
			{ID: "p4", Author: model.Author{ID: "u2"}, Wall: ann, Text: "в гостях"},
			{ID: "p2", Author: ann, Wall: ann, Text: "вторая", Comments: 4},
			{ID: "p1", Author: ann, Wall: ann, Text: "первая", Likes: 2, Liked: true},
		},
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

func (f *fakePosts) CreateComment(_ context.Context, token, postID, text string, photoIDs []string) (model.Comment, error) {
	f.token, f.created, f.photos = token, text, photoIDs
	return model.Comment{ID: "c9", PostID: postID, Text: text, PhotoIDs: photoIDs, CreatedAt: time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)}, f.err
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

// fakeMedia: u1 has the photos ph1 and ph2.
type fakeMedia struct {
	uploaded []byte
	kept     string
	deleted  bool
	err      error
}

func (f *fakeMedia) UploadPhoto(_ context.Context, _ string, image []byte) (string, error) {
	f.uploaded = image
	return "ph9", f.err
}

func (f *fakeMedia) OwnPhotos(_ context.Context, _ string, photoIDs []string) ([]string, error) {
	var own []string
	for _, id := range photoIDs {
		if id == "ph1" || id == "ph2" {
			own = append(own, id)
		}
	}
	return own, f.err
}

func (f *fakeMedia) Photo(_ context.Context, userID, photoID string, small bool) ([]byte, error) {
	if userID != "u1" || photoID != "ph1" {
		return nil, failure.New(failure.NotFound, "photo not found")
	}
	if small {
		return []byte("small-photo"), nil
	}
	return []byte("large-photo"), nil
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
	// changes feed Subscribe; closing it ends the stream with streamEnd.
	changes   chan string
	streamEnd error
	mu        sync.Mutex // the event stream counts from its goroutines
	err       error
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
			{ID: "n-1", Kind: "wall_post", ActorID: "u2", PostID: "p1", CreatedAt: at},
			{ID: "n-2", Kind: "repost", ActorID: "u2", PostID: "p1", CreatedAt: at},
		},
		NextCursor: "n-2",
	}, f.err
}

func (f *fakeNotifications) CountUnread(_ context.Context, token string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.token = token
	return 3, f.err
}

func (f *fakeNotifications) Subscribe(ctx context.Context, token string) (model.NotificationChanges, error) {
	if token != "tok" {
		return nil, failure.New(failure.Unauthenticated, "invalid access token")
	}
	return func() (string, error) {
		select {
		case c, ok := <-f.changes:
			if !ok {
				return "", f.streamEnd
			}
			return c, nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}, nil
}

func (f *fakeNotifications) MarkRead(_ context.Context, token, id string) error {
	f.token, f.markedUpTo = token, id
	return f.err
}

// fakeMessages: ann is u1, bob u2. Ann has dialogs with bob and with an
// account that is gone.
type fakeMessages struct {
	token, peerID, cursor, after, sentText, markedUpTo string
	// peerWrote: the peer has written to the caller.
	peerWrote bool
	// dialogs replace the default page when set.
	dialogs []model.Dialog
	// events feed Subscribe; closing it ends the stream with streamEnd.
	events    chan model.MessageEvent
	streamEnd error
	mu        sync.Mutex // the event stream counts from its goroutines
	err       error
}

func (f *fakeMessages) Subscribe(ctx context.Context, _ string) (model.MessageEvents, error) {
	return func() (model.MessageEvent, error) {
		select {
		case e, ok := <-f.events:
			if !ok {
				return model.MessageEvent{}, f.streamEnd
			}
			return e, nil
		case <-ctx.Done():
			return model.MessageEvent{}, ctx.Err()
		}
	}, nil
}

func (f *fakeMessages) HasMessagesFrom(_ context.Context, token, peerID string) (bool, error) {
	f.token, f.peerID = token, peerID
	return f.peerWrote, f.err
}

func (f *fakeMessages) Send(_ context.Context, token, recipientID, text string, photoIDs []string) (model.Message, error) {
	f.token, f.peerID, f.sentText = token, recipientID, text
	return model.Message{ID: "m9", SenderID: "u1", RecipientID: recipientID, Text: text, PhotoIDs: photoIDs, CreatedAt: time.Date(2026, 10, 5, 22, 0, 0, 0, time.UTC)}, f.err
}

func (f *fakeMessages) Dialogs(_ context.Context, token, cursor string, _ int) (model.DialogPage, error) {
	f.token, f.cursor = token, cursor
	if f.dialogs != nil {
		return model.DialogPage{Dialogs: f.dialogs}, f.err
	}
	return model.DialogPage{
		Dialogs: []model.Dialog{
			{PeerID: "u2", LastMessage: model.Message{ID: "m2", SenderID: "u2", RecipientID: "u1", Text: "как дела?"}, Unread: true},
			{PeerID: "gone", LastMessage: model.Message{ID: "m1", SenderID: "u1", RecipientID: "gone", Text: "ау", Read: true}},
		},
		NextCursor: "m1",
	}, f.err
}

func (f *fakeMessages) CountUnread(_ context.Context, token string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.token = token
	return 2, f.err
}

func (f *fakeMessages) History(_ context.Context, token, peerID, cursor, after string, _ int) (model.MessagePage, error) {
	f.token, f.peerID, f.cursor, f.after = token, peerID, cursor, after
	return model.MessagePage{
		Messages: []model.Message{
			{ID: "m1", SenderID: "u1", RecipientID: peerID, Text: "привет", Read: true},
			{ID: "m2", SenderID: peerID, RecipientID: "u1", Text: "как дела?"},
		},
		NextCursor:   "m1",
		PeerReadUpTo: "m1",
	}, f.err
}

func (f *fakeMessages) MarkRead(_ context.Context, token, peerID, upTo string) error {
	f.token, f.peerID, f.markedUpTo = token, peerID, upTo
	return f.err
}

type harness struct {
	handler       http.Handler
	metrics       *prometheus.Registry
	auth          *fakeAuth
	users         *fakeUsers
	posts         *fakePosts
	media         *fakeMedia
	notifications *fakeNotifications
	messages      *fakeMessages
}

func newHarness(cfg rest.Config) harness {
	if cfg.RequestTimeout == 0 {
		cfg.RequestTimeout = time.Second
	}

	h := harness{
		auth:          &fakeAuth{},
		users:         &fakeUsers{},
		posts:         &fakePosts{},
		media:         &fakeMedia{},
		notifications: &fakeNotifications{},
		messages:      &fakeMessages{},
	}
	h.metrics = prometheus.NewRegistry()
	h.handler = mustServer(cfg, rest.Backends{
		Auth:          h.auth,
		Users:         h.users,
		Posts:         h.posts,
		Media:         h.media,
		Notifications: h.notifications,
		Messages:      h.messages,
	}, h.metrics).Handler()

	return h
}

// mustServer is rest.NewServer for tests, with metrics in reg.
func mustServer(cfg rest.Config, backends rest.Backends, reg prometheus.Registerer) *rest.Server {
	srv, err := rest.NewServer(cfg, backends, slog.New(slog.NewTextHandler(io.Discard, nil)), reg)
	if err != nil {
		panic(err)
	}

	return srv
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
