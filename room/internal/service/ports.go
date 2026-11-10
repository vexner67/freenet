package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/room/internal/room"
)

type RoomRepository interface {
	Delete(context.Context, uuid.UUID) error
	Get(context.Context, uuid.UUID) (*room.Room, error)
	Save(context.Context, *room.Room) error
	List(context.Context, Filter) ([]*room.Room, error)
}
type BanRepository interface {
	DeleteUser(context.Context, uuid.UUID) error
	Until(context.Context, uuid.UUID, uuid.UUID) (time.Time, error)
	SaveBan(context.Context, uuid.UUID, uuid.UUID, time.Time) error
}
type TxManager interface {
	WithinTx(context.Context, func(context.Context) error) error
}

type Presence interface {
	Join(context.Context, uuid.UUID, uuid.UUID, time.Time, int, int) (Transition, error)
	Leave(context.Context, uuid.UUID, uuid.UUID, time.Time) (Member, bool, error)
	Heartbeat(context.Context, uuid.UUID, uuid.UUID, time.Time) error
	Members(context.Context, uuid.UUID) ([]Member, error)
}
type PresenceIndex interface {
	Current(context.Context, []uuid.UUID) (map[uuid.UUID]uuid.UUID, error)
	Live(context.Context) ([]uuid.UUID, error)
	EmptySince(context.Context, uuid.UUID) (time.Time, error)
	Close(context.Context, uuid.UUID, time.Time) ([]Member, error)
}
type PresenceExpiry interface {
	Expire(context.Context, uuid.UUID, uuid.UUID, time.Time, time.Time) (Member, bool, error)
}

type SweepLock interface {
	SweepLocked(context.Context, func(context.Context) error) error
}
type PlaybackStore interface {
	ResetPlayback(context.Context, uuid.UUID, uint64, room.Playback) error
	Playback(context.Context, uuid.UUID) (room.Playback, error)
	SetPlayback(context.Context, uuid.UUID, uuid.UUID, uint64, room.Playback) error
}
type ChatLog interface {
	AppendChat(context.Context, uuid.UUID, room.Chat) error
	Chat(context.Context, uuid.UUID) ([]room.Chat, error)
}
type Users interface {
	Access(context.Context, uuid.UUID, uuid.UUID) (Relationships, error)
	Blocked(context.Context, uuid.UUID, []uuid.UUID) (map[uuid.UUID]bool, error)
	CanInvite(context.Context, uuid.UUID, uuid.UUID) (bool, error)
}
type Clubs interface {
	Access(context.Context, uuid.UUID, uuid.UUID) (ClubAccess, error)
}
type MediaServer interface {
	Token(context.Context, uuid.UUID, uuid.UUID) (MediaToken, error)
	Remove(context.Context, uuid.UUID, uuid.UUID) error
	Mute(context.Context, uuid.UUID, uuid.UUID) error
	Delete(context.Context, uuid.UUID) error
}
type Notifier interface {
	Invite(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, string) error
	Revoke(context.Context, uuid.UUID) error
}
type Publisher interface {
	Publish(context.Context, uuid.UUID, Event)
	Subscribe(uuid.UUID) (<-chan Event, func())
}
type Clock interface{ Now() time.Time }

// Observer records only bounded aggregates; implementations live outside service.
type Observer interface {
	Join(error)
	Leave(time.Duration)
	Playback()
	Snapshot(map[room.Kind]int, int)
}

type MediaTokens interface {
	Verify(string) (uuid.UUID, uuid.UUID, error)
}
