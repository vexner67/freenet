// Package rest is the public HTTP/JSON API of freenet.
package rest

import (
	"context"
	"log/slog"
	"net/http"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
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
	Deactivate(ctx context.Context, accessToken, reason string) (model.User, error)
	Reactivate(ctx context.Context, accessToken string) (model.User, error)
	UpdateProfile(ctx context.Context, accessToken string, p model.Profile) (model.User, error)
	UpdatePrivacy(ctx context.Context, accessToken string, p model.Privacy) (model.User, error)
	GetProfile(ctx context.Context, nickname string) (model.PublicProfile, error)
	GetProfiles(ctx context.Context, ids []string) ([]model.PublicProfile, error)
	CanPostOnWall(ctx context.Context, accessToken, userID string) (bool, error)
	CanMessage(ctx context.Context, accessToken, userID string, wroteFirst bool) (bool, error)
	CanSeeClubs(ctx context.Context, accessToken, userID string) (bool, error)

	Block(ctx context.Context, accessToken, userID string) error
	Unblock(ctx context.Context, accessToken, userID string) error
	Blacklist(ctx context.Context, accessToken, cursor string, limit int) (model.UserPage, error)
	Relation(ctx context.Context, accessToken, userID string) (model.Relation, error)

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
	// The photos of a club, served as avatars under its ID; the caller
	// checks that the user runs the club.
	UploadClubAvatar(ctx context.Context, accessToken, clubID string, image []byte) (string, error)
	DeleteClubAvatars(ctx context.Context, accessToken, clubID, keep string) error

	UploadPhoto(ctx context.Context, accessToken string, image []byte) (string, error)
	OwnPhotos(ctx context.Context, accessToken string, photoIDs []string) ([]string, error)
	Photo(ctx context.Context, userID, photoID string, small bool) ([]byte, error)
}

// Posts: methods that read take an optional access token ("" for
// anonymous viewers), with which posts tell whether the viewer likes them.
type Posts interface {
	Create(ctx context.Context, accessToken string, np model.NewPost) (model.Post, error)
	Repost(ctx context.Context, accessToken, postID, text string) (model.Post, error)
	Delete(ctx context.Context, accessToken, id string) error
	Edit(ctx context.Context, accessToken, id, text string, photoIDs []string) (model.Post, error)
	Pin(ctx context.Context, accessToken, id string) error
	Unpin(ctx context.Context, accessToken, id string) error
	Get(ctx context.Context, accessToken, id string) (model.Post, error)
	GetMany(ctx context.Context, ids []string) ([]model.Post, error)
	List(ctx context.Context, accessToken, wallID, cursor string, limit int) (model.PostPage, error)
	Feed(ctx context.Context, accessToken string, sources model.FeedSources, cursor string, limit int) (model.PostPage, error)

	Like(ctx context.Context, accessToken, postID string) (int, error)
	Unlike(ctx context.Context, accessToken, postID string) (int, error)

	CreateComment(ctx context.Context, accessToken, postID string, c model.NewComment) (model.Comment, error)
	LikeComment(ctx context.Context, accessToken, id string) (int, error)
	UnlikeComment(ctx context.Context, accessToken, id string) (int, error)
	DeleteComment(ctx context.Context, accessToken, id string) error
	Comments(ctx context.Context, accessToken, postID, cursor string, limit int) (model.CommentPage, error)
	CommentsByID(ctx context.Context, ids []string) ([]model.Comment, error)
}

type Notifications interface {
	List(ctx context.Context, accessToken string, unread bool, cursor string, limit int) (model.NotificationPage, error)
	CountUnread(ctx context.Context, accessToken string) (int, error)
	MarkRead(ctx context.Context, accessToken, id string) error
	Subscribe(ctx context.Context, accessToken string) (model.NotificationChanges, error)
}

type Messages interface {
	Send(ctx context.Context, accessToken, recipientID, text string, photoIDs []string) (model.Message, error)
	Dialogs(ctx context.Context, accessToken, cursor string, limit int) (model.DialogPage, error)
	CountUnread(ctx context.Context, accessToken string) (int, error)
	History(ctx context.Context, accessToken, peerID, cursor, after string, limit int) (model.MessagePage, error)
	MarkRead(ctx context.Context, accessToken, peerID, upTo string) error
	Delete(ctx context.Context, accessToken, id string) error
	HasMessagesFrom(ctx context.Context, accessToken, peerID string) (bool, error)
	Subscribe(ctx context.Context, accessToken string) (model.MessageEvents, error)
}

