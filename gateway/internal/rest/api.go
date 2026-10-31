// Package rest is the public HTTP/JSON API of freenet.
package rest

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/pkg/pubsub"
)

type Auth interface {
	RequestCode(ctx context.Context, req model.CodeRequest, clientIP string) error
	NewCaptcha(ctx context.Context, clientIP string) (model.Captcha, error)
	// VerifyCode and RefreshToken say where the user is, for the list of
	// their sessions.
	VerifyCode(ctx context.Context, email, code string, client model.Client) (model.Tokens, error)
	RefreshToken(ctx context.Context, refreshToken string, client model.Client) (model.Tokens, error)
	Logout(ctx context.Context, refreshToken string) error
	ListSessions(ctx context.Context, accessToken string) ([]model.Session, error)
	EndSession(ctx context.Context, accessToken, sessionID string) error
	// EndOtherSessions returns how many sessions ended.
	EndOtherSessions(ctx context.Context, accessToken string) (int, error)
	TwoFactorAuth
}

// TwoFactorAuth is the second step of signing in with an authenticator
// app: VerifyCode answers a page that has it on with a ticket for it.
type TwoFactorAuth interface {
	VerifyTwoFactor(ctx context.Context, ticket, code string, client model.Client) (model.Tokens, error)
	TwoFactor(ctx context.Context, accessToken string) (model.TwoFactor, error)
	SetUpTwoFactor(ctx context.Context, accessToken, account string) (model.TwoFactorSetup, error)
	// EnableTwoFactor and RenewRecoveryCodes return the recovery codes.
	EnableTwoFactor(ctx context.Context, accessToken, code string) ([]string, error)
	DisableTwoFactor(ctx context.Context, accessToken, code string) error
	RenewRecoveryCodes(ctx context.Context, accessToken, code string) ([]string, error)
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
	CanMessage(ctx context.Context, accessToken, userID string, wroteFirst bool) (model.MessageTerms, error)
	// PayMessage pays for a message of the caller to userID at price; the
	// payment has no ID for a free one. RefundMessage gives it back.
	PayMessage(ctx context.Context, accessToken, userID string, wroteFirst bool, price int64) (model.MessagePayment, error)
	RefundMessage(ctx context.Context, accessToken, paymentID string) error
	// BoughtPosts are those of postIDs the caller has paid to see; BuyPost
	// pays for one and returns the votes left.
	BoughtPosts(ctx context.Context, accessToken string, postIDs []string) ([]string, error)
	BuyPost(ctx context.Context, accessToken, postID, authorID string, price int64) (int64, error)
	// ResolveMentions are the IDs of the people the caller names by these
	// nicknames who hear of it: who exist and whom the caller may name.
	ResolveMentions(ctx context.Context, accessToken string, nicknames []string) ([]string, error)
	// ResolveInvites are the people the caller names by nickname to add
	// to a chat, and whether each lets them; nobody for an unknown one.
	ResolveInvites(ctx context.Context, accessToken string, nicknames []string) ([]model.Invitee, error)
	// KnownNicknames are those of nicknames that are someone's, as their
	// owners write them, and which of them are verified.
	KnownNicknames(ctx context.Context, nicknames []string) (model.KnownNicknames, error)
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
	// AcceptInvite follows the user whose invitation link the caller
	// signed up by, once and while the caller's page is new.
	AcceptInvite(ctx context.Context, accessToken, userID string) error
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
	// Statistics counts the people of the site, those on it now and
	// those with the checkmark.
	Statistics(ctx context.Context) (model.People, error)
	// SetVerified gives userID the checkmark of a confirmed page or takes
	// it; only an administrator of the site may.
	SetVerified(ctx context.Context, accessToken, userID string, verified bool) error
	// SetAlert puts on userID the label with the words of alert or, with
	// "", takes it; only an administrator of the site may.
	SetAlert(ctx context.Context, accessToken, userID, alert string) error
	// Ban blocks userID until a time, the zero time for good; Unban lifts
	// the block. Only an administrator of the site may.
	Ban(ctx context.Context, accessToken, userID, reason string, until time.Time) error
	Unban(ctx context.Context, accessToken, userID string) error
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
	UploadChatAvatar(ctx context.Context, accessToken, chatID string, image []byte) (string, error)
	DeleteChatAvatars(ctx context.Context, accessToken, chatID, keep string) error
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

	// The bytes of documents: UploadDocument keeps a file of the caller,
	// Document reads one of ownerID, DeleteDocuments deletes those of a
	// document the post service refused.
	UploadDocument(ctx context.Context, accessToken string, data []byte) (string, error)
	Document(ctx context.Context, ownerID, documentID string) ([]byte, error)
	DeleteDocuments(ctx context.Context, ownerID string, documentIDs []string) error
}

// Documents are openvk's "Мои Документы", which the post service keeps:
// files of people that posts and messages attach.
type Documents interface {
	Add(ctx context.Context, accessToken, id, fileName string, size int64) (model.Document, error)
	List(ctx context.Context, accessToken, cursor string, limit int) (model.DocumentPage, error)
	// GetMany tells the documents of up to 50 ids that exist, anyone's.
	GetMany(ctx context.Context, ids []string) ([]model.Document, error)
	Rename(ctx context.Context, accessToken, id, name string) (model.Document, error)
	Delete(ctx context.Context, accessToken, id string) error
}

