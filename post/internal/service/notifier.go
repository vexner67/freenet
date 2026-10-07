package service

import (
	"context"

	"github.com/google/uuid"
)

// EventKind is what someone did with a post, for notifications.
type EventKind int

const (
	EventLike EventKind = iota + 1
	EventComment
	EventWallPost
	EventRepost
)

// Event: RecipientID is the author of the post for likes, comments and
// reposts and the owner of the wall for wall posts, ActorID who did it.
// PostID is the post, but for reposts the repost, which shows the shared
// post. CommentID is uuid.Nil but for comments.
type Event struct {
	Kind        EventKind
	RecipientID uuid.UUID
	ActorID     uuid.UUID
	PostID      uuid.UUID
	CommentID   uuid.UUID
}

// Notifier records what the notification service has to hear about posts.
// It is called inside the transaction of the change (TxManager), so the
// notification is recorded if and only if the change is; it is delivered
// afterwards, retried while the notification service is down.
type Notifier interface {
	Notify(ctx context.Context, e Event) error
	// Retract takes back the notification of an undone like or comment.
	Retract(ctx context.Context, e Event) error
	// ForgetPost drops the notifications about a deleted post.
	ForgetPost(ctx context.Context, postID uuid.UUID) error
}

// PhotoKeeper records what the media service has to do with the photos
// of posts, inside the transaction of the change, as Notifier: media keeps
// the photos of a published post for good and deletes those of a deleted
// one. Unpublished photos it deletes after a while by itself.
type PhotoKeeper interface {
	Publish(ctx context.Context, ownerID uuid.UUID, photoIDs []uuid.UUID) error
	Delete(ctx context.Context, ownerID uuid.UUID, photoIDs []uuid.UUID) error
}

// TxManager runs fn in one transaction: everything it saves is kept if it
// returns nil and undone otherwise.
type TxManager interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}