// Clubs: Get and Members take an optional access token ("" for anonymous
// viewers); a club ref is its ID or its address.
type Clubs interface {
	Create(ctx context.Context, accessToken, name, description string) (model.ClubView, error)
	Get(ctx context.Context, accessToken, ref string) (model.ClubView, error)
	GetMany(ctx context.Context, ids []string) ([]model.Club, error)
	Update(ctx context.Context, accessToken, clubID string, changes model.ClubChanges) (model.ClubView, error)
	Join(ctx context.Context, accessToken, clubID string) (model.ClubView, error)
	Leave(ctx context.Context, accessToken, clubID string) (model.ClubView, error)
	Members(ctx context.Context, clubID, cursor string, limit int, adminsOnly bool) (model.ClubMemberPage, error)
	SetAdmin(ctx context.Context, accessToken, clubID, userID string, admin bool) error
	Transfer(ctx context.Context, accessToken, clubID, userID string) error
	OfUser(ctx context.Context, userID, cursor string, limit int) (model.UserClubPage, error)
	Mine(ctx context.Context, accessToken, cursor string, limit int, managed bool) (model.ClubPage, error)
	Search(ctx context.Context, query, cursor string, limit int) (model.ClubSearchPage, error)
	SetAvatar(ctx context.Context, accessToken, clubID, avatarID string) (model.ClubView, error)
	Pin(ctx context.Context, accessToken, clubID string, pinned bool) error
	Pinned(ctx context.Context, accessToken string) ([]model.Club, error)
	MineIDs(ctx context.Context, accessToken string) ([]string, error)
	CanPostOnWall(ctx context.Context, accessToken, clubID string, fromClub bool) (bool, error)
}

// Backends are the services the API is made of.
type Backends struct {
	Auth          Auth
	Users         Users
	Posts         Posts
	Media         Media
	Notifications Notifications
	Messages      Messages
	Clubs         Clubs
}

type api struct {
	auth              Auth
	users             Users
	posts             Posts
	media             Media
	notifications     Notifications
	messages          Messages
	clubs             Clubs
	logger            *slog.Logger
	trustForwardedFor bool
	webDir            string
	eventOrigins      []string
	eventStreams      prometheus.Gauge
	// stopping ends when the server stops, closing the event streams,
	// whose connections Shutdown leaves alone; streams counts them.
	stopping context.Context
	streams  sync.WaitGroup
}

