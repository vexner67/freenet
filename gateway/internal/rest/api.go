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
	// VerifyCode and RefreshToken say where the user is, for the list of
	// their sessions.
	VerifyCode(ctx context.Context, email, code string, client model.Client) (model.Tokens, error)
	RefreshToken(ctx context.Context, refreshToken string, client model.Client) (model.Tokens, error)
	Logout(ctx context.Context, refreshToken string) error
	ListSessions(ctx context.Context, accessToken string) ([]model.Session, error)
	EndSession(ctx context.Context, accessToken, sessionID string) error
	// EndOtherSessions returns how many sessions ended.
	EndOtherSessions(ctx context.Context, accessToken string) (int, error)
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
	// ResolveInvites are the people the caller names by nickname to add
	// to a chat, and whether each lets them; nobody for an unknown one.
	ResolveInvites(ctx context.Context, accessToken string, nicknames []string) ([]model.Invitee, error)
	// KnownNicknames are those of nicknames that are someone's, as their
	// owners write them.
	KnownNicknames(ctx context.Context, nicknames []string) ([]string, error)
	// Access is what of userID's page the caller may see.
	Access(ctx context.Context, accessToken, userID string) (model.Access, error)

	Block(ctx context.Context, accessToken, userID string) error
	Unblock(ctx context.Context, accessToken, userID string) error
	Blacklist(ctx context.Context, accessToken, cursor string, limit int) (model.UserPage, error)
	Relation(ctx context.Context, accessToken, userID string) (model.Relation, error)

	// Ignore leaves the wall of a user or a community out of the caller's
	// news ("Игнорируемые источники"); IgnoredIDs are those walls.
	Ignore(ctx context.Context, accessToken, sourceID string) error
	Unignore(ctx context.Context, accessToken string, sourceIDs []string) error
	IgnoredIDs(ctx context.Context, accessToken string) ([]string, error)

	Follow(ctx context.Context, accessToken, userID string) error
	Unfollow(ctx context.Context, accessToken, userID string) error
	BlockedIDs(ctx context.Context, accessToken string) ([]string, error)
	FollowStats(ctx context.Context, accessToken, userID string) (model.FollowStats, error)
	ListFollowers(ctx context.Context, userID, cursor string, limit int) (model.UserPage, error)
	ListFollowing(ctx context.Context, userID, cursor string, limit int) (model.UserPage, error)
	// ListOnlineFollowing is whom userID follows who is online now.
	ListOnlineFollowing(ctx context.Context, userID, cursor string, limit int) (model.UserPage, error)
	// ListCommonFollowing is whom both userID and the caller follow.
	ListCommonFollowing(ctx context.Context, accessToken, userID, cursor string, limit int) (model.UserPage, error)
	UpdateStatus(ctx context.Context, accessToken, status string) (model.User, error)
	UpdateNickname(ctx context.Context, accessToken, nickname string) (model.User, error)
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
	// ClaimPhotos takes the caller's uploads for the one post that is
	// about to show them, all or none, telling whether it did;
	// ReleasePhotos gives them back when the post was refused.
	ClaimPhotos(ctx context.Context, accessToken string, photoIDs []string) (bool, error)
	ReleasePhotos(ctx context.Context, accessToken string, photoIDs []string) error
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
	// Reposts are the posts that share a post, newest first.
	Reposts(ctx context.Context, accessToken, postID, cursor string, limit int) (model.PostPage, error)
	Delete(ctx context.Context, accessToken, id string) error
	Edit(ctx context.Context, accessToken, id string, edit model.PostEdit) (model.Post, error)
	Pin(ctx context.Context, accessToken, id string) error
	Unpin(ctx context.Context, accessToken, id string) error
	Get(ctx context.Context, accessToken, id string) (model.Post, error)
	GetMany(ctx context.Context, accessToken string, ids []string) ([]model.Post, error)
	// Count counts the posts on all walls.
	Count(ctx context.Context) (int, error)
	// Archive puts a post away in the archive of its wall, or back;
	// CloseComments lets nobody comment it anymore, or again. The owner of
	// the wall only.
	Archive(ctx context.Context, accessToken, id string, archived bool) error
	CloseComments(ctx context.Context, accessToken, id string, closed bool) error
	// ArchiveWall puts away every post of a wall written in year (0: in
	// any), or brings them back, and tells how many; WallYears are the
	// years its posts were written in, the latest first. Those who keep
	// the archive of the wall only.
	ArchiveWall(ctx context.Context, accessToken, wallID string, year int, archived bool) (int, error)
	WallYears(ctx context.Context, accessToken, wallID string) ([]int, error)
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
	// Polls are the polls of those of the posts (at most 100) that ask
	// one, as the viewer sees them; Vote and RetractVote return the poll
	// as it became; PollVoters are who chose an option, newest first.
	Polls(ctx context.Context, accessToken string, postIDs []string) ([]model.Poll, error)
	Vote(ctx context.Context, accessToken, postID string, options []int) (model.Poll, error)
	RetractVote(ctx context.Context, accessToken, postID string) (model.Poll, error)
	PollVoters(ctx context.Context, postID string, option int, cursor string, limit int) (model.LikerPage, error)
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
	// MessagesByID are the messages that exist, with the beginnings of
	// their texts; MessageLikers are who put a heart on one.
	MessagesByID(ctx context.Context, ids []string) ([]model.TopicMessage, error)
	MessageLikers(ctx context.Context, id, cursor string, limit int) (model.LikerPage, error)
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
	CommentPhoto(ctx context.Context, accessToken, id string, c model.NewPhotoComment) (model.PhotoComment, error)
	DeletePhotoComment(ctx context.Context, accessToken, id string) error
	PhotoComments(ctx context.Context, accessToken, id, cursor string, limit int) (model.PhotoCommentPage, error)
	// PhotoComment is a comment under a photo with its whole text.
	PhotoComment(ctx context.Context, id string) (model.PhotoComment, error)
	// PhotoCommentsByID are the comments that exist, with the beginnings
	// of their texts.
	PhotoCommentsByID(ctx context.Context, ids []string) ([]model.PhotoComment, error)
	LikePhotoComment(ctx context.Context, accessToken, id string) (int, error)
	UnlikePhotoComment(ctx context.Context, accessToken, id string) (int, error)
	PhotoCommentLikers(ctx context.Context, id, cursor string, limit int) (model.LikerPage, error)
}

