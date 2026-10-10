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
	// UpdateProfile: partner is the nickname of whom the relationship
	// names, "" for nobody.
	UpdateProfile(ctx context.Context, accessToken string, p model.Profile, partner string) (model.User, error)
	UpdateContacts(ctx context.Context, accessToken string, c model.Contacts) (model.User, error)
	UpdateInterests(ctx context.Context, accessToken string, in model.Interests) (model.User, error)
	UpdateExtraFields(ctx context.Context, accessToken string, fields []model.ExtraField) (model.User, error)
	UpdatePrivacy(ctx context.Context, accessToken string, p model.Privacy) (model.User, error)
	// GetProfile is the page of nickname as the caller, anonymous without
	// accessToken, sees it.
	GetProfile(ctx context.Context, accessToken, nickname string) (model.PublicProfile, error)
	GetProfiles(ctx context.Context, ids []string) ([]model.PublicProfile, error)
	CanPostOnWall(ctx context.Context, accessToken, userID string) (bool, error)
	CanMessage(ctx context.Context, accessToken, userID string, wroteFirst bool) (bool, error)
	// ResolveMentions are the IDs of the people the caller names by these
	// nicknames who hear of it: who exist and whom the caller may name.
	ResolveMentions(ctx context.Context, accessToken string, nicknames []string) ([]string, error)
	// Access is what of userID's page the caller may see.
	Access(ctx context.Context, accessToken, userID string) (model.Access, error)

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
	// ListOnlineFollowing is whom userID follows who is online now.
	ListOnlineFollowing(ctx context.Context, userID, cursor string, limit int) (model.UserPage, error)
	// ListCommonFollowing is whom both userID and the caller follow.
	ListCommonFollowing(ctx context.Context, accessToken, userID, cursor string, limit int) (model.UserPage, error)
	UpdateStatus(ctx context.Context, accessToken, status string) (model.User, error)
	UpdateInterface(ctx context.Context, accessToken string, ui model.Interface) (model.User, error)
	// MarkOnline: the caller is on the site now.
	MarkOnline(ctx context.Context, accessToken string) error
	// Statistics counts the people of the site and those on it now.
	Statistics(ctx context.Context) (users, online int, err error)
	// SetBackdrop shows photoID ("" for none) on a side of the caller's
	// page, telling the photo it showed before.
	SetBackdrop(ctx context.Context, accessToken, side, photoID string) (model.User, string, error)

	SetAvatar(ctx context.Context, accessToken, avatarID string) (model.User, error)
	Search(ctx context.Context, q model.UserQuery, cursor string, limit int) (model.UserPage, error)
}

type Media interface {
	UploadAvatar(ctx context.Context, accessToken string, image []byte) (string, error)
	DeleteAvatars(ctx context.Context, accessToken, keep string) error
	Avatar(ctx context.Context, userID, avatarID string, small bool) ([]byte, error)
	// The photos of a club, served as avatars under its ID; the caller
	// checks that the user runs the club.
	UploadClubAvatar(ctx context.Context, accessToken, clubID string, image []byte) (string, error)
	DeleteClubAvatars(ctx context.Context, accessToken, clubID, keep string) error
	UploadClubPhoto(ctx context.Context, accessToken, clubID string, image []byte) (string, error)
	DeleteClubPhotos(ctx context.Context, accessToken, clubID string, keep []string) error

	UploadPhoto(ctx context.Context, accessToken string, image []byte) (string, error)
	OwnPhotos(ctx context.Context, accessToken string, photoIDs []string) ([]string, error)
	Photo(ctx context.Context, userID, photoID string, small bool) ([]byte, error)
	// PublishPhotos keeps uploaded photos of ownerID for good, as a side
	// of the backdrop; DeletePublishedPhotos deletes such photos.
	PublishPhotos(ctx context.Context, ownerID string, photoIDs []string) error
	DeletePublishedPhotos(ctx context.Context, ownerID string, photoIDs []string) error
}

