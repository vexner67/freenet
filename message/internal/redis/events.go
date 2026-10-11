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

var errNoEvent = errors.New("event is neither a message, a read mark, a deletion nor an edit")

// EventCodec is a pubsub.Codec for service.Event.
type EventCodec struct{}

type eventJSON struct {
	UserID  uuid.UUID    `json:"user_id"`
	Message *messageJSON `json:"message,omitempty"`
	Read    *readJSON    `json:"read,omitempty"`
	Deleted *deletedJSON `json:"deleted,omitempty"`
	Edited  *messageJSON `json:"edited,omitempty"`
}

type deletedJSON struct {
	ID          uuid.UUID `json:"id"`
	SenderID    uuid.UUID `json:"sender_id"`
	RecipientID uuid.UUID `json:"recipient_id"`
}

type messageJSON struct {
	ID          uuid.UUID   `json:"id"`
	SenderID    uuid.UUID   `json:"sender_id"`
	RecipientID uuid.UUID   `json:"recipient_id"`
	Text        string      `json:"text"`
	Photos      []uuid.UUID `json:"photos,omitempty"`
	Post        uuid.UUID   `json:"post"`
	SharedPhoto uuid.UUID   `json:"shared_photo"`
	CreatedAt   time.Time   `json:"created_at"`
	// EditedAt is zero for a message never edited.
	EditedAt  time.Time       `json:"edited_at"`
	Forwarded []forwardedJSON `json:"forwarded,omitempty"`
	// DeletedAt is zero for a message never taken back.
	DeletedAt time.Time `json:"deleted_at"`
}

type forwardedJSON struct {
	SenderID  uuid.UUID   `json:"sender_id"`
	Text      string      `json:"text"`
	Photos    []uuid.UUID `json:"photos,omitempty"`
	Post      uuid.UUID   `json:"post"`
	Photo     uuid.UUID   `json:"shared_photo"`
	CreatedAt time.Time   `json:"created_at"`
}

func toMessageJSON(m *message.Message) *messageJSON {
	forwarded := make([]forwardedJSON, len(m.Forwarded()))
	for i, f := range m.Forwarded() {
		forwarded[i] = forwardedJSON{
			SenderID: f.SenderID, Text: f.Text.String(), Photos: f.Photos, Post: f.Post, Photo: f.SharedPhoto, CreatedAt: f.CreatedAt,
		}
	}

	return &messageJSON{
		Forwarded:   forwarded,
		DeletedAt:   m.DeletedAt(),
		ID:          m.ID(),
		SenderID:    m.SenderID(),
		RecipientID: m.RecipientID(),
		Text:        m.Text().String(),
		Photos:      m.Photos(),
		Post:        m.Post(),
		SharedPhoto: m.SharedPhoto(),
		CreatedAt:   m.CreatedAt(),
		EditedAt:    m.EditedAt(),
	}
}

// message rebuilds the message of an event. It comes from Redis, so it
// is checked as any input is.
func (m *messageJSON) message() (*message.Message, error) {
	stored := message.Stored{
		ID: m.ID, SenderID: m.SenderID, RecipientID: m.RecipientID, CreatedAt: m.CreatedAt, EditedAt: m.EditedAt, DeletedAt: m.DeletedAt,
	}

	// A message taken back says nothing.
	if !m.DeletedAt.IsZero() {
		return message.Restore(stored), nil
	}

	text, err := message.NewText(m.Text)
	if err != nil {
		return nil, errs.Wrap("message.NewText", err)
	}

	photos, err := message.NewPhotos(m.Photos)
	if err != nil {
		return nil, errs.Wrap("message.NewPhotos", err)
	}

	forwarded, err := m.forwarded()
	if err != nil {
		return nil, errs.Wrap("m.forwarded", err)
	}

	if m.SharedPhoto != uuid.Nil {
		if stored.Content, err = message.NewPhotoShare(text, m.SharedPhoto); err != nil {
			return nil, errs.Wrap("message.NewPhotoShare", err)
		}

		return message.Restore(stored), nil
	}

	if stored.Content, err = message.NewContent(text, photos, m.Post, forwarded); err != nil {
		return nil, errs.Wrap("message.NewContent", err)
	}

	return message.Restore(stored), nil
}

func (m *messageJSON) forwarded() (message.Forwards, error) {
	carried := make([]message.Forwarded, len(m.Forwarded))
	for i, f := range m.Forwarded {
		text, err := message.NewText(f.Text)
		if err != nil {
			return nil, errs.Wrap("message.NewText", err)
		}

		photos, err := message.NewPhotos(f.Photos)
		if err != nil {
			return nil, errs.Wrap("message.NewPhotos", err)
		}

		carried[i] = message.Forwarded{
			SenderID: f.SenderID, Text: text, Photos: photos, Post: f.Post, SharedPhoto: f.Photo, CreatedAt: f.CreatedAt,
		}
	}

	forwards, err := message.NewForwards(carried)
	if err != nil {
		return nil, errs.Wrap("message.NewForwards", err)
	}

	return forwards, nil
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
		ej.Message = toMessageJSON(e.Message)
	case e.Edited != nil:
		ej.Edited = toMessageJSON(e.Edited)
	case e.Read != nil:
		ej.Read = &readJSON{ReaderID: e.Read.ReaderID, PeerID: e.Read.PeerID, UpTo: e.Read.UpTo}
	case e.Deleted != nil:
		ej.Deleted = &deletedJSON{ID: e.Deleted.ID, SenderID: e.Deleted.SenderID, RecipientID: e.Deleted.RecipientID}
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
		m, err := ej.Message.message()
		if err != nil {
			return uuid.Nil, service.Event{}, errs.Wrap("ej.Message.message", err)
		}

		return ej.UserID, service.Event{Message: m}, nil
	case ej.Edited != nil:
		m, err := ej.Edited.message()
		if err != nil {
			return uuid.Nil, service.Event{}, errs.Wrap("ej.Edited.message", err)
		}

		return ej.UserID, service.Event{Edited: m}, nil
	case ej.Read != nil:
		return ej.UserID, service.Event{
			Read: &service.ReadMark{ReaderID: ej.Read.ReaderID, PeerID: ej.Read.PeerID, UpTo: ej.Read.UpTo},
		}, nil
	case ej.Deleted != nil:
		return ej.UserID, service.Event{
			Deleted: &service.Deleted{ID: ej.Deleted.ID, SenderID: ej.Deleted.SenderID, RecipientID: ej.Deleted.RecipientID},
		}, nil
	default:
		return uuid.Nil, service.Event{}, errNoEvent
	}
}
