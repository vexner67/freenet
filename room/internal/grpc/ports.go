package grpc

import (
	"context"
	"github.com/google/uuid"
	"github.com/vexner67/freenet/room/internal/room"
	"github.com/vexner67/freenet/room/internal/service"
)

type Reader interface {
	GetRoom(context.Context, uuid.UUID, uuid.UUID) (service.View, error)
	ListRooms(context.Context, uuid.UUID, service.Filter, bool) ([]service.View, uuid.UUID, error)
	CurrentRooms(context.Context, uuid.UUID, []uuid.UUID) (map[uuid.UUID]service.View, error)
}
type Editor interface {
	CreateRoom(context.Context, service.CreateInput) (*room.Room, error)
	UpdateRoom(context.Context, service.UpdateInput) (service.View, error)
	CloseRoom(context.Context, uuid.UUID, uuid.UUID) error
}
type Memberships interface {
	JoinRoom(context.Context, uuid.UUID, uuid.UUID) (service.Joined, error)
	LeaveRoom(context.Context, uuid.UUID, uuid.UUID) error
	Heartbeat(context.Context, uuid.UUID, uuid.UUID) error
	Token(context.Context, uuid.UUID, uuid.UUID) (service.MediaToken, error)
}
type Content interface {
	SendChat(context.Context, service.ChatInput) (room.Chat, error)
	SetPlayback(context.Context, service.PlaybackInput) (room.Playback, error)
}
type Management interface {
	KickMember(context.Context, service.MemberInput) error
	MuteMember(context.Context, service.MemberInput) error
	Invite(context.Context, service.MemberInput) error
}
type Cleaner interface {
	PurgeUser(context.Context, uuid.UUID) error
	PurgeClub(context.Context, uuid.UUID) error
}
type Subscriber interface {
	Subscribe(uuid.UUID) (<-chan service.Event, func())
}
type MediaAccess interface {
	AuthorizeMedia(context.Context, string) error
}

type Deps struct {
	MediaAccess MediaAccess
	Reader      Reader
	Editor      Editor
	Members     Memberships
	Content     Content
	Management  Management
	Cleaner     Cleaner
	Events      Subscriber
	MaxMembers  int
}