// Posts: methods that read take an optional access token ("" for
// anonymous viewers), with which posts tell whether the viewer likes them.
type Posts interface {
	Create(ctx context.Context, accessToken string, np model.NewPost) (model.Post, error)
	Repost(ctx context.Context, accessToken, postID string, nr model.NewRepost) (model.Post, error)
	Delete(ctx context.Context, accessToken, id string) error
	Edit(ctx context.Context, accessToken, id string, edit model.PostEdit) (model.Post, error)
	Pin(ctx context.Context, accessToken, id string) error
	Unpin(ctx context.Context, accessToken, id string) error
	Get(ctx context.Context, accessToken, id string) (model.Post, error)
	GetMany(ctx context.Context, ids []string) ([]model.Post, error)
	// Count counts the posts on all walls.
	Count(ctx context.Context) (int, error)
	// Archive puts a post away in the archive of its wall, or back;
	// CloseComments lets nobody comment it anymore, or again. The owner of
	// the wall only.
	Archive(ctx context.Context, accessToken, id string, archived bool) error
	CloseComments(ctx context.Context, accessToken, id string, closed bool) error
	// List pages a wall: Filter is "", "owner", "others" or "archive"
	// (InvalidArgument otherwise), as the tabs of openvk's wall, Query
	// what the posts say ("Поиск").
	List(ctx context.Context, accessToken, wallID string, pick model.WallPick, cursor string, limit int) (model.PostPage, error)
	Feed(ctx context.Context, accessToken string, sources model.FeedSources, cursor string, limit int) (model.PostPage, error)

	Like(ctx context.Context, accessToken, postID string) (int, error)
	Unlike(ctx context.Context, accessToken, postID string) (int, error)

	CreateComment(ctx context.Context, accessToken, postID string, c model.NewComment) (model.Comment, error)
	LikeComment(ctx context.Context, accessToken, id string) (int, error)
	// PostLikers and CommentLikers are who like it, newest first.
	PostLikers(ctx context.Context, postID, cursor string, limit int) (model.LikerPage, error)
	CommentLikers(ctx context.Context, commentID, cursor string, limit int) (model.LikerPage, error)
	UnlikeComment(ctx context.Context, accessToken, id string) (int, error)
	DeleteComment(ctx context.Context, accessToken, id string) error
	Comments(ctx context.Context, accessToken, postID, cursor string, limit int) (model.CommentPage, error)
	LastComments(ctx context.Context, accessToken string, postIDs []string) ([]model.Comment, error)
	CommentsByID(ctx context.Context, ids []string) ([]model.Comment, error)
	// Comment is one comment, whole; the lists carry beginnings of texts.
	Comment(ctx context.Context, id string) (model.Comment, error)

	Suggest(ctx context.Context, accessToken, wallID, text string, photoIDs []string) (model.Suggestion, error)
	Suggestions(ctx context.Context, accessToken, wallID string, mine bool, cursor string, limit int) (model.SuggestionPage, error)
	// Suggestion is a suggestion with its whole text.
	Suggestion(ctx context.Context, accessToken, id string) (model.Suggestion, error)
	PublishSuggestion(ctx context.Context, accessToken, id, text string, signed bool) (model.Post, error)
	DeleteSuggestion(ctx context.Context, accessToken, id string) error
}

// Topics are the discussions of communities; methods that read take an
// optional access token, with which topics and messages tell whether they
// are the viewer's.
type Topics interface {
	Create(ctx context.Context, accessToken string, nt model.NewTopic) (model.Topic, error)
	Get(ctx context.Context, accessToken, id string) (model.Topic, error)
	List(ctx context.Context, accessToken, clubID, query, cursor string, limit int) (model.TopicPage, error)
	Edit(ctx context.Context, accessToken, id string, changes model.TopicChanges) (model.Topic, error)
	Delete(ctx context.Context, accessToken, id string) error
	Write(ctx context.Context, accessToken, topicID string, nm model.NewTopicMessage) (model.TopicMessage, error)
	Messages(ctx context.Context, accessToken, topicID, cursor string, limit int) (model.TopicMessagePage, error)
	// Message is a message of a topic with its whole text.
	Message(ctx context.Context, id string) (model.TopicMessage, error)
	DeleteMessage(ctx context.Context, accessToken, id string) error
	LikeMessage(ctx context.Context, accessToken, id string) (int, error)
	UnlikeMessage(ctx context.Context, accessToken, id string) (int, error)
}