func (a *api) routes() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET "+HealthPath, a.health)

	mux.HandleFunc("POST /api/v1/auth/code", a.requestCode)
	mux.HandleFunc("POST /api/v1/auth/verify", a.verifyCode)
	mux.HandleFunc("POST /api/v1/auth/refresh", a.refreshToken)
	mux.HandleFunc("POST /api/v1/auth/logout", a.logout)

	mux.HandleFunc("GET /api/v1/me", a.getMe)
	mux.HandleFunc("PUT /api/v1/me/profile", a.updateProfile)
	mux.HandleFunc("PUT /api/v1/me/privacy", a.updatePrivacy)
	mux.HandleFunc("POST /api/v1/me/deactivate", a.deactivate)
	mux.HandleFunc("POST /api/v1/me/reactivate", a.reactivate)

	mux.HandleFunc("GET /api/v1/users/{nickname}", a.getProfile)

	mux.HandleFunc("GET /api/v1/me/blacklist", a.listBlacklist)
	mux.HandleFunc("PUT /api/v1/me/blacklist/{nickname}", a.block)
	mux.HandleFunc("DELETE /api/v1/me/blacklist/{nickname}", a.unblock)

	mux.HandleFunc("GET /api/v1/users/{nickname}/posts", a.listPosts)
	mux.HandleFunc("POST /api/v1/users/{nickname}/posts", a.createWallPost)
	mux.HandleFunc("POST /api/v1/me/posts", a.createPost)
	mux.HandleFunc("GET /api/v1/posts/{id}", a.getPost)
	mux.HandleFunc("DELETE /api/v1/posts/{id}", a.deletePost)
	mux.HandleFunc("PATCH /api/v1/posts/{id}", a.editPost)
	mux.HandleFunc("PUT /api/v1/posts/{id}/pin", a.pinPost)
	mux.HandleFunc("DELETE /api/v1/posts/{id}/pin", a.unpinPost)
	mux.HandleFunc("POST /api/v1/posts/{id}/reposts", a.repostPost)
	mux.HandleFunc("PUT /api/v1/posts/{id}/like", a.likePost)
	mux.HandleFunc("DELETE /api/v1/posts/{id}/like", a.unlikePost)

	mux.HandleFunc("GET /api/v1/posts/{id}/comments", a.listComments)
	mux.HandleFunc("POST /api/v1/posts/{id}/comments", a.createComment)
	mux.HandleFunc("DELETE /api/v1/comments/{id}", a.deleteComment)
	mux.HandleFunc("PUT /api/v1/comments/{id}/like", a.likeComment)
	mux.HandleFunc("DELETE /api/v1/comments/{id}/like", a.unlikeComment)

	mux.HandleFunc("POST /api/v1/users/{nickname}/follow", a.follow)
	mux.HandleFunc("DELETE /api/v1/users/{nickname}/follow", a.unfollow)
	mux.HandleFunc("GET /api/v1/users/{nickname}/followers", a.listFollowers)
	mux.HandleFunc("GET /api/v1/users/{nickname}/following", a.listFollowing)
	mux.HandleFunc("GET /api/v1/users/{nickname}/clubs", a.listUserClubs)

	mux.HandleFunc("GET /api/v1/feed", a.feed)

	mux.HandleFunc("GET /api/v1/users", a.searchUsers)

	mux.HandleFunc("GET /api/v1/notifications", a.listNotifications)
	mux.HandleFunc("GET /api/v1/notifications/unread-count", a.countUnreadNotifications)
	mux.HandleFunc("POST /api/v1/notifications/read", a.markNotificationsRead)

	mux.HandleFunc("GET "+eventsPath, a.events)

	mux.HandleFunc("GET /api/v1/dialogs", a.listDialogs)
	mux.HandleFunc("GET /api/v1/dialogs/unread-count", a.countUnreadDialogs)
	mux.HandleFunc("GET /api/v1/dialogs/{nickname}/messages", a.listMessages)
	mux.HandleFunc("POST /api/v1/dialogs/{nickname}/messages", a.sendMessage)
	mux.HandleFunc("POST /api/v1/dialogs/{nickname}/read", a.markDialogRead)
	mux.HandleFunc("DELETE /api/v1/messages/{id}", a.deleteMessage)

	mux.HandleFunc("POST /api/v1/clubs", a.createClub)
	mux.HandleFunc("GET /api/v1/me/clubs", a.listMyClubs)
	mux.HandleFunc("GET /api/v1/me/clubs/pinned", a.listPinnedClubs)
	mux.HandleFunc("GET /api/v1/clubs", a.searchClubs)
	mux.HandleFunc("GET /api/v1/clubs/{ref}", a.getClub)
	mux.HandleFunc("PUT /api/v1/clubs/{ref}", a.updateClub)
	mux.HandleFunc("PUT /api/v1/clubs/{ref}/avatar", a.uploadClubAvatar)
	mux.HandleFunc("DELETE /api/v1/clubs/{ref}/avatar", a.deleteClubAvatar)
	mux.HandleFunc("PUT /api/v1/clubs/{ref}/pin", a.pinClub)
	mux.HandleFunc("DELETE /api/v1/clubs/{ref}/pin", a.unpinClub)
	mux.HandleFunc("PUT /api/v1/clubs/{ref}/membership", a.joinClub)
	mux.HandleFunc("DELETE /api/v1/clubs/{ref}/membership", a.leaveClub)
	mux.HandleFunc("GET /api/v1/clubs/{ref}/members", a.listClubMembers)
	mux.HandleFunc("PUT /api/v1/clubs/{ref}/admins/{nickname}", a.appointAdmin)
	mux.HandleFunc("DELETE /api/v1/clubs/{ref}/admins/{nickname}", a.dismissAdmin)
	mux.HandleFunc("PUT /api/v1/clubs/{ref}/owner/{nickname}", a.transferClub)
	mux.HandleFunc("GET /api/v1/clubs/{ref}/posts", a.listClubPosts)
	mux.HandleFunc("POST /api/v1/clubs/{ref}/posts", a.createClubPost)

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
