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

// Notifier tells the notification service what happened to posts. Delivery
// is best effort: a lost notification must not fail the like or comment
// that caused it, so the methods return nothing and implementations handle
// their failures themselves.
type Notifier interface {
	Notify(ctx context.Context, e Event)
	// Retract takes back the notification of an undone like or comment.
	Retract(ctx context.Context, e Event)
	// ForgetPost drops the notifications about a deleted post.
	ForgetPost(ctx context.Context, postID uuid.UUID)
}