// Faves are the lists of the caller alone: what they like ("Закладки" of
// openvk), newest like first, and what they wrote ("Написанные мной
// записи"), newest first.
type Faves interface {
	Posts(ctx context.Context, accessToken, cursor string, limit int) (model.PostPage, error)
	Comments(ctx context.Context, accessToken, cursor string, limit int) (model.CommentPage, error)
	Photos(ctx context.Context, accessToken, cursor string, limit int) (model.FavePhotoPage, error)
	OwnPosts(ctx context.Context, accessToken, cursor string, limit int) (model.PostPage, error)
}

// Notes are the notes of people ("Заметки") and the comments under them.
type Notes interface {
	Create(ctx context.Context, accessToken, title, text string) (model.Note, error)
	// Get is a note with its whole text; List carries the beginnings.
	Get(ctx context.Context, accessToken, id string) (model.Note, error)
	List(ctx context.Context, accessToken, authorID, cursor string, limit int) (model.NotePage, error)
	Edit(ctx context.Context, accessToken, id, title, text string) (model.Note, error)
	Delete(ctx context.Context, accessToken, id string) error
	Comment(ctx context.Context, accessToken, noteID, text string) (model.NoteComment, error)
	DeleteComment(ctx context.Context, accessToken, id string) error
	Comments(ctx context.Context, accessToken, noteID, cursor string, limit int) (model.NoteCommentPage, error)
	// CommentByID is a comment with its whole text; CommentsByID are
	// those that exist, with the beginnings of their texts.
	CommentByID(ctx context.Context, id string) (model.NoteComment, error)
	CommentsByID(ctx context.Context, ids []string) ([]model.NoteComment, error)
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
	// DeleteMany deletes several messages at once, all or none: for both
	// sides, as Delete, or for the caller alone, as Hide.
	DeleteMany(ctx context.Context, accessToken string, ids []string, forEveryone bool) error
	Edit(ctx context.Context, accessToken, id string, e model.MessageEdit) (model.Message, error)
	Search(ctx context.Context, accessToken, peerID, query, cursor string, limit int) (model.FoundMessages, error)
	HasMessagesFrom(ctx context.Context, accessToken, peerID string) (bool, error)
	Subscribe(ctx context.Context, accessToken string) (model.MessageEvents, error)
}

