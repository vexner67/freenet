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
)

var (
	ErrUnknownKind    = errors.New("unknown notification kind")
	ErrNoRecipient    = errors.New("notification has no recipient")
	ErrNoActor        = errors.New("notification has no actor")
	ErrActorRecipient = errors.New("nobody is notified about their own actions")
	ErrTargetMismatch = errors.New("notification target does not match its kind")
	ErrNotFound       = errors.New("notification not found")
)

// Event is what happened, to whom and by whom. PostID is set for likes and
// comments, CommentID for comments only; unused IDs are uuid.Nil.
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
	case KindLike:
		ok = hasPost && !hasComment
	case KindComment:
		ok = hasPost && hasComment
	case KindFollow:
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
