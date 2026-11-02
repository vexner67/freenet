package fedi

import (
	"time"

	"github.com/google/uuid"
)

// Follower is someone of another server who follows a page here: their
// server gets what the page posts at Inbox (the shared inbox of the
// server when it has one, so that a post goes to it once).
type Follower struct {
	userID    uuid.UUID
	actor     URL
	inbox     URL
	createdAt time.Time
}

func NewFollower(userID uuid.UUID, actor *Remote, now time.Time) *Follower {
	return &Follower{
		userID:    userID,
		actor:     actor.ID(),
		inbox:     actor.DeliveryInbox(),
		createdAt: now.UTC(),
	}
}

func RestoreFollower(userID uuid.UUID, actor, inbox URL, createdAt time.Time) *Follower {
	return &Follower{
		userID:    userID,
		actor:     actor,
		inbox:     inbox,
		createdAt: createdAt,
	}
}

func (f *Follower) UserID() uuid.UUID {
	return f.userID
}

func (f *Follower) Actor() URL {
	return f.actor
}

func (f *Follower) Inbox() URL {
	return f.inbox
}

func (f *Follower) CreatedAt() time.Time {
	return f.createdAt
}
