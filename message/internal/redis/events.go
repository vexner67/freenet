// Package redis carries the live events of message between replicas: the
// JSON that pkg/pubsub.Redis sends through a Redis channel.
package redis

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/message/internal/message"
	"github.com/vexner67/freenet/message/internal/service"
	"github.com/vexner67/freenet/pkg/errs"
)

// Channel is the Redis channel of the events.
const Channel = "message:events"

var errNoEvent = errors.New("event is neither a message nor a read mark")

// EventCodec is a pubsub.Codec for service.Event.
type EventCodec struct{}

type eventJSON struct {
	UserID  uuid.UUID    `json:"user_id"`
	Message *messageJSON `json:"message,omitempty"`
	Read    *readJSON    `json:"read,omitempty"`
}

type messageJSON struct {
	ID          uuid.UUID   `json:"id"`
	SenderID    uuid.UUID   `json:"sender_id"`
	RecipientID uuid.UUID   `json:"recipient_id"`
	Text        string      `json:"text"`
	Photos      []uuid.UUID `json:"photos,omitempty"`
	CreatedAt   time.Time   `json:"created_at"`
}

type readJSON struct {
	ReaderID uuid.UUID `json:"reader_id"`
	PeerID   uuid.UUID `json:"peer_id"`
	UpTo     uuid.UUID `json:"up_to"`
}

func (EventCodec) Encode(userID uuid.UUID, e service.Event) ([]byte, error) {
	ej := eventJSON{UserID: userID}

	switch {
	case e.Message != nil:
		m := e.Message
		ej.Message = &messageJSON{
			ID:          m.ID(),
			SenderID:    m.SenderID(),
			RecipientID: m.RecipientID(),
			Text:        m.Text().String(),
			Photos:      m.Photos(),
			CreatedAt:   m.CreatedAt(),
		}
	case e.Read != nil:
		ej.Read = &readJSON{ReaderID: e.Read.ReaderID, PeerID: e.Read.PeerID, UpTo: e.Read.UpTo}
	default:
		return nil, errNoEvent
	}

	data, err := json.Marshal(ej)
	if err != nil {
		return nil, errs.Wrap("json.Marshal", err)
	}

	return data, nil
}

func (EventCodec) Decode(data []byte) (uuid.UUID, service.Event, error) {
	var ej eventJSON
	if err := json.Unmarshal(data, &ej); err != nil {
		return uuid.Nil, service.Event{}, errs.Wrap("json.Unmarshal", err)
	}

	switch {
	case ej.Message != nil:
		m := ej.Message

		// From Redis, so checked as any input is.
		text, err := message.NewText(m.Text)
		if err != nil {
			return uuid.Nil, service.Event{}, errs.Wrap("message.NewText", err)
		}

		photos, err := message.NewPhotos(m.Photos)
		if err != nil {
			return uuid.Nil, service.Event{}, errs.Wrap("message.NewPhotos", err)
		}

		content, err := message.NewContent(text, photos)
		if err != nil {
			return uuid.Nil, service.Event{}, errs.Wrap("message.NewContent", err)
		}

		return ej.UserID, service.Event{
			Message: message.Restore(message.Stored{
				ID: m.ID, SenderID: m.SenderID, RecipientID: m.RecipientID, Content: content, CreatedAt: m.CreatedAt,
			}),
		}, nil
	case ej.Read != nil:
		return ej.UserID, service.Event{
			Read: &service.ReadMark{ReaderID: ej.Read.ReaderID, PeerID: ej.Read.PeerID, UpTo: ej.Read.UpTo},
		}, nil
	default:
		return uuid.Nil, service.Event{}, errNoEvent
	}
}