// Albums are the photo albums of people and communities; methods that
// read take an optional access token, with which photos tell whether the
// viewer uploaded them.
type Albums interface {
	Create(ctx context.Context, accessToken string, na model.NewAlbum) (model.Album, error)
	Get(ctx context.Context, accessToken, id string) (model.Album, error)
	List(ctx context.Context, accessToken, ownerID, cursor string, limit int) (model.AlbumPage, error)
	Edit(ctx context.Context, accessToken, id, name, description string) (model.Album, error)
	Delete(ctx context.Context, accessToken, id string) error
	AddPhotos(ctx context.Context, accessToken, albumID string, photos []model.NewAlbumPhoto) ([]model.AlbumPhoto, error)
	// AddAvatarPhoto puts a new photo of a page in its "Фотографии со
	// страницы": the caller's, or of a club they run (clubID).
	AddAvatarPhoto(ctx context.Context, accessToken, clubID, photoID string) error
	Photos(ctx context.Context, accessToken, albumID, cursor string, limit int) (model.AlbumPhotoPage, error)
	Photo(ctx context.Context, accessToken, id string) (model.AlbumPhoto, model.Album, error)
	Describe(ctx context.Context, accessToken, id, description string) (model.AlbumPhoto, error)
	DeletePhoto(ctx context.Context, accessToken, id string) error
	// SetCover makes a photo of the album show it; MovePhoto puts a photo
	// in another album of the same owner.
	SetCover(ctx context.Context, accessToken, albumID, photoID string) error
	MovePhoto(ctx context.Context, accessToken, photoID, albumID string) error
	// The heart, the likers and the comments under a photo.
	LikePhoto(ctx context.Context, accessToken, id string) (int, error)
	UnlikePhoto(ctx context.Context, accessToken, id string) (int, error)
	PhotoLikers(ctx context.Context, id, cursor string, limit int) (model.LikerPage, error)
	CommentPhoto(ctx context.Context, accessToken, id, text string) (model.PhotoComment, error)
	DeletePhotoComment(ctx context.Context, accessToken, id string) error
	PhotoComments(ctx context.Context, accessToken, id, cursor string, limit int) (model.PhotoCommentPage, error)
	// PhotoComment is a comment under a photo with its whole text.
	PhotoComment(ctx context.Context, id string) (model.PhotoComment, error)
}

type Notifications interface {
	List(ctx context.Context, accessToken string, unread bool, cursor string, limit int) (model.NotificationPage, error)
	CountUnread(ctx context.Context, accessToken string) (int, error)
	MarkRead(ctx context.Context, accessToken, id string) error
	Subscribe(ctx context.Context, accessToken string) (model.NotificationChanges, error)
}

type Messages interface {
	Send(ctx context.Context, accessToken, recipientID string, d model.MessageDraft) (model.Message, error)
	Dialogs(ctx context.Context, accessToken, cursor string, limit int) (model.DialogPage, error)
	CountUnread(ctx context.Context, accessToken string) (int, error)
	History(ctx context.Context, accessToken, peerID string, at model.MessagesAt, limit int) (model.MessagePage, error)
	// Whole is a message of the dialog with peerID with its whole text.
	Whole(ctx context.Context, accessToken, peerID, id string) (model.Message, error)
	MarkRead(ctx context.Context, accessToken, peerID, upTo string) error
	Delete(ctx context.Context, accessToken, id string) error
	// Hide deletes a message for the caller alone; Edit changes what
	// their own says; Search finds theirs that say query, in the dialog
	// with peerID or, for "", in all.
	Hide(ctx context.Context, accessToken, id string) error
	Edit(ctx context.Context, accessToken, id, text string) (model.Message, error)
	Search(ctx context.Context, accessToken, peerID, query, cursor string, limit int) (model.FoundMessages, error)
	HasMessagesFrom(ctx context.Context, accessToken, peerID string) (bool, error)
	Subscribe(ctx context.Context, accessToken string) (model.MessageEvents, error)
}

