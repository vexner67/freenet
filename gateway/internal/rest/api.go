// Package rest is the public HTTP/JSON API of freenet.
package rest

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/vexner67/freenet/gateway/internal/model"
)

type Auth interface {
	RequestCode(ctx context.Context, email, clientIP string) error
	VerifyCode(ctx context.Context, email, code string) (model.Tokens, error)
	RefreshToken(ctx context.Context, refreshToken string) (model.Tokens, error)
	Logout(ctx context.Context, refreshToken string) error
}

type Users interface {
	GetMe(ctx context.Context, accessToken string) (model.User, error)
	UpdateProfile(ctx context.Context, accessToken string, p model.Profile) (model.User, error)
	UpdatePrivacy(ctx context.Context, accessToken string, p model.Privacy) (model.User, error)
	GetProfile(ctx context.Context, nickname string) (model.PublicProfile, error)
	GetProfiles(ctx context.Context, ids []string) ([]model.PublicProfile, error)

	Follow(ctx context.Context, accessToken, userID string) error
	Unfollow(ctx context.Context, accessToken, userID string) error
	FollowingIDs(ctx context.Context, accessToken string) ([]string, error)
	FollowStats(ctx context.Context, accessToken, userID string) (model.FollowStats, error)
	ListFollowers(ctx context.Context, userID, cursor string, limit int) (model.UserPage, error)
	ListFollowing(ctx context.Context, userID, cursor string, limit int) (model.UserPage, error)

	SetAvatar(ctx context.Context, accessToken, avatarID string) (model.User, error)
	Search(ctx context.Context, query, cursor string, limit int) (model.UserPage, error)
}

type Media interface {
	UploadAvatar(ctx context.Context, accessToken string, image []byte) (string, error)
	DeleteAvatars(ctx context.Context, accessToken, keep string) error
	Avatar(ctx context.Context, userID, avatarID string, small bool) ([]byte, error)

	UploadPhoto(ctx context.Context, accessToken string, image []byte) (string, error)
	OwnPhotos(ctx context.Context, accessToken string, photoIDs []string) ([]string, error)
	DeletePhotos(ctx context.Context, accessToken string, photoIDs []string) error
	Photo(ctx context.Context, userID, photoID string, small bool) ([]byte, error)
}

// Posts: methods that read take an optional access token ("" for
// anonymous viewers), with which posts tell whether the viewer likes them.
type Posts interface {
	Create(ctx context.Context, accessToken, text string, photoIDs []string) (model.Post, error)
	Delete(ctx context.Context, accessToken, id string) error
	Get(ctx context.Context, accessToken, id string) (model.Post, error)
	GetMany(ctx context.Context, ids []string) ([]model.Post, error)
	List(ctx context.Context, accessToken, authorID, cursor string, limit int) (model.PostPage, error)
	Feed(ctx context.Context, accessToken string, authorIDs []string, cursor string, limit int) (model.PostPage, error)

	Like(ctx context.Context, accessToken, postID string) (int, error)
	Unlike(ctx context.Context, accessToken, postID string) (int, error)

	CreateComment(ctx context.Context, accessToken, postID, text string) (model.Comment, error)
	DeleteComment(ctx context.Context, accessToken, id string) error
	Comments(ctx context.Context, postID, cursor string, limit int) (model.CommentPage, error)
	CommentsByID(ctx context.Context, ids []string) ([]model.Comment, error)
}

type Notifications interface {
	List(ctx context.Context, accessToken string, unread bool, cursor string, limit int) (model.NotificationPage, error)
	CountUnread(ctx context.Context, accessToken string) (int, error)
	MarkRead(ctx context.Context, accessToken, id string) error
}

type Messages interface {
	Send(ctx context.Context, accessToken, recipientID, text string) (model.Message, error)
	Dialogs(ctx context.Context, accessToken, cursor string, limit int) (model.DialogPage, error)
	CountUnread(ctx context.Context, accessToken string) (int, error)
	History(ctx context.Context, accessToken, peerID, cursor, after string, limit int) (model.MessagePage, error)
	MarkRead(ctx context.Context, accessToken, peerID, upTo string) error
}