// Chats are the chats of several people ("беседы"); all take the access
// token of someone in the chat. History reads older than cursor or, with
// after, newer than it.
type Chats interface {
	Create(ctx context.Context, accessToken, title string, memberIDs []string) (model.Chat, error)
	Get(ctx context.Context, accessToken, id string) (model.ChatView, error)
	List(ctx context.Context, accessToken, cursor string, limit int) (model.ChatPage, error)
	Rename(ctx context.Context, accessToken, id, title string) error
	Invite(ctx context.Context, accessToken, id string, userIDs []string) error
	Kick(ctx context.Context, accessToken, id, userID string) error
	Leave(ctx context.Context, accessToken, id string) error
	Return(ctx context.Context, accessToken, id string) error
	Send(ctx context.Context, accessToken, id string, d model.MessageDraft) (model.ChatMessage, error)
	History(ctx context.Context, accessToken, id string, at model.MessagesAt, limit int) (model.ChatMessagePage, error)
	// Message is a message with its whole text.
	Message(ctx context.Context, accessToken, id string) (model.ChatMessage, error)
	Edit(ctx context.Context, accessToken, id string, e model.MessageEdit) (model.ChatMessage, error)
	// Search finds messages in the chat or, for "", in all the caller's.
	Search(ctx context.Context, accessToken, chatID, query, cursor string, limit int) (model.FoundChatMessages, error)
	Delete(ctx context.Context, accessToken, id string) error
	MarkRead(ctx context.Context, accessToken, id, upTo string) error
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
	// JoinRequests are those who ask to join a closed club; Decide lets
	// one of them in or turns them down. Both are for those who run it.
	JoinRequests(ctx context.Context, accessToken, clubID, cursor string, limit int) (model.JoinRequestPage, error)
	Decide(ctx context.Context, accessToken, clubID, userID string, accept bool) error
	Describe(ctx context.Context, accessToken, clubID, userID string, change model.SeatChange) error
	SetAdmin(ctx context.Context, accessToken, clubID, userID string, admin bool) error
	Transfer(ctx context.Context, accessToken, clubID, userID string) error
	OfUser(ctx context.Context, userID, cursor string, limit int, filter model.ClubFilter) (model.UserClubPage, error)
	Mine(ctx context.Context, accessToken, cursor string, limit int, filter model.ClubFilter) (model.ClubPage, error)
	Search(ctx context.Context, query, cursor string, limit int) (model.ClubSearchPage, error)
	SetAvatar(ctx context.Context, accessToken, clubID, avatarID string) (model.ClubView, error)
	SetBackdrop(ctx context.Context, accessToken, clubID, side, photoID string) (model.ClubView, error)
	Pin(ctx context.Context, accessToken, clubID string, pinned bool) error
	Pinned(ctx context.Context, accessToken string) ([]model.Club, error)
	// MyRoles tells what the caller is in those of up to 100 clubs they
	// are in, by the ID of the club.
	MyRoles(ctx context.Context, accessToken string, clubIDs []string) (map[string]string, error)
	// HiddenManagers are those who run each of up to 100 clubs and whom
	// the caller (accessToken may be empty) may not know to, by the ID of
	// the club; clubs that hide nobody are left out. Not to be shown.
	HiddenManagers(ctx context.Context, accessToken string, clubIDs []string) (map[string][]string, error)
	CanPostOnWall(ctx context.Context, accessToken, clubID string, fromClub bool) (model.WallAccess, error)
}

// Gifts are the gifts people send each other ("Подарки"), which the user
// service keeps.
type Gifts interface {
	// Catalog is what there is to send.
	Catalog(ctx context.Context) ([]model.GiftCollection, error)
	Send(ctx context.Context, accessToken, userID string, d model.GiftDraft) (model.Gift, error)
	// List pages the gifts sent to userID as the caller, anonymous without
	// accessToken, sees them, newest first.
	List(ctx context.Context, accessToken, userID, cursor string, limit int) (model.GiftPage, error)
	Delete(ctx context.Context, accessToken, giftID string) error
}

// Votes are openvk's "голоса", which the user service keeps: vouchers give
// them, gifts cost them and people hand them to each other.
type Votes interface {
	// Transfer hands votes of the caller to userID and returns what the
	// caller has left.
	Transfer(ctx context.Context, accessToken, userID string, votes int64) (int64, error)
	// Redeem gives the caller what the voucher carries.
	Redeem(ctx context.Context, accessToken, token string) (model.Redeemed, error)
	// CreateVoucher, ListVouchers and DeleteVoucher are the
	// administrators'.
	CreateVoucher(ctx context.Context, accessToken string, d model.VoucherDraft) (model.Voucher, error)
	ListVouchers(ctx context.Context, accessToken, cursor string, limit int) (model.VoucherPage, error)
	DeleteVoucher(ctx context.Context, accessToken, voucherID string) error
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
	Chats         Chats
	Topics        Topics
	Albums        Albums
	Notes         Notes
	Faves         Faves
	Gifts         Gifts
	Votes         Votes
}