// Clubs: Get and Members take an optional access token ("" for anonymous
// viewers); a club ref is its ID or its address.
type Clubs interface {
	Create(ctx context.Context, accessToken, name, description string) (model.ClubView, error)
	CreateEvent(ctx context.Context, accessToken string, e model.NewEvent) (model.ClubView, error)
	Get(ctx context.Context, accessToken, ref string) (model.ClubView, error)
	GetMany(ctx context.Context, ids []string) ([]model.Club, error)
	// Count counts the groups, events not among them.
	Count(ctx context.Context) (int, error)
	Update(ctx context.Context, accessToken, clubID string, changes model.ClubChanges) (model.ClubView, error)
	Join(ctx context.Context, accessToken, clubID string, maybe bool) (model.ClubView, error)
	Leave(ctx context.Context, accessToken, clubID string) (model.ClubView, error)
	Members(ctx context.Context, accessToken, clubID, cursor string, limit int, filter model.ClubMemberFilter) (model.ClubMemberPage, error)
	Describe(ctx context.Context, accessToken, clubID, userID, title string, hidden bool) error
	SetAdmin(ctx context.Context, accessToken, clubID, userID string, admin bool) error
	Transfer(ctx context.Context, accessToken, clubID, userID string) error
	OfUser(ctx context.Context, userID, cursor string, limit int, filter model.ClubFilter) (model.UserClubPage, error)
	Mine(ctx context.Context, accessToken, cursor string, limit int, filter model.ClubFilter) (model.ClubPage, error)
	Search(ctx context.Context, query, cursor string, limit int) (model.ClubSearchPage, error)
	SetAvatar(ctx context.Context, accessToken, clubID, avatarID string) (model.ClubView, error)
	SetBackdrop(ctx context.Context, accessToken, clubID, side, photoID string) (model.ClubView, error)
	Pin(ctx context.Context, accessToken, clubID string, pinned bool) error
	Pinned(ctx context.Context, accessToken string) ([]model.Club, error)
	MineIDs(ctx context.Context, accessToken string) ([]string, error)
	CanPostOnWall(ctx context.Context, accessToken, clubID string, fromClub bool) (model.WallAccess, error)
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
	Topics        Topics
	Albums        Albums
}

