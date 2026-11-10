package model

import "time"

type RoomSettings struct{ Title, Kind, Access, Control string }
type RoomDraft struct {
	RoomSettings
	ClubID    string
	Permanent bool
}
type Room struct {
	Friends             int
	ID, OwnerID, ClubID string
	RoomSettings
	Permanent  bool
	CreatedAt  time.Time
	Members    []RoomMember
	Playback   RoomPlayback
	MaxMembers int
}
type RoomMember struct {
	UserID   string
	JoinedAt time.Time
}
type RoomQueueItem struct{ Kind, MediaID, Title string }
type RoomPlayback struct {
	Kind, MediaID string
	Playing       bool
	Position      float64
	UpdatedAt     time.Time
	Version       uint64
	Queue         []RoomQueueItem
}
type RoomChat struct {
	ID, RoomID, AuthorID, Text, StickerID string
	CreatedAt                             time.Time
}
type RoomMedia struct {
	URL, Token string
	ExpiresAt  time.Time
}
type RoomJoined struct {
	Room       Room
	Chat       []RoomChat
	MutedIDs   []string
	Media      RoomMedia
	ServerTime time.Time
}
type RoomPage struct {
	Rooms      []Room
	NextCursor string
}
type RoomQuery struct {
	OwnerID, ClubID, Cursor string
	Limit                   int
}
type RoomSummary struct{ ID, Title string }
type RoomEvent struct {
	Kind, RoomID, UserID string
	Room                 *Room
	Chat                 *RoomChat
	Playback             *RoomPlayback
}
type RoomEvents func() (RoomEvent, error)