type api struct {
	auth              Auth
	users             Users
	posts             Posts
	media             Media
	notifications     Notifications
	messages          Messages
	chats             Chats
	clubs             Clubs
	topics            Topics
	albums            Albums
	notes             Notes
	faves             Faves
	gifts             Gifts
	votes             Votes
	logger            *slog.Logger
	trustForwardedFor bool
	webDir            string
	eventOrigins      []string
	eventStreams      prometheus.Gauge
	// statistics are the numbers of "о сайте", kept for a while.
	statistics *statistics
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
	mux.HandleFunc("PUT /api/v1/me/nickname", a.updateNickname)
	mux.HandleFunc("PUT /api/v1/me/interests", a.updateInterests)
	mux.HandleFunc("PUT /api/v1/me/fields", a.updateExtraFields)
	mux.HandleFunc("PUT /api/v1/me/privacy", a.updatePrivacy)
	mux.HandleFunc("PUT /api/v1/me/interface", a.updateInterface)
	mux.HandleFunc("GET /api/v1/me/sessions", a.listSessions)
	mux.HandleFunc("DELETE /api/v1/me/sessions", a.endOtherSessions)
	mux.HandleFunc("DELETE /api/v1/me/sessions/{id}", a.endSession)
	mux.HandleFunc("POST /api/v1/me/deactivate", a.deactivate)
	mux.HandleFunc("POST /api/v1/me/reactivate", a.reactivate)

	mux.HandleFunc("GET /api/v1/users/{nickname}", a.getProfile)

	mux.HandleFunc("GET /api/v1/me/blacklist", a.listBlacklist)
	mux.HandleFunc("PUT /api/v1/me/blacklist/{nickname}", a.block)
	mux.HandleFunc("DELETE /api/v1/me/blacklist/{nickname}", a.unblock)

	mux.HandleFunc("GET /api/v1/users/{nickname}/posts", a.listPosts)
	mux.HandleFunc("POST /api/v1/users/{nickname}/posts", a.createWallPost)
	mux.HandleFunc("PUT /api/v1/users/{nickname}/posts/archive", a.archiveWall)
	mux.HandleFunc("DELETE /api/v1/users/{nickname}/posts/archive", a.unarchiveWall)
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
	mux.HandleFunc("GET /api/v1/posts/{id}/reposts", a.listReposts)
	mux.HandleFunc("PUT /api/v1/posts/{id}/like", a.likePost)
	mux.HandleFunc("DELETE /api/v1/posts/{id}/like", a.unlikePost)
	mux.HandleFunc("GET /api/v1/posts/{id}/likes", a.listPostLikers)
	mux.HandleFunc("PUT /api/v1/posts/{id}/poll/vote", a.votePoll)
	mux.HandleFunc("DELETE /api/v1/posts/{id}/poll/vote", a.retractPollVote)
	mux.HandleFunc("GET /api/v1/posts/{id}/poll/voters", a.listPollVoters)

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
	mux.HandleFunc("GET /api/v1/me/feed/ignored", a.listIgnored)
	mux.HandleFunc("DELETE /api/v1/me/feed/ignored", a.unignoreMany)
	mux.HandleFunc("PUT /api/v1/me/feed/ignored/users/{nickname}", a.ignoreUser)
	mux.HandleFunc("DELETE /api/v1/me/feed/ignored/users/{nickname}", a.unignoreUser)
	mux.HandleFunc("PUT /api/v1/me/feed/ignored/clubs/{ref}", a.ignoreClub)
	mux.HandleFunc("DELETE /api/v1/me/feed/ignored/clubs/{ref}", a.unignoreClub)

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
	mux.HandleFunc("DELETE /api/v1/messages", a.deleteMessages)

	mux.HandleFunc("POST /api/v1/chats", a.createChat)
	mux.HandleFunc("GET /api/v1/chats/{id}", a.getChat)
	mux.HandleFunc("PATCH /api/v1/chats/{id}", a.renameChat)
	mux.HandleFunc("POST /api/v1/chats/{id}/members", a.addChatMembers)
	mux.HandleFunc("DELETE /api/v1/chats/{id}/members/{nickname}", a.removeChatMember)
	mux.HandleFunc("PUT /api/v1/chats/{id}/membership", a.returnToChat)
	mux.HandleFunc("DELETE /api/v1/chats/{id}/membership", a.leaveChat)
	mux.HandleFunc("GET /api/v1/chats/{id}/messages", a.listChatMessages)
	mux.HandleFunc("POST /api/v1/chats/{id}/messages", a.sendChatMessage)
	mux.HandleFunc("POST /api/v1/chats/{id}/read", a.markChatRead)
	mux.HandleFunc("GET /api/v1/chat-messages/{id}", a.getChatMessage)
	mux.HandleFunc("PATCH /api/v1/chat-messages/{id}", a.editChatMessage)
	mux.HandleFunc("DELETE /api/v1/chat-messages/{id}", a.deleteChatMessage)

	mux.HandleFunc("POST /api/v1/clubs", a.createClub)
	mux.HandleFunc("POST /api/v1/events", a.createEvent)
	mux.HandleFunc("GET /api/v1/about", a.about)
	mux.HandleFunc("GET /api/v1/nicknames", a.knownNicknames)
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
	mux.HandleFunc("GET /api/v1/clubs/{ref}/requests", a.listJoinRequests)
	mux.HandleFunc("PUT /api/v1/clubs/{ref}/requests/{nickname}", a.acceptJoinRequest)
	mux.HandleFunc("DELETE /api/v1/clubs/{ref}/requests/{nickname}", a.rejectJoinRequest)
	mux.HandleFunc("PUT /api/v1/clubs/{ref}/owner/{nickname}", a.transferClub)
	mux.HandleFunc("GET /api/v1/clubs/{ref}/posts", a.listClubPosts)
	mux.HandleFunc("POST /api/v1/clubs/{ref}/posts", a.createClubPost)
	mux.HandleFunc("PUT /api/v1/clubs/{ref}/posts/archive", a.archiveClubWall)
	mux.HandleFunc("DELETE /api/v1/clubs/{ref}/posts/archive", a.unarchiveClubWall)
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
	mux.HandleFunc("GET /api/v1/topic-messages/{id}/likes", a.listTopicMessageLikers)
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
	mux.HandleFunc("PUT /api/v1/photo-comments/{id}/like", a.likePhotoComment)
	mux.HandleFunc("DELETE /api/v1/photo-comments/{id}/like", a.unlikePhotoComment)
	mux.HandleFunc("GET /api/v1/photo-comments/{id}/likes", a.listPhotoCommentLikers)
	mux.HandleFunc("POST /api/v1/album-photos/{id}/reposts", a.shareAlbumPhoto)
	mux.HandleFunc("GET /api/v1/me/faves/posts", a.listFavePosts)
	mux.HandleFunc("GET /api/v1/me/faves/comments", a.listFaveComments)
	mux.HandleFunc("GET /api/v1/me/faves/photos", a.listFavePhotos)
	mux.HandleFunc("GET /api/v1/me/posts", a.listOwnPosts)
	mux.HandleFunc("GET /api/v1/gifts", a.giftCatalog)
	mux.HandleFunc("GET /api/v1/users/{nickname}/gifts", a.listGifts)
	mux.HandleFunc("POST /api/v1/users/{nickname}/gifts", a.sendGift)
	mux.HandleFunc("DELETE /api/v1/gifts/{id}", a.deleteGift)
	mux.HandleFunc("POST /api/v1/me/vouchers", a.redeemVoucher)
	mux.HandleFunc("POST /api/v1/users/{nickname}/votes", a.transferVotes)
	mux.HandleFunc("GET /api/v1/vouchers", a.listVouchers)
	mux.HandleFunc("POST /api/v1/vouchers", a.createVoucher)
	mux.HandleFunc("DELETE /api/v1/vouchers/{id}", a.deleteVoucher)

	mux.HandleFunc("GET /api/v1/users/{nickname}/notes", a.listUserNotes)
	mux.HandleFunc("POST /api/v1/me/notes", a.createNote)
	mux.HandleFunc("GET /api/v1/notes/{id}", a.getNote)
	mux.HandleFunc("PATCH /api/v1/notes/{id}", a.editNote)
	mux.HandleFunc("DELETE /api/v1/notes/{id}", a.deleteNote)
	mux.HandleFunc("GET /api/v1/notes/{id}/comments", a.listNoteComments)
	mux.HandleFunc("POST /api/v1/notes/{id}/comments", a.createNoteComment)
	mux.HandleFunc("DELETE /api/v1/note-comments/{id}", a.deleteNoteComment)
	mux.HandleFunc("GET /api/v1/note-comments/{id}/text", a.noteCommentText)

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