type api struct {
	auth              Auth
	users             Users
	posts             Posts
	media             Media
	notifications     Notifications
	messages          Messages
	clubs             Clubs
	topics            Topics
	albums            Albums
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
	mux.HandleFunc("PUT /api/v1/me/contacts", a.updateContacts)
	mux.HandleFunc("PUT /api/v1/me/status", a.updateStatus)
	mux.HandleFunc("PUT /api/v1/me/interests", a.updateInterests)
	mux.HandleFunc("PUT /api/v1/me/fields", a.updateExtraFields)
	mux.HandleFunc("PUT /api/v1/me/privacy", a.updatePrivacy)
	mux.HandleFunc("PUT /api/v1/me/interface", a.updateInterface)
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
	mux.HandleFunc("PUT /api/v1/posts/{id}/archive", a.archivePost)
	mux.HandleFunc("DELETE /api/v1/posts/{id}/archive", a.unarchivePost)
	mux.HandleFunc("PUT /api/v1/posts/{id}/closed-comments", a.closeComments)
	mux.HandleFunc("DELETE /api/v1/posts/{id}/closed-comments", a.openComments)
	mux.HandleFunc("POST /api/v1/posts/{id}/reposts", a.repostPost)
	mux.HandleFunc("PUT /api/v1/posts/{id}/like", a.likePost)
	mux.HandleFunc("DELETE /api/v1/posts/{id}/like", a.unlikePost)
	mux.HandleFunc("GET /api/v1/posts/{id}/likes", a.listPostLikers)

	mux.HandleFunc("GET /api/v1/posts/{id}/comments", a.listComments)
	mux.HandleFunc("POST /api/v1/posts/{id}/comments", a.createComment)
	mux.HandleFunc("GET /api/v1/comments/{id}/text", a.commentText)
	mux.HandleFunc("DELETE /api/v1/comments/{id}", a.deleteComment)
	mux.HandleFunc("PUT /api/v1/comments/{id}/like", a.likeComment)
	mux.HandleFunc("DELETE /api/v1/comments/{id}/like", a.unlikeComment)
	mux.HandleFunc("GET /api/v1/comments/{id}/likes", a.listCommentLikers)

	mux.HandleFunc("POST /api/v1/users/{nickname}/follow", a.follow)
	mux.HandleFunc("DELETE /api/v1/users/{nickname}/follow", a.unfollow)
	mux.HandleFunc("GET /api/v1/users/{nickname}/followers", a.listFollowers)
	mux.HandleFunc("GET /api/v1/users/{nickname}/following", a.listFollowing)
	mux.HandleFunc("GET /api/v1/users/{nickname}/clubs", a.listUserClubs)
	mux.HandleFunc("GET /api/v1/users/{nickname}/events", a.listUserEvents)

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
	mux.HandleFunc("GET /api/v1/dialogs/{nickname}/messages/{id}", a.getMessage)
	mux.HandleFunc("POST /api/v1/dialogs/{nickname}/read", a.markDialogRead)
	mux.HandleFunc("GET /api/v1/messages/search", a.searchMessages)
	mux.HandleFunc("PATCH /api/v1/messages/{id}", a.editMessage)
	mux.HandleFunc("DELETE /api/v1/messages/{id}", a.deleteMessage)

	mux.HandleFunc("POST /api/v1/clubs", a.createClub)
	mux.HandleFunc("POST /api/v1/events", a.createEvent)
	mux.HandleFunc("GET /api/v1/about", a.about)
	mux.HandleFunc("GET /api/v1/me/clubs", a.listMyClubs)
	mux.HandleFunc("GET /api/v1/me/events", a.listMyEvents)
	mux.HandleFunc("GET /api/v1/me/clubs/pinned", a.listPinnedClubs)
	mux.HandleFunc("GET /api/v1/clubs", a.searchClubs)
	mux.HandleFunc("GET /api/v1/clubs/{ref}", a.getClub)
	mux.HandleFunc("PUT /api/v1/clubs/{ref}", a.updateClub)
	mux.HandleFunc("PUT /api/v1/clubs/{ref}/avatar", a.uploadClubAvatar)
	mux.HandleFunc("DELETE /api/v1/clubs/{ref}/avatar", a.deleteClubAvatar)
	mux.HandleFunc("PUT /api/v1/clubs/{ref}/backdrop/{side}", a.uploadClubBackdrop)
	mux.HandleFunc("DELETE /api/v1/clubs/{ref}/backdrop", a.deleteClubBackdrop)
	mux.HandleFunc("PUT /api/v1/me/backdrop/{side}", a.uploadBackdrop)
	mux.HandleFunc("DELETE /api/v1/me/backdrop", a.deleteBackdrop)
	mux.HandleFunc("PUT /api/v1/clubs/{ref}/pin", a.pinClub)
	mux.HandleFunc("DELETE /api/v1/clubs/{ref}/pin", a.unpinClub)
	mux.HandleFunc("PUT /api/v1/clubs/{ref}/membership", a.joinClub)
	mux.HandleFunc("DELETE /api/v1/clubs/{ref}/membership", a.leaveClub)
	mux.HandleFunc("GET /api/v1/clubs/{ref}/members", a.listClubMembers)
	mux.HandleFunc("PUT /api/v1/clubs/{ref}/admins/{nickname}", a.appointAdmin)
	mux.HandleFunc("DELETE /api/v1/clubs/{ref}/admins/{nickname}", a.dismissAdmin)
	mux.HandleFunc("PATCH /api/v1/clubs/{ref}/admins/{nickname}", a.describeAdmin)
	mux.HandleFunc("PUT /api/v1/clubs/{ref}/owner/{nickname}", a.transferClub)
	mux.HandleFunc("GET /api/v1/clubs/{ref}/posts", a.listClubPosts)
	mux.HandleFunc("POST /api/v1/clubs/{ref}/posts", a.createClubPost)
	mux.HandleFunc("GET /api/v1/clubs/{ref}/suggestions", a.listClubSuggestions)
	mux.HandleFunc("POST /api/v1/clubs/{ref}/suggestions", a.suggestClubPost)
	mux.HandleFunc("POST /api/v1/suggestions/{id}/publish", a.publishSuggestion)
	mux.HandleFunc("DELETE /api/v1/suggestions/{id}", a.deleteSuggestion)
	mux.HandleFunc("GET /api/v1/suggestions/{id}/text", a.suggestionText)
	mux.HandleFunc("GET /api/v1/clubs/{ref}/topics", a.listTopics)
	mux.HandleFunc("POST /api/v1/clubs/{ref}/topics", a.createTopic)
	mux.HandleFunc("GET /api/v1/topics/{id}", a.getTopic)
	mux.HandleFunc("PATCH /api/v1/topics/{id}", a.editTopic)
	mux.HandleFunc("DELETE /api/v1/topics/{id}", a.deleteTopic)
	mux.HandleFunc("GET /api/v1/topics/{id}/messages", a.listTopicMessages)
	mux.HandleFunc("POST /api/v1/topics/{id}/messages", a.createTopicMessage)
	mux.HandleFunc("DELETE /api/v1/topic-messages/{id}", a.deleteTopicMessage)
	mux.HandleFunc("GET /api/v1/topic-messages/{id}/text", a.topicMessageText)
	mux.HandleFunc("PUT /api/v1/topic-messages/{id}/like", a.likeTopicMessage)
	mux.HandleFunc("DELETE /api/v1/topic-messages/{id}/like", a.unlikeTopicMessage)

	mux.HandleFunc("GET /api/v1/users/{nickname}/albums", a.listUserAlbums)
	mux.HandleFunc("GET /api/v1/clubs/{ref}/albums", a.listClubAlbums)
	mux.HandleFunc("POST /api/v1/me/albums", a.createAlbum)
	mux.HandleFunc("POST /api/v1/clubs/{ref}/albums", a.createClubAlbum)
	mux.HandleFunc("GET /api/v1/albums/{id}", a.getAlbum)
	mux.HandleFunc("PATCH /api/v1/albums/{id}", a.editAlbum)
	mux.HandleFunc("DELETE /api/v1/albums/{id}", a.deleteAlbum)
	mux.HandleFunc("PUT /api/v1/albums/{id}/cover", a.setAlbumCover)
	mux.HandleFunc("GET /api/v1/albums/{id}/photos", a.listAlbumPhotos)
	mux.HandleFunc("POST /api/v1/albums/{id}/photos", a.addAlbumPhotos)
	mux.HandleFunc("GET /api/v1/album-photos/{id}", a.getAlbumPhoto)
	mux.HandleFunc("PATCH /api/v1/album-photos/{id}", a.editAlbumPhoto)
	mux.HandleFunc("DELETE /api/v1/album-photos/{id}", a.deleteAlbumPhoto)
	mux.HandleFunc("PUT /api/v1/album-photos/{id}/album", a.moveAlbumPhoto)
	mux.HandleFunc("PUT /api/v1/album-photos/{id}/like", a.likeAlbumPhoto)
	mux.HandleFunc("DELETE /api/v1/album-photos/{id}/like", a.unlikeAlbumPhoto)
	mux.HandleFunc("GET /api/v1/album-photos/{id}/likes", a.listAlbumPhotoLikers)
	mux.HandleFunc("GET /api/v1/album-photos/{id}/comments", a.listPhotoComments)
	mux.HandleFunc("POST /api/v1/album-photos/{id}/comments", a.createPhotoComment)
	mux.HandleFunc("DELETE /api/v1/photo-comments/{id}", a.deletePhotoComment)
	mux.HandleFunc("GET /api/v1/photo-comments/{id}/text", a.photoCommentText)
	mux.HandleFunc("POST /api/v1/album-photos/{id}/reposts", a.shareAlbumPhoto)

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