// Backends are the services the API is made of.
type Backends struct {
	Auth          Auth
	Users         Users
	Posts         Posts
	Media         Media
	Notifications Notifications
	Messages      Messages
}

type api struct {
	auth              Auth
	users             Users
	posts             Posts
	media             Media
	notifications     Notifications
	messages          Messages
	logger            *slog.Logger
	trustForwardedFor bool
	webDir            string
}

func (a *api) routes() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", a.health)

	mux.HandleFunc("POST /api/v1/auth/code", a.requestCode)
	mux.HandleFunc("POST /api/v1/auth/verify", a.verifyCode)
	mux.HandleFunc("POST /api/v1/auth/refresh", a.refreshToken)
	mux.HandleFunc("POST /api/v1/auth/logout", a.logout)

	mux.HandleFunc("GET /api/v1/me", a.getMe)
	mux.HandleFunc("PUT /api/v1/me/profile", a.updateProfile)
	mux.HandleFunc("PUT /api/v1/me/privacy", a.updatePrivacy)

	mux.HandleFunc("GET /api/v1/users/{nickname}", a.getProfile)

	mux.HandleFunc("GET /api/v1/users/{nickname}/posts", a.listPosts)
	mux.HandleFunc("POST /api/v1/me/posts", a.createPost)
	mux.HandleFunc("GET /api/v1/posts/{id}", a.getPost)
	mux.HandleFunc("DELETE /api/v1/posts/{id}", a.deletePost)
	mux.HandleFunc("PUT /api/v1/posts/{id}/like", a.likePost)
	mux.HandleFunc("DELETE /api/v1/posts/{id}/like", a.unlikePost)

	mux.HandleFunc("GET /api/v1/posts/{id}/comments", a.listComments)
	mux.HandleFunc("POST /api/v1/posts/{id}/comments", a.createComment)
	mux.HandleFunc("DELETE /api/v1/comments/{id}", a.deleteComment)

	mux.HandleFunc("POST /api/v1/users/{nickname}/follow", a.follow)
	mux.HandleFunc("DELETE /api/v1/users/{nickname}/follow", a.unfollow)
	mux.HandleFunc("GET /api/v1/users/{nickname}/followers", a.listFollowers)
	mux.HandleFunc("GET /api/v1/users/{nickname}/following", a.listFollowing)

	mux.HandleFunc("GET /api/v1/feed", a.feed)

	mux.HandleFunc("GET /api/v1/users", a.searchUsers)

	mux.HandleFunc("GET /api/v1/notifications", a.listNotifications)
	mux.HandleFunc("GET /api/v1/notifications/unread-count", a.countUnreadNotifications)
	mux.HandleFunc("POST /api/v1/notifications/read", a.markNotificationsRead)

	mux.HandleFunc("GET /api/v1/dialogs", a.listDialogs)
	mux.HandleFunc("GET /api/v1/dialogs/unread-count", a.countUnreadDialogs)
	mux.HandleFunc("GET /api/v1/dialogs/{nickname}/messages", a.listMessages)
	mux.HandleFunc("POST /api/v1/dialogs/{nickname}/messages", a.sendMessage)
	mux.HandleFunc("POST /api/v1/dialogs/{nickname}/read", a.markDialogRead)

	mux.HandleFunc("PUT /api/v1/me/avatar", a.uploadAvatar)
	mux.HandleFunc("DELETE /api/v1/me/avatar", a.deleteAvatar)
	mux.HandleFunc("GET /media/avatars/{userID}/{avatarID}/{file}", a.serveAvatar)

	mux.HandleFunc("POST /api/v1/me/photos", a.uploadPhoto)
	mux.HandleFunc("GET /media/photos/{userID}/{photoID}/{file}", a.servePhoto)

	if a.webDir != "" {
		registerPages(mux, a.webDir)
	}

	// Catch-all for JSON errors instead of the mux's plain-text ones.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		a.noRoute(mux, w, r)
	})

	return mux
}