// Stories are what people show for a day, VK's "истории"; all need an
// access token.
type Stories interface {
	Tell(ctx context.Context, accessToken, photoID, text string) (model.Story, error)
	// Feed is the stories above the caller's news, grouped by author: the
	// caller first, then those with a story not viewed yet.
	Feed(ctx context.Context, accessToken string, hidden, ignored []string) ([]model.Story, error)
	User(ctx context.Context, accessToken, userID string, hidden []string) ([]model.Story, error)
	Get(ctx context.Context, accessToken, id string) (model.Story, error)
	View(ctx context.Context, accessToken, id string, hidden bool) error
	Like(ctx context.Context, accessToken, id string) error
	Unlike(ctx context.Context, accessToken, id string) error
	Comment(ctx context.Context, accessToken, id, text string) (model.StoryComment, error)
	// Comments pages the comments of a story, the oldest first.
	Comments(ctx context.Context, accessToken, id, cursor string, limit int) (model.StoryCommentPage, error)
	DeleteComment(ctx context.Context, accessToken, id string) error
	// Viewers are who viewed a story of the caller, the latest first.
	Viewers(ctx context.Context, accessToken, id, cursor string, limit int) (model.LikerPage, error)
	Delete(ctx context.Context, accessToken, id string) error
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
	// Search finds the posts of every wall by a tag or a part of the
	// text, without those of the people hidden.
	Search(ctx context.Context, accessToken, query string, hidden []string, cursor string, limit int) (model.PostPage, error)

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

	Postpone(ctx context.Context, accessToken string, in model.NewPostponed) (model.Postponed, error)
	// PostponedPosts are the posts that wait for a wall, the soonest
	// first; PostponedPost is one of them, whole.
	PostponedPosts(ctx context.Context, accessToken, wallID string) ([]model.Postponed, error)
	PostponedPost(ctx context.Context, accessToken, id string) (model.Postponed, error)
	EditPostponed(ctx context.Context, accessToken, id string, edit model.PostponedEdit) (model.Postponed, error)
	PublishPostponed(ctx context.Context, accessToken, id string) (model.Post, error)
	DeletePostponed(ctx context.Context, accessToken, id string) error
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

type Notifications interface {
	List(ctx context.Context, accessToken string, unread bool, cursor string, limit int) (model.NotificationPage, error)
	CountUnread(ctx context.Context, accessToken string) (int, error)
	MarkRead(ctx context.Context, accessToken, id string) error
	Subscribe(ctx context.Context, accessToken string) (model.NotificationChanges, error)
	// PushKey is "" when pushes are off.
	PushKey(ctx context.Context) (string, error)
	SubscribePush(ctx context.Context, accessToken string, s model.PushSubscription) error
	UnsubscribePush(ctx context.Context, accessToken, endpoint string) error
}

type Messages interface {
	Send(ctx context.Context, accessToken, recipientID string, d model.MessageDraft) (model.Message, error)
	Dialogs(ctx context.Context, accessToken, cursor string, limit int) (model.DialogPage, error)
	CountUnread(ctx context.Context, accessToken string) (int, error)
	History(ctx context.Context, accessToken, peerID string, at model.MessagesAt, limit int) (model.MessagePage, error)
	MarkRead(ctx context.Context, accessToken, peerID, upTo string) error
	// CallSignal hands a sealed signal of a call to the peer.
	CallSignal(ctx context.Context, accessToken, peerID, sealed string) error
	// CallServers are the STUN and TURN servers of a call.
	CallServers(ctx context.Context, accessToken string) (model.CallServers, error)
	// Typing tells the peer the caller is writing to them.
	Typing(ctx context.Context, accessToken, peerID string) error
	Delete(ctx context.Context, accessToken, id string) error
	// Hide deletes a message for the caller alone; Edit changes what
	// their own says.
	Hide(ctx context.Context, accessToken, id string) error
	// DeleteMany deletes several messages at once, all or none: for both
	// sides, as Delete, or for the caller alone, as Hide.
	DeleteMany(ctx context.Context, accessToken string, ids []string, forEveryone bool) error
	Edit(ctx context.Context, accessToken, id string, e model.MessageEdit) (model.Message, error)
	// React puts the caller's reaction on a message of their dialog, or
	// takes theirs away for "".
	React(ctx context.Context, accessToken, id, reaction string) error
	HasMessagesFrom(ctx context.Context, accessToken, peerID string) (bool, error)
	Subscribe(ctx context.Context, accessToken string) (model.MessageEvents, error)
	// SetKey makes publicKey the one messages to and from the caller are
	// sealed with; Key is userID's, nil for a user who has none yet.
	SetKey(ctx context.Context, accessToken string, publicKey []byte) error
	Key(ctx context.Context, accessToken, userID string) ([]byte, error)
}

// Chats are the chats of several people ("беседы"); all take the access
// token of someone in the chat. History reads older than cursor or, with
// after, newer than it.
type Chats interface {
	Create(ctx context.Context, accessToken, title string, memberIDs []string) (model.Chat, error)
	Get(ctx context.Context, accessToken, id string) (model.ChatView, error)
	List(ctx context.Context, accessToken, cursor string, limit int) (model.ChatPage, error)
	Rename(ctx context.Context, accessToken, id, title string) error
	Pin(ctx context.Context, accessToken, id, messageID string) error
	Unpin(ctx context.Context, accessToken, id string) error
	// SetPhoto shows a photo uploaded under the chat's ID; "" removes it.
	SetPhoto(ctx context.Context, accessToken, id, photoID string) error
	Invite(ctx context.Context, accessToken, id string, userIDs []string) error
	Kick(ctx context.Context, accessToken, id, userID string) error
	Leave(ctx context.Context, accessToken, id string) error
	Return(ctx context.Context, accessToken, id string) error
	// DeleteChat deletes the chat for everyone in it with its messages:
	// the creator's call.
	DeleteChat(ctx context.Context, accessToken, id string) error
	Send(ctx context.Context, accessToken, id string, d model.MessageDraft) (model.ChatMessage, error)
	History(ctx context.Context, accessToken, id string, at model.MessagesAt, limit int) (model.ChatMessagePage, error)
	// Message is a message with its whole text.
	Message(ctx context.Context, accessToken, id string) (model.ChatMessage, error)
	Edit(ctx context.Context, accessToken, id string, e model.MessageEdit) (model.ChatMessage, error)
	React(ctx context.Context, accessToken, id, reaction string) error
	// Search finds messages in the chat or, for "", in all the caller's.
	Search(ctx context.Context, accessToken, chatID, query, cursor string, limit int) (model.FoundChatMessages, error)
	Delete(ctx context.Context, accessToken, id string) error
	MarkRead(ctx context.Context, accessToken, id, upTo string) error
	// Typing tells the others in the chat the caller is writing there.
	Typing(ctx context.Context, accessToken, id string) error
}

// Clubs: Get and Members take an optional access token ("" for anonymous
// viewers); a club ref is its ID or its address.
type Clubs interface {
	Create(ctx context.Context, accessToken, name, description string) (model.ClubView, error)
	Get(ctx context.Context, accessToken, ref string) (model.ClubView, error)
	GetMany(ctx context.Context, ids []string) ([]model.Club, error)
	// Count counts the groups.
	Count(ctx context.Context) (int, error)
	Update(ctx context.Context, accessToken, clubID string, changes model.ClubChanges) (model.ClubView, error)
	Join(ctx context.Context, accessToken, clubID string) (model.ClubView, error)
	Leave(ctx context.Context, accessToken, clubID string) (model.ClubView, error)
	Members(ctx context.Context, accessToken, clubID, cursor string, limit int, filter model.ClubMemberFilter) (model.ClubMemberPage, error)
	// JoinRequests are those who ask to join a closed club; Decide lets
	// one of them in or turns them down. Both are for those who run it.
	JoinRequests(ctx context.Context, accessToken, clubID, cursor string, limit int) (model.JoinRequestPage, error)
	Decide(ctx context.Context, accessToken, clubID, userID string, accept bool) error
	Describe(ctx context.Context, accessToken, clubID, userID string, change model.SeatChange) error
	SetAdmin(ctx context.Context, accessToken, clubID, userID string, admin bool) error
	Transfer(ctx context.Context, accessToken, clubID, userID string) error
	// Delete deletes the club with its wall, topics and albums: the
	// owner's call.
	Delete(ctx context.Context, accessToken, clubID string) error
	OfUser(ctx context.Context, userID, cursor string, limit int, filter model.ClubFilter) (model.UserClubPage, error)
	Mine(ctx context.Context, accessToken, cursor string, limit int, filter model.ClubFilter) (model.ClubPage, error)
	Search(ctx context.Context, q model.ClubQuery, cursor string, limit int) (model.ClubSearchPage, error)
	// SetVerified gives the club the checkmark or takes it; whether the
	// caller is an administrator of the site is the gateway's to tell.
	SetVerified(ctx context.Context, clubID string, verified bool) error
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
	// Catalog is what there is to send, and what collectibles may be.
	Catalog(ctx context.Context) (model.GiftCatalog, error)
	Send(ctx context.Context, accessToken, userID string, d model.GiftDraft) (model.Gift, error)
	// List pages the gifts sent to userID as the caller, anonymous without
	// accessToken, sees them, newest first.
	List(ctx context.Context, accessToken, userID, cursor string, limit int) (model.GiftPage, error)
	Delete(ctx context.Context, accessToken, giftID string) error
	// SetPrice is for the administrators of the site: what a gift costs
	// from now on.
	SetPrice(ctx context.Context, accessToken, gift string, price int64) error
	GiftMarket
}

// GiftMarket is the collectible gifts, as Telegram's: upgraded, given away,
// sold and auctioned. Upgrade, Transfer, SetSale, StartAuction and
// CancelAuction are for the owner; Buy and Bid say the buyer's votes
// after; MintAuction and SetSupply are for the administrators of the site.
type GiftMarket interface {
	Upgrade(ctx context.Context, accessToken, giftID string) (model.Gift, error)
	Transfer(ctx context.Context, accessToken, giftID, userID string) error
	SetSale(ctx context.Context, accessToken, giftID string, price int64) (model.Gift, error)
	Buy(ctx context.Context, accessToken, giftID string, price int64) (int64, error)
	Collectible(ctx context.Context, accessToken, giftID string) (model.Collectible, error)
	Market(ctx context.Context, accessToken string, f model.MarketFilter, cursor string, limit int) (model.CollectiblePage, error)
	StartAuction(ctx context.Context, accessToken, giftID string, start int64, hours int) error
	MintAuction(ctx context.Context, accessToken string, e model.GiftEdition) (string, error)
	CancelAuction(ctx context.Context, accessToken, giftID string) error
	Bid(ctx context.Context, accessToken, giftID string, amount int64) (int64, error)
	Auctions(ctx context.Context, accessToken, cursor string, limit int) (model.CollectiblePage, error)
	SetSupply(ctx context.Context, accessToken, gift string, supply int64) error
}

// Stickers are openvk's "Стикеры", which the user service keeps: packs
// people take, free or for votes, and send in messages, and those they
// make of their photos in the "Студия стикеров".
type Stickers interface {
	// Catalog is every pack, with which of them the caller, anonymous
	// without accessToken, has taken and keeps among their stickers.
	Catalog(ctx context.Context, accessToken string) ([]model.StickerPack, error)
	Add(ctx context.Context, accessToken, packID string) error
	Remove(ctx context.Context, accessToken, packID string) error
	// Mine are the packs the caller made, newest first.
	Mine(ctx context.Context, accessToken string) ([]model.StickerPack, error)
	// Create makes a pack of the caller's photos in d.Added.
	Create(ctx context.Context, accessToken string, d model.StickerPackDraft) (model.StickerPack, error)
	// Edit returns the pack and the photos it no longer has.
	Edit(ctx context.Context, accessToken, packID string, d model.StickerPackDraft) (model.StickerPack, []string, error)
	// Delete returns the photos the pack had.
	Delete(ctx context.Context, accessToken, packID string) ([]string, error)
	// Author is whose photo the sticker is; "" for one of the site or none.
	Author(ctx context.Context, sticker string) (string, error)
}

// Votes are openvk's "голоса", which the user service keeps: vouchers give
// them, gifts cost them and people hand them to each other.
type Votes interface {
	// Transfer hands votes of the caller to userID and returns what the
	// caller has left and the fee paid on top.
	// The note goes with the transfer to both statements.
	Transfer(ctx context.Context, accessToken, userID string, votes int64, note string) (model.Transferred, error)
	// RaiseRating is "Повысить рейтинг": percent of rating for votes.
	RaiseRating(ctx context.Context, accessToken string, percent int) (model.RatingRaised, error)
	// Statement is the caller's statement of votes: flow "in", "out" or
	// empty for both.
	Statement(ctx context.Context, accessToken, cursor, flow string, limit int) (model.Statement, error)
	// Redeem gives the caller what the voucher carries.
	Redeem(ctx context.Context, accessToken, token string) (model.Redeemed, error)
	// CreateVoucher, ListVouchers and DeleteVoucher are the
	// administrators'.
	CreateVoucher(ctx context.Context, accessToken string, d model.VoucherDraft) (model.Voucher, error)
	ListVouchers(ctx context.Context, accessToken, cursor string, limit int) (model.VoucherPage, error)
	DeleteVoucher(ctx context.Context, accessToken, voucherID string) error
	// Rate is what a vote is worth, in kopecks; SetRate is the
	// administrators'.
	Rate(ctx context.Context) (int64, error)
	SetRate(ctx context.Context, accessToken string, kopecks int64) error
	// Treasury is public; Bonus and ClaimBonus are the caller's daily
	// bonus out of it.
	Treasury(ctx context.Context) (model.Treasury, error)
	// SetEconomy sets the bonus and the fees; the administrators'.
	SetEconomy(ctx context.Context, accessToken string, e model.Economy) error
	Bonus(ctx context.Context, accessToken string) (model.Bonus, error)
	ClaimBonus(ctx context.Context, accessToken string) (model.Redeemed, error)
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
	Faves         Faves
	Gifts         Gifts
	Stickers      Stickers
	Votes         Votes
	Ads           Ads
	Decor         Decor
	Prime         Prime
	Casino        Casino
	Mine          Mine
	Pets          Pets
	Firms         Firms
	Titles        Titles
	Bank          Bank
	Poker         Poker
	Holdem        Holdem
	Bets          Bets
	Documents     Documents
	Articles      Articles
	Stories       Stories
	Canvas        Canvas
	Dating        Dating
	// Previews reads the cards of the pages posts link to; nil makes
	// posts without cards.
	Previews Previews
}

// Previews reads what a web page says of itself for the card of a post;
// it never reaches inside the network the gateway is in.
type Previews interface {
	Read(ctx context.Context, url string) (model.LinkPreview, error)
}

type api struct {
	auth          Auth
	users         Users
	posts         Posts
	media         Media
	notifications Notifications
	messages      Messages
	chats         Chats
	clubs         Clubs
	topics        Topics
	albums        Albums
	faves         Faves
	gifts         Gifts
	stickers      Stickers
	votes         Votes
	ads           Ads
	decors        Decor
	primes        Prime
	casinos       Casino
	mines         Mine
	pets          Pets
	firmsBackend  Firms
	titles        Titles
	banks         Bank
	pokers        Poker
	holdems       Holdem
	bets          Bets
	documents     Documents
	articles      Articles
	stories       Stories
	canvas        Canvas
	// paints tells the open canvases of the cells just painted. A hub of
	// this process is enough while there is one gateway; a second would
	// show its pixels at the next reconnect.
	paints            *pubsub.Hub[struct{}, canvasPaintRequest]
	canvasStreams     atomic.Int64
	dating            Dating
	previews          Previews
	logger            *slog.Logger
	trustForwardedFor bool
	webDir            string
	eventOrigins      []string
	eventStreams      prometheus.Gauge
	// statistics are the numbers of "о сайте", kept for a while.
	statistics *statistics
	// batched serves the reads of a batch: the routes, counted by the
	// metrics as if each came by itself.
	batched http.Handler
	// stopping ends when the server stops, closing the event streams,
	// whose connections Shutdown leaves alone; streams counts them.
	stopping context.Context
	streams  sync.WaitGroup
}

func (a *api) routes() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET "+HealthPath, a.health)

	mux.HandleFunc("GET /api/v1/auth/captcha", a.newCaptcha)
	mux.HandleFunc("POST /api/v1/auth/code", a.requestCode)
	mux.HandleFunc("POST /api/v1/auth/verify", a.verifyCode)
	mux.HandleFunc("POST /api/v1/auth/refresh", a.refreshToken)
	mux.HandleFunc("POST /api/v1/auth/logout", a.logout)

	mux.HandleFunc("POST "+batchPath, a.batch)

	mux.HandleFunc("GET /api/v1/me", a.getMe)
	mux.HandleFunc("PUT /api/v1/me/profile", a.updateProfile)
	mux.HandleFunc("PUT /api/v1/me/contacts", a.updateContacts)
	mux.HandleFunc("PUT /api/v1/me/status", a.updateStatus)
	mux.HandleFunc("PUT /api/v1/me/nickname", a.updateNickname)
	mux.HandleFunc("PUT /api/v1/me/interests", a.updateInterests)
	mux.HandleFunc("PUT /api/v1/me/fields", a.updateExtraFields)
	mux.HandleFunc("PUT /api/v1/me/privacy", a.updatePrivacy)
	mux.HandleFunc("PUT /api/v1/me/interface", a.updateInterface)
	mux.HandleFunc("POST /api/v1/auth/two-factor", a.verifyTwoFactor)
	mux.HandleFunc("GET /api/v1/me/two-factor", a.getTwoFactor)
	mux.HandleFunc("POST /api/v1/me/two-factor/setup", a.setUpTwoFactor)
	mux.HandleFunc("POST /api/v1/me/two-factor/enable", a.enableTwoFactor)
	mux.HandleFunc("POST /api/v1/me/two-factor/disable", a.disableTwoFactor)
	mux.HandleFunc("POST /api/v1/me/two-factor/recovery", a.renewRecoveryCodes)
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
	mux.HandleFunc("GET /api/v1/posts", a.searchPosts)
	mux.HandleFunc("GET /api/v1/posts/{id}", a.getPost)
	mux.HandleFunc("DELETE /api/v1/posts/{id}", a.deletePost)
	mux.HandleFunc("PATCH /api/v1/posts/{id}", a.editPost)
	mux.HandleFunc("PUT /api/v1/posts/{id}/pin", a.pinPost)
	mux.HandleFunc("DELETE /api/v1/posts/{id}/pin", a.unpinPost)
	mux.HandleFunc("POST /api/v1/posts/{id}/purchase", a.buyPost)
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
	mux.HandleFunc("POST /api/v1/users/{nickname}/invitation", a.acceptInvite)
	mux.HandleFunc("GET /api/v1/users/{nickname}/followers", a.listFollowers)
	mux.HandleFunc("GET /api/v1/users/{nickname}/following", a.listFollowing)
	mux.HandleFunc("GET /api/v1/users/{nickname}/clubs", a.listUserClubs)

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
	mux.HandleFunc("GET /api/v1/push/key", a.getPushKey)
	mux.HandleFunc("PUT /api/v1/me/push", a.subscribePush)
	mux.HandleFunc("DELETE /api/v1/me/push", a.unsubscribePush)

	mux.HandleFunc("GET "+eventsPath, a.events)

	mux.HandleFunc("GET /api/v1/dialogs", a.listDialogs)
	mux.HandleFunc("GET /api/v1/dialogs/unread-count", a.countUnreadDialogs)
	mux.HandleFunc("GET /api/v1/dialogs/{nickname}/messages", a.listMessages)
	mux.HandleFunc("POST /api/v1/dialogs/{nickname}/messages", a.sendMessage)
	mux.HandleFunc("POST /api/v1/dialogs/{nickname}/read", a.markDialogRead)
	mux.HandleFunc("POST /api/v1/dialogs/{nickname}/typing", a.dialogTyping)
	mux.HandleFunc("POST /api/v1/dialogs/{nickname}/call", a.dialogCall)
	mux.HandleFunc("GET /api/v1/call-servers", a.callServers)
	mux.HandleFunc("GET /api/v1/messages/search", a.searchMessages)
	mux.HandleFunc("PATCH /api/v1/messages/{id}", a.editMessage)
	mux.HandleFunc("DELETE /api/v1/messages/{id}", a.deleteMessage)
	mux.HandleFunc("PUT /api/v1/messages/{id}/reaction", a.reactToMessage)
	mux.HandleFunc("DELETE /api/v1/messages/{id}/reaction", a.reactToMessage)
	mux.HandleFunc("DELETE /api/v1/messages", a.deleteMessages)
	mux.HandleFunc("GET /api/v1/me/message-key", a.getMyMessageKey)
	mux.HandleFunc("PUT /api/v1/me/message-key", a.setMessageKey)
	mux.HandleFunc("GET /api/v1/users/{nickname}/message-key", a.getMessageKey)

	mux.HandleFunc("POST /api/v1/chats", a.createChat)
	mux.HandleFunc("GET /api/v1/chats/{id}", a.getChat)
	mux.HandleFunc("PATCH /api/v1/chats/{id}", a.renameChat)
	mux.HandleFunc("DELETE /api/v1/chats/{id}", a.deleteChat)
	mux.HandleFunc("PUT /api/v1/chats/{id}/pin", a.pinChatMessage)
	mux.HandleFunc("DELETE /api/v1/chats/{id}/pin", a.unpinChatMessage)
	mux.HandleFunc("PUT /api/v1/chats/{id}/photo", a.uploadChatPhoto)
	mux.HandleFunc("DELETE /api/v1/chats/{id}/photo", a.deleteChatPhoto)
	mux.HandleFunc("POST /api/v1/chats/{id}/members", a.addChatMembers)
	mux.HandleFunc("DELETE /api/v1/chats/{id}/members/{nickname}", a.removeChatMember)
	mux.HandleFunc("PUT /api/v1/chats/{id}/membership", a.returnToChat)
	mux.HandleFunc("DELETE /api/v1/chats/{id}/membership", a.leaveChat)
	mux.HandleFunc("GET /api/v1/chats/{id}/messages", a.listChatMessages)
	mux.HandleFunc("POST /api/v1/chats/{id}/messages", a.sendChatMessage)
	mux.HandleFunc("POST /api/v1/chats/{id}/read", a.markChatRead)
	mux.HandleFunc("POST /api/v1/chats/{id}/typing", a.chatTyping)
	mux.HandleFunc("GET /api/v1/chat-messages/{id}", a.getChatMessage)
	mux.HandleFunc("PATCH /api/v1/chat-messages/{id}", a.editChatMessage)
	mux.HandleFunc("DELETE /api/v1/chat-messages/{id}", a.deleteChatMessage)
	mux.HandleFunc("PUT /api/v1/chat-messages/{id}/reaction", a.reactToChatMessage)
	mux.HandleFunc("DELETE /api/v1/chat-messages/{id}/reaction", a.reactToChatMessage)

	mux.HandleFunc("POST /api/v1/clubs", a.createClub)
	mux.HandleFunc("GET /api/v1/about", a.about)
	mux.HandleFunc("GET /api/v1/nicknames", a.knownNicknames)
	mux.HandleFunc("GET /api/v1/me/clubs", a.listMyClubs)
	mux.HandleFunc("GET /api/v1/me/clubs/pinned", a.listPinnedClubs)
	mux.HandleFunc("GET /api/v1/clubs", a.searchClubs)
	mux.HandleFunc("GET /api/v1/clubs/{ref}", a.getClub)
	mux.HandleFunc("PUT /api/v1/clubs/{ref}", a.updateClub)
	mux.HandleFunc("DELETE /api/v1/clubs/{ref}", a.deleteClub)
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
	mux.HandleFunc("GET /api/v1/me/postponed", a.listMyPostponed)
	mux.HandleFunc("POST /api/v1/me/postponed", a.postponeMyPost)
	mux.HandleFunc("GET /api/v1/clubs/{ref}/postponed", a.listClubPostponed)
	mux.HandleFunc("POST /api/v1/clubs/{ref}/postponed", a.postponeClubPost)
	mux.HandleFunc("GET /api/v1/postponed/{id}", a.getPostponed)
	mux.HandleFunc("PATCH /api/v1/postponed/{id}", a.editPostponed)
	mux.HandleFunc("POST /api/v1/postponed/{id}/publish", a.publishPostponed)
	mux.HandleFunc("DELETE /api/v1/postponed/{id}", a.deletePostponed)
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
	mux.HandleFunc("PUT /api/v1/gifts/{gift}/price", a.setGiftPrice)
	mux.HandleFunc("PUT /api/v1/gifts/{gift}/supply", a.setGiftSupply)
	mux.HandleFunc("GET /api/v1/gifts/{id}", a.getCollectible)
	mux.HandleFunc("POST /api/v1/gifts/{id}/upgrade", a.upgradeGift)
	mux.HandleFunc("POST /api/v1/gifts/{id}/transfer", a.transferGift)
	mux.HandleFunc("PUT /api/v1/gifts/{id}/sale", a.putGiftOnSale)
	mux.HandleFunc("DELETE /api/v1/gifts/{id}/sale", a.takeGiftOffSale)
	mux.HandleFunc("POST /api/v1/gifts/{id}/purchase", a.buyGift)
	mux.HandleFunc("POST /api/v1/gifts/{id}/auction", a.startGiftAuction)
	mux.HandleFunc("DELETE /api/v1/gifts/{id}/auction", a.cancelGiftAuction)
	mux.HandleFunc("POST /api/v1/gifts/{id}/bids", a.bidGift)
	mux.HandleFunc("GET /api/v1/market", a.giftMarket)
	mux.HandleFunc("GET /api/v1/auctions", a.giftAuctions)
	mux.HandleFunc("POST /api/v1/auctions", a.mintGiftAuction)
	mux.HandleFunc("GET /api/v1/me/documents", a.listDocuments)
	mux.HandleFunc("POST /api/v1/me/documents", a.uploadDocument)
	mux.HandleFunc("PATCH /api/v1/documents/{id}", a.renameDocument)
	mux.HandleFunc("DELETE /api/v1/documents/{id}", a.deleteDocument)

	mux.HandleFunc("POST /api/v1/articles", a.createArticle)
	mux.HandleFunc("GET /api/v1/articles/{id}", a.getArticle)
	mux.HandleFunc("PUT /api/v1/articles/{id}", a.editArticle)
	mux.HandleFunc("DELETE /api/v1/articles/{id}", a.deleteArticle)
	mux.HandleFunc("GET /api/v1/users/{nickname}/articles", a.listArticles)
	mux.HandleFunc("GET /api/v1/stories", a.listFeedStories)
	mux.HandleFunc("GET /api/v1/users/{nickname}/stories", a.listUserStories)
	mux.HandleFunc("POST /api/v1/me/stories", a.tellStory)
	mux.HandleFunc("PUT /api/v1/stories/{id}/view", a.viewStory)
	mux.HandleFunc("GET /api/v1/stories/{id}/viewers", a.listStoryViewers)
	mux.HandleFunc("DELETE /api/v1/stories/{id}", a.deleteStory)
	mux.HandleFunc("PUT /api/v1/stories/{id}/like", a.likeStory)
	mux.HandleFunc("DELETE /api/v1/stories/{id}/like", a.unlikeStory)
	mux.HandleFunc("GET /api/v1/stories/{id}/comments", a.listStoryComments)
	mux.HandleFunc("POST /api/v1/stories/{id}/comments", a.createStoryComment)
	mux.HandleFunc("DELETE /api/v1/story-comments/{id}", a.deleteStoryComment)

	mux.HandleFunc("GET /api/v1/stickers", a.stickerCatalog)
	mux.HandleFunc("PUT /api/v1/me/stickers/{pack}", a.addStickerPack)
	mux.HandleFunc("DELETE /api/v1/me/stickers/{pack}", a.removeStickerPack)
	mux.HandleFunc("GET /api/v1/me/sticker-packs", a.myStickerPacks)
	mux.HandleFunc("POST /api/v1/me/sticker-packs", a.createStickerPack)
	mux.HandleFunc("PATCH /api/v1/sticker-packs/{pack}", a.editStickerPack)
	mux.HandleFunc("DELETE /api/v1/sticker-packs/{pack}", a.deleteStickerPack)
	mux.HandleFunc("POST /api/v1/me/vouchers", a.redeemVoucher)
	mux.HandleFunc("GET /api/v1/me/votes", a.voteStatement)
	mux.HandleFunc("GET /api/v1/me/bonus", a.bonus)
	mux.HandleFunc("POST /api/v1/me/bonus", a.claimBonus)
	mux.HandleFunc("POST /api/v1/me/rating", a.raiseRating)
	mux.HandleFunc("GET /api/v1/treasury", a.treasury)
	mux.HandleFunc("PUT /api/v1/economy", a.setEconomy)
	mux.HandleFunc("POST /api/v1/users/{nickname}/votes", a.transferVotes)
	mux.HandleFunc("GET /api/v1/vouchers", a.listVouchers)
	mux.HandleFunc("POST /api/v1/vouchers", a.createVoucher)
	mux.HandleFunc("DELETE /api/v1/vouchers/{id}", a.deleteVoucher)

	mux.HandleFunc("GET /api/v1/ads", a.listAds)
	mux.HandleFunc("POST /api/v1/ads", a.createAd)
	mux.HandleFunc("PATCH /api/v1/ads/{id}", a.editAd)
	mux.HandleFunc("DELETE /api/v1/ads/{id}", a.deleteAd)
	mux.HandleFunc("POST /api/v1/ads/{id}/views", a.viewAd)
	mux.HandleFunc("POST /api/v1/ads/{id}/clicks", a.clickAd)
	mux.HandleFunc("GET /api/v1/vote-rate", a.voteRate)
	mux.HandleFunc("PUT /api/v1/vote-rate", a.setVoteRate)
	mux.HandleFunc("GET /api/v1/decor", a.decor)
	mux.HandleFunc("PUT /api/v1/decor", a.setDecor)
	mux.HandleFunc("GET /api/v1/prime", a.primePrices)
	mux.HandleFunc("PUT /api/v1/prime/prices", a.setPrimePrices)
	mux.HandleFunc("POST /api/v1/me/prime", a.buyPrime)
	mux.HandleFunc("PUT /api/v1/me/prime/invisible", a.setPrimeInvisible)
	mux.HandleFunc("PUT /api/v1/me/prime/style", a.setPrimeStyle)
	mux.HandleFunc("POST /api/v1/users/{nickname}/prime", a.givePrime)
	mux.HandleFunc("GET /api/v1/casino", a.casino)
	mux.HandleFunc("PUT /api/v1/casino", a.setCasino)
	mux.HandleFunc("GET /api/v1/casino/settings", a.casinoSettings)
	mux.HandleFunc("POST /api/v1/casino/spins", a.spinCasino)
	mux.HandleFunc("PUT /api/v1/casino/bets", a.setCasinoBets)
	mux.HandleFunc("POST /api/v1/casino/wagers", a.betCasino)
	mux.HandleFunc("PUT /api/v1/casino/rocket", a.setCasinoRocket)
	mux.HandleFunc("PUT /api/v1/casino/jackpot", a.setCasinoJackpot)
	mux.HandleFunc("POST /api/v1/casino/rockets", a.launchRocket)
	mux.HandleFunc("GET /api/v1/casino/rockets/{id}", a.rocket)
	mux.HandleFunc("POST /api/v1/casino/rockets/{id}/cash", a.cashRocket)
	mux.HandleFunc("GET /api/v1/casino/poker", a.poker)
	mux.HandleFunc("POST /api/v1/casino/poker/hands", a.dealPoker)
	mux.HandleFunc("POST /api/v1/casino/poker/hands/{id}/{action}", a.playPoker)
	mux.HandleFunc("GET /api/v1/casino/poker/settings", a.pokerSettings)
	mux.HandleFunc("PUT /api/v1/casino/poker/settings", a.setPokerSettings)
	mux.HandleFunc("GET /api/v1/casino/tables", a.listHoldemTables)
	mux.HandleFunc("POST /api/v1/casino/tables", a.createHoldemTable)
	mux.HandleFunc("POST /api/v1/casino/tables/quick", a.quickHoldem)
	mux.HandleFunc("GET /api/v1/casino/tables/{id}", a.holdemTable)
	mux.HandleFunc("POST /api/v1/casino/tables/{id}/seat", a.sitHoldemTable)
	mux.HandleFunc("DELETE /api/v1/casino/tables/{id}/seat", a.leaveHoldemTable)
	mux.HandleFunc("POST /api/v1/casino/tables/{id}/actions", a.actHoldemTable)
	mux.HandleFunc("POST /api/v1/casino/tables/{id}/back", a.backHoldemTable)
	mux.HandleFunc("POST /api/v1/casino/tables/{id}/invitations", a.inviteHoldemTable)

	mux.HandleFunc("GET /api/v1/me/mine", a.mine)
	mux.HandleFunc("POST /api/v1/me/mine/digs", a.digOre)
	mux.HandleFunc("POST /api/v1/me/mine/sales", a.sellOre)
	mux.HandleFunc("GET /api/v1/mine/settings", a.mineSettings)
	mux.HandleFunc("PUT /api/v1/mine/settings", a.setMine)
	mux.HandleFunc("POST /api/v1/users/{nickname}/visits", a.visitPage)
	mux.HandleFunc("GET /api/v1/me/guests", a.listGuests)
	mux.HandleFunc("GET /api/v1/top", a.listTop)
	mux.HandleFunc("GET /api/v1/me/pets", a.getPets)
	mux.HandleFunc("POST /api/v1/me/pets", a.adoptPet)
	mux.HandleFunc("GET /api/v1/me/pets/named", a.namedPet)
	mux.HandleFunc("POST /api/v1/me/pets/named", a.claimNamedPet)
	mux.HandleFunc("GET /api/v1/me/pets/fights", a.listPetFights)
	mux.HandleFunc("POST /api/v1/me/pets/cases", a.openPetCase)
	mux.HandleFunc("POST /api/v1/me/pets/drop", a.takePetDrop)
	mux.HandleFunc("GET /api/v1/me/pets/arena", a.getPetArena)
	mux.HandleFunc("POST /api/v1/me/pets/arena", a.runPetArena)
	mux.HandleFunc("GET /api/v1/canvas", a.getCanvas)
	mux.HandleFunc("GET "+canvasLivePath, a.canvasLive)
	mux.HandleFunc("POST /api/v1/canvas/cells", a.paintCanvas)
	mux.HandleFunc("GET /api/v1/canvas/cells/{x}/{y}", a.getCanvasCell)
	mux.HandleFunc("GET /api/v1/me/dating", a.getDating)
	mux.HandleFunc("PUT /api/v1/me/dating", a.putDating)
	mux.HandleFunc("POST /api/v1/me/dating/answers", a.answerDating)
	mux.HandleFunc("DELETE /api/v1/me/pets/drop", a.leavePetDrop)
	mux.HandleFunc("PATCH /api/v1/me/pets/{id}", a.renamePet)
	mux.HandleFunc("DELETE /api/v1/me/pets/{id}", a.releasePet)
	mux.HandleFunc("POST /api/v1/me/pets/{id}/feed", a.feedPet)
	mux.HandleFunc("POST /api/v1/me/pets/{id}/play", a.playPet)
	mux.HandleFunc("POST /api/v1/me/pets/{id}/fights", a.fightPet)
	mux.HandleFunc("PUT /api/v1/me/pets/{id}/stance", a.setPetStance)
	mux.HandleFunc("POST /api/v1/me/pets/{id}/battle", a.startPetBattle)
	mux.HandleFunc("POST /api/v1/me/pets/{id}/battle/moves", a.movePetBattle)
	mux.HandleFunc("POST /api/v1/me/pets/{id}/clothes", a.buyPetItem)
	mux.HandleFunc("PUT /api/v1/me/pets/{id}/outfit", a.dressPet)
	mux.HandleFunc("GET /api/v1/users/{nickname}/pets", a.getUserPets)
	mux.HandleFunc("GET /api/v1/users/{nickname}/firms", a.getUserFirms)
	mux.HandleFunc("GET /api/v1/users/{nickname}/titles", a.getUserTitles)
	mux.HandleFunc("POST /api/v1/users/{nickname}/titles", a.grantTitle)
	mux.HandleFunc("DELETE /api/v1/titles/{id}", a.revokeTitle)
	mux.HandleFunc("GET /api/v1/pet/settings", a.petSettings)
	mux.HandleFunc("PUT /api/v1/pet/settings", a.setPetSettings)
	mux.HandleFunc("GET /api/v1/firms", a.firms)
	mux.HandleFunc("POST /api/v1/firms/{action}", a.runFirms)
	mux.HandleFunc("GET /api/v1/stocks", a.stocks)
	mux.HandleFunc("POST /api/v1/stocks/{action}", a.runStocks)
	mux.HandleFunc("GET /api/v1/firm/settings", a.firmSettings)
	mux.HandleFunc("PUT /api/v1/firm/settings", a.setFirmSettings)
	mux.HandleFunc("GET /api/v1/me/bank", a.bank)
	mux.HandleFunc("POST /api/v1/me/bank/{action}", a.moveBank)
	mux.HandleFunc("GET /api/v1/bank/settings", a.bankSettings)
	mux.HandleFunc("PUT /api/v1/bank/settings", a.setBankSettings)
	mux.HandleFunc("GET /api/v1/bets", a.betEvents)
	mux.HandleFunc("POST /api/v1/bets", a.createBetEvent)
	mux.HandleFunc("POST /api/v1/bets/{id}/stakes", a.stakeBet)
	mux.HandleFunc("POST /api/v1/bets/{id}/close", a.changeBetEvent(Bets.Close))
	mux.HandleFunc("POST /api/v1/bets/{id}/resolve", a.resolveBetEvent)
	mux.HandleFunc("POST /api/v1/bets/{id}/cancel", a.changeBetEvent(Bets.Cancel))
	mux.HandleFunc("PUT /api/v1/bets/{id}/odds", a.setBetEventOdds)
	mux.HandleFunc("DELETE /api/v1/bets/{id}", a.changeBetEvent(Bets.Delete))
	mux.HandleFunc("PUT /api/v1/users/{nickname}/verification", a.verify)
	mux.HandleFunc("DELETE /api/v1/users/{nickname}/verification", a.verify)
	mux.HandleFunc("PUT /api/v1/users/{nickname}/alert", a.setAlert)
	mux.HandleFunc("DELETE /api/v1/users/{nickname}/alert", a.setAlert)
	mux.HandleFunc("PUT /api/v1/users/{nickname}/ban", a.ban)
	mux.HandleFunc("DELETE /api/v1/users/{nickname}/ban", a.unban)
	mux.HandleFunc("PUT /api/v1/clubs/{ref}/verification", a.verifyClub)
	mux.HandleFunc("DELETE /api/v1/clubs/{ref}/verification", a.verifyClub)

	mux.HandleFunc("PUT /api/v1/me/avatar", a.uploadAvatar)
	mux.HandleFunc("DELETE /api/v1/me/avatar", a.deleteAvatar)
	mux.HandleFunc("GET /media/avatars/{userID}/{avatarID}/{file}", a.serveAvatar)

	mux.HandleFunc("POST /api/v1/me/photos", a.uploadPhoto)
	mux.HandleFunc("GET /media/photos/{userID}/{photoID}/{file}", a.servePhoto)
	mux.HandleFunc("GET /media/stickers/{sticker}", a.serveSticker)
	mux.HandleFunc("GET /media/documents/{id}", a.serveDocument)

	if a.webDir != "" {
		registerPages(mux, a.webDir)
		mux.HandleFunc("GET /robots.txt", a.robots)
		mux.HandleFunc("GET /sitemap.xml", a.sitemap)
	}

	// Catch-all for JSON errors instead of the mux's plain-text ones.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		a.noRoute(mux, w, r)
	})

	return mux
}
