package service

import (
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/room/internal/room"
)

type Filter struct {
	IncludeClosed           bool
	PersonalOnly            bool
	OwnerID, ClubID, Cursor uuid.UUID
	Limit                   int
	PermanentOnly           bool
}
type Member struct {
	UserID                uuid.UUID
	JoinedAt, HeartbeatAt time.Time
}
type Transition struct {
	PreviousRoom   uuid.UUID
	PreviousMember Member
	Joined         bool
}
type Relationships struct{ Friend, Blocked, Banned bool }
type ClubAccess struct{ Member, Manager bool }
type MediaToken struct {
	URL, Token string
	ExpiresAt  time.Time
}
type View struct {
	Friends    int
	Room       *room.Room
	Members    []Member
	Playback   room.Playback
	MaxMembers int
}
type Joined struct {
	View       View
	Chat       []room.Chat
	MutedIDs   []uuid.UUID
	Media      MediaToken
	ServerTime time.Time
}

type Event struct {
	Kind           string
	RoomID, UserID uuid.UUID
	Room           *room.Room
	Chat           *room.Chat
	Playback       *room.Playback
}

type CreateInput struct {
	ActorID uuid.UUID
	Room    room.Input
}
type UpdateInput struct {
	ActorID, RoomID uuid.UUID
	Room            room.Input
}
type PlaybackInput struct {
	ActorID, RoomID uuid.UUID
	Version         uint64
	Playback        room.PlaybackInput
}
type ChatInput struct {
	ActorID, RoomID, StickerID uuid.UUID
	Text                       room.ChatText
}
type MemberInput struct{ ActorID, RoomID, MemberID uuid.UUID }
