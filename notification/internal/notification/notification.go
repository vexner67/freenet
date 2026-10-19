// Package notification is the domain of notifications: what other people
// did with a user's posts and page.
package notification

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// Kind is what happened.
type Kind string

const (
	// KindLike: the actor liked the recipient's post.
	KindLike Kind = "like"
	// KindComment: the actor commented on the recipient's post.
	KindComment Kind = "comment"
	// KindFollow: the actor followed the recipient.
	KindFollow Kind = "follow"
	// KindWallPost: the actor wrote a post on the recipient's wall.
	KindWallPost Kind = "wall_post"
	// KindRepost: the actor shared the recipient's post; PostID is the
	// actor's repost, which shows the shared post.
	KindRepost Kind = "repost"
	// KindCommentReply: the actor answered the recipient's comment under
	// PostID; CommentID is the answer.
	KindCommentReply Kind = "comment_reply"
	// KindCommentLike: the actor liked the recipient's comment CommentID
	// under PostID.
	KindCommentLike Kind = "comment_like"
	// KindPostAccepted: the actor, a community, published the post the
	// recipient suggested for its wall; PostID is the published post.
	KindPostAccepted Kind = "post_accepted"
	// KindSuggestedPost: someone suggested a post for the wall of the
	// actor, a community the recipient runs ("новые записи в предложке" of
	// openvk); PostID is the suggestion, not a post.
	KindSuggestedPost Kind = "suggested_post"
	// KindClubAdmin: the recipient was made an admin of the actor, a
	// community ("назначил вас руководителем сообщества" of openvk).
	KindClubAdmin Kind = "club_admin"
	// KindTopicMessage: the actor wrote in the topic the recipient
	// started; PostID is the topic and CommentID the message.
	KindTopicMessage Kind = "topic_message"
	// KindPostMention: the actor named the recipient in the post PostID
	// ("Вас упомянул пользователь" of openvk).
	KindPostMention Kind = "post_mention"
	// KindCommentMention: the actor named the recipient in the comment
	// CommentID under the post PostID.
	KindCommentMention Kind = "comment_mention"
	// KindTopicMention: the actor named the recipient in the message
	// CommentID of the topic PostID.
	KindTopicMention Kind = "topic_mention"
	// KindPhotoLike: the actor liked the photo PostID the recipient put
	// into an album.
	KindPhotoLike Kind = "photo_like"
	// KindPhotoComment: the actor wrote the comment CommentID under the
	// recipient's photo PostID ("оставил комментарий под вашей
	// фотографией" of openvk).
	KindPhotoComment Kind = "photo_comment"
	// KindTopicMessageLike: the actor liked the recipient's message
	// CommentID of the topic PostID.
	KindTopicMessageLike Kind = "topic_message_like"
	// KindPhotoMention: the actor named the recipient in the comment
	// CommentID under the photo PostID.
	KindPhotoMention Kind = "photo_mention"
	// KindNoteComment: the actor wrote the comment CommentID under the
	// recipient's note PostID.
	KindNoteComment Kind = "note_comment"
	// KindGift: the actor sent the recipient the gift PostID.
	KindGift Kind = "gift"
	// KindVotes: the actor transferred votes to the recipient; PostID
	// names the transfer.
	KindVotes Kind = "votes"
	// KindInvite: the actor signed up by the recipient's invitation link
	// and follows the recipient now.
	KindInvite Kind = "invite"
	// KindVerified: the actor, an administrator of the site, gave the
	// recipient's page the checkmark. One per page, whoever gave it.
	KindVerified Kind = "verified"
	// KindClubVerified: the actor, a community the recipient owns, got the
	// checkmark of the administrators of the site. One per community,
	// whoever owns it.
	KindClubVerified Kind = "club_verified"
	// KindStoryLike: the actor liked the recipient's story PostID.
	KindStoryLike Kind = "story_like"
	// KindStoryComment: the actor wrote the comment CommentID under the
	// recipient's story PostID.
	KindStoryComment Kind = "story_comment"
)

var (
	ErrUnknownKind    = errors.New("unknown notification kind")
	ErrNoRecipient    = errors.New("notification has no recipient")
	ErrNoActor        = errors.New("notification has no actor")
	ErrActorRecipient = errors.New("nobody is notified about their own actions")
	ErrTargetMismatch = errors.New("notification target does not match its kind")
	ErrNotFound       = errors.New("notification not found")
)

// Event is what happened, to whom and by whom. PostID is set for all but
// follows, invitations, appointments and checkmarks (it is the photo or
// the topic for theirs),
// CommentID for comments, replies, comment likes, messages of topics and
// their likes, comments of photos and mentions in comments and topics;
// unused IDs are uuid.Nil.
type Event struct {
	Kind        Kind
	RecipientID uuid.UUID
	ActorID     uuid.UUID
	PostID      uuid.UUID
	CommentID   uuid.UUID
}

// Validate checks that the event names exactly what its kind needs.
func (e Event) Validate() error {
	if e.RecipientID == uuid.Nil {
		return ErrNoRecipient
	}

	if e.ActorID == uuid.Nil {
		return ErrNoActor
	}

	if e.ActorID == e.RecipientID {
		return ErrActorRecipient
	}

	hasPost, hasComment := e.PostID != uuid.Nil, e.CommentID != uuid.Nil

	var ok bool

	switch e.Kind {
	case KindLike, KindWallPost, KindRepost, KindPostAccepted, KindSuggestedPost, KindPostMention, KindPhotoLike, KindGift,
		KindVotes, KindStoryLike:
		ok = hasPost && !hasComment
	case KindComment, KindCommentReply, KindCommentLike, KindTopicMessage, KindCommentMention, KindTopicMention,
		KindPhotoComment, KindTopicMessageLike, KindPhotoMention, KindNoteComment,
		KindStoryComment:
		ok = hasPost && hasComment
	case KindFollow, KindClubAdmin, KindInvite, KindVerified, KindClubVerified:
		ok = !hasPost && !hasComment
	default:
		return ErrUnknownKind
	}

	if !ok {
		return ErrTargetMismatch
	}

	return nil
}

type Notification struct {
	id        uuid.UUID
	event     Event
	createdAt time.Time
}

// New creates a notification about a valid event. Its UUIDv7 ID grows with
// time, so notifications are ordered and paged by ID, and "read up to"
// is one ID.
func New(e Event, now time.Time) (*Notification, error) {
	if err := e.Validate(); err != nil {
		return nil, err
	}

	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	return &Notification{
		id:    id,
		event: e,
		// PostgreSQL keeps microseconds; trim now so it reads back equal.
		createdAt: now.UTC().Truncate(time.Microsecond),
	}, nil
}

// Restore rebuilds a notification from storage without validation or new IDs.
// Before is the ID below every notification made at t or later and above
// every one made earlier: IDs are UUIDv7, which begin with the time.
func Before(t time.Time) uuid.UUID {
	var id uuid.UUID

	ms := max(t.UnixMilli(), 0)
	for i := 5; i >= 0; i-- {
		id[i] = byte(ms)
		ms >>= 8
	}

	return id
}

func Restore(id uuid.UUID, e Event, createdAt time.Time) *Notification {
	return &Notification{
		id:        id,
		event:     e,
		createdAt: createdAt,
	}
}

func (n *Notification) ID() uuid.UUID {
	return n.id
}

func (n *Notification) Event() Event {
	return n.event
}

func (n *Notification) CreatedAt() time.Time {
	return n.createdAt
}
