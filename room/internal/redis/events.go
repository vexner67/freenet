package redis

import (
	"encoding/json"
	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/room/internal/room"
	"github.com/vexner67/freenet/room/internal/service"
	"time"
)

type roomRecord struct {
	ID, OwnerID, ClubID uuid.UUID
	Title               room.Title
	Kind                room.Kind
	Access              room.Access
	Control             room.Control
	Permanent           bool
	CreatedAt, ClosedAt time.Time
}

func encodeRoom(r *room.Room) *roomRecord {
	if r == nil {
		return nil
	}
	return &roomRecord{r.ID(), r.OwnerID(), r.ClubID(), r.Title(), r.Kind(), r.Access(), r.Control(), r.Permanent(), r.CreatedAt(), r.ClosedAt()}
}
func (r roomRecord) domain() *room.Room {
	return room.Restore(r.ID, room.Input{OwnerID: r.OwnerID, ClubID: r.ClubID, Title: r.Title, Kind: r.Kind, Access: r.Access, Control: r.Control, Permanent: r.Permanent}, r.CreatedAt, r.ClosedAt)
}

type eventRecord struct {
	Recipient      uuid.UUID
	Kind           string
	RoomID, UserID uuid.UUID
	Room           *roomRecord
	Chat           *chat
	Playback       *playback
}
type EventCodec struct{}

func (EventCodec) Encode(user uuid.UUID, e service.Event) ([]byte, error) {
	record := eventRecord{Recipient: user, Kind: e.Kind, RoomID: e.RoomID, UserID: e.UserID, Room: encodeRoom(e.Room)}
	if e.Chat != nil {
		c := encodeChat(*e.Chat)
		record.Chat = &c
	}
	if e.Playback != nil {
		p := encodePlayback(*e.Playback)
		record.Playback = &p
	}
	raw, err := json.Marshal(record)
	return raw, errs.Wrap("json.Marshal", err)
}
func (EventCodec) Decode(raw []byte) (uuid.UUID, service.Event, error) {
	var record eventRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return uuid.Nil, service.Event{}, errs.Wrap("json.Unmarshal", err)
	}
	e := service.Event{Kind: record.Kind, RoomID: record.RoomID, UserID: record.UserID}
	if record.Room != nil {
		e.Room = record.Room.domain()
	}
	if record.Chat != nil {
		c := record.Chat.domain()
		e.Chat = &c
	}
	if record.Playback != nil {
		p, err := record.Playback.domain()
		if err != nil {
			return uuid.Nil, service.Event{}, errs.Wrap("record.Playback.domain", err)
		}
		e.Playback = &p
	}
	return record.Recipient, e, nil
}
