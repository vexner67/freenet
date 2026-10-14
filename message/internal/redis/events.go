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

var errNoEvent = errors.New("event is neither a message, a read mark, a deletion, an edit nor of a chat")

// EventCodec is a pubsub.Codec for service.Event.
type EventCodec struct{}

type eventJSON struct {
	UserID  uuid.UUID    `json:"user_id"`
	Message *messageJSON `json:"message,omitempty"`
	Read    *readJSON    `json:"read,omitempty"`
	Deleted *deletedJSON `json:"deleted,omitempty"`
	Edited  *messageJSON `json:"edited,omitempty"`
	Chat    *chatJSON    `json:"chat,omitempty"`
}

// chatJSON is what happened in a chat: one of the three.
type chatJSON struct {
	Message *chatMessageJSON `json:"message,omitempty"`
	Edited  *chatMessageJSON `json:"edited,omitempty"`
	Read    *chatReadJSON    `json:"read,omitempty"`
}

type chatReadJSON struct {
	ChatID uuid.UUID `json:"chat_id"`
	UpTo   uuid.UUID `json:"up_to"`
}

type chatMessageJSON struct {
	ID        uuid.UUID   `json:"id"`
	ChatID    uuid.UUID   `json:"chat_id"`
	SenderID  uuid.UUID   `json:"sender_id"`
	Text      string      `json:"text"`
	Photos    []uuid.UUID `json:"photos,omitempty"`
	Action    string      `json:"action,omitempty"`
	TargetID  uuid.UUID   `json:"target_id"`
	CreatedAt time.Time   `json:"created_at"`
	// EditedAt and DeletedAt are zero for never.
	EditedAt    time.Time       `json:"edited_at"`
	DeletedAt   time.Time       `json:"deleted_at"`
	Post        uuid.UUID       `json:"post"`
	SharedPhoto uuid.UUID       `json:"shared_photo"`
	Forwarded   []forwardedJSON `json:"forwarded,omitempty"`
}

func toChatMessageJSON(m *message.ChatMessage) *chatMessageJSON {
	if m == nil {
		return nil
	}

	return &chatMessageJSON{
		ID:        m.ID(),
		ChatID:    m.ChatID(),
		SenderID:  m.SenderID(),
		Text:      m.Text().String(),
		Photos:    m.Photos(),
		Action:    string(m.Action()),
		TargetID:  m.TargetID(),
		CreatedAt: m.CreatedAt(),
		EditedAt:  m.EditedAt(),
		DeletedAt: m.DeletedAt(),

		Post:        m.Post(),
		SharedPhoto: m.SharedPhoto(),
		Forwarded:   toForwardedJSON(m.Forwarded()),
	}
}

var errUnknownAction = errors.New("unknown chat action")

// message rebuilds the message of a chat event. It comes from Redis, so
// it is checked as any input is.
func (m *chatMessageJSON) message() (*message.ChatMessage, error) {
	switch message.Action(m.Action) {
	case message.ActionNone, message.ActionCreate, message.ActionTitle, message.ActionInvite,
		message.ActionKick, message.ActionLeave, message.ActionReturn:
	default:
		return nil, errUnknownAction
	}

	stored := message.StoredChatMessage{
		ID: m.ID, ChatID: m.ChatID, SenderID: m.SenderID, Action: message.Action(m.Action),
		TargetID: m.TargetID, CreatedAt: m.CreatedAt, EditedAt: m.EditedAt, DeletedAt: m.DeletedAt,
	}

	// What happened in the chat says at most a title; a message taken
	// back says nothing.
	if stored.Action != message.ActionNone || !m.DeletedAt.IsZero() {
		text, err := message.NewText(m.Text)
		if err != nil {
			return nil, errs.Wrap("message.NewText", err)
		}

		stored.Content = message.RestoreContent(text, nil, uuid.Nil, nil)

		return message.RestoreChatMessage(stored), nil
	}

	var err error
	if stored.Content, err = contentOf(m.Text, m.Photos, m.Post, m.SharedPhoto, m.Forwarded); err != nil {
		return nil, errs.Wrap("contentOf", err)
	}

	return message.RestoreChatMessage(stored), nil
}

func toChatJSON(e *service.ChatEvent) *chatJSON {
	cj := &chatJSON{Message: toChatMessageJSON(e.Message), Edited: toChatMessageJSON(e.Edited)}
	if e.Read != nil {
		cj.Read = &chatReadJSON{ChatID: e.Read.ChatID, UpTo: e.Read.UpTo}
	}

	return cj
}

// event rebuilds what happened in a chat.
func (c *chatJSON) event() (*service.ChatEvent, error) {
	switch {
	case c.Message != nil:
		m, err := c.Message.message()

		return &service.ChatEvent{Message: m}, errs.Wrap("c.Message.message", err)
	case c.Edited != nil:
		m, err := c.Edited.message()

		return &service.ChatEvent{Edited: m}, errs.Wrap("c.Edited.message", err)
	case c.Read != nil:
		return &service.ChatEvent{Read: &service.ChatRead{ChatID: c.Read.ChatID, UpTo: c.Read.UpTo}}, nil
	default:
		return nil, errNoEvent
	}
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
	// Sealed is what a message of a dialog says, sealed: base64 in JSON.
	Sealed []byte `json:"sealed,omitempty"`
}

type forwardedJSON struct {
	SenderID  uuid.UUID   `json:"sender_id"`
	Text      string      `json:"text"`
	Photos    []uuid.UUID `json:"photos,omitempty"`
	Post      uuid.UUID   `json:"post"`
	Photo     uuid.UUID   `json:"shared_photo"`
	CreatedAt time.Time   `json:"created_at"`
	Depth     int         `json:"depth,omitempty"`
}

func toForwardedJSON(carried message.Forwards) []forwardedJSON {
	forwarded := make([]forwardedJSON, len(carried))
	for i, f := range carried {
		forwarded[i] = forwardedJSON{
			SenderID: f.SenderID, Text: f.Text.String(), Photos: f.Photos, Post: f.Post, Photo: f.SharedPhoto, CreatedAt: f.CreatedAt, Depth: f.Depth,
		}
	}

	return forwarded
}

func toMessageJSON(m *message.Message) *messageJSON {
	return &messageJSON{
		Forwarded:   toForwardedJSON(m.Forwarded()),
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
		Sealed:      m.Sealed().Bytes(),
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

	sealed, err := message.NewSealed(m.Sealed)
	if err != nil {
		return nil, errs.Wrap("message.NewSealed", err)
	}

	if stored.Content, err = sealedContentOf(sealed, m.Photos, m.Post, m.SharedPhoto, m.Forwarded); err != nil {
		return nil, errs.Wrap("sealedContentOf", err)
	}

	return message.Restore(stored), nil
}

// sealedContentOf rebuilds what a message of a dialog says and shows,
// checking it as a new one.
func sealedContentOf(sealed message.Sealed, photoIDs []uuid.UUID, post, sharedPhoto uuid.UUID, carried []forwardedJSON) (message.Content, error) {
	photos, err := message.NewPhotos(photoIDs)
	if err != nil {
		return message.Content{}, errs.Wrap("message.NewPhotos", err)
	}

	forwarded, err := forwardsOf(carried)
	if err != nil {
		return message.Content{}, errs.Wrap("forwardsOf", err)
	}

	if sharedPhoto != uuid.Nil {
		content, err := message.NewSealedPhotoShare(sealed, sharedPhoto)

		return content, errs.Wrap("message.NewSealedPhotoShare", err)
	}

	content, err := message.NewSealedContent(sealed, photos, post, forwarded)

	return content, errs.Wrap("message.NewSealedContent", err)
}

// contentOf rebuilds what a message of a chat says and shows, checking it
// as a new one.
func contentOf(said string, photoIDs []uuid.UUID, post, sharedPhoto uuid.UUID, carried []forwardedJSON) (message.Content, error) {
	text, err := message.NewText(said)
	if err != nil {
		return message.Content{}, errs.Wrap("message.NewText", err)
	}

	photos, err := message.NewPhotos(photoIDs)
	if err != nil {
		return message.Content{}, errs.Wrap("message.NewPhotos", err)
	}

	forwarded, err := forwardsOf(carried)
	if err != nil {
		return message.Content{}, errs.Wrap("forwardsOf", err)
	}

	if sharedPhoto != uuid.Nil {
		content, err := message.NewPhotoShare(text, sharedPhoto)

		return content, errs.Wrap("message.NewPhotoShare", err)
	}

	content, err := message.NewContent(text, photos, post, forwarded)

	return content, errs.Wrap("message.NewContent", err)
}

func forwardsOf(raw []forwardedJSON) (message.Forwards, error) {
	carried := make([]message.Forwarded, len(raw))
	for i, f := range raw {
		text, err := message.NewText(f.Text)
		if err != nil {
			return nil, errs.Wrap("message.NewText", err)
		}

		photos, err := message.NewPhotos(f.Photos)
		if err != nil {
			return nil, errs.Wrap("message.NewPhotos", err)
		}

		carried[i] = message.Forwarded{
			SenderID: f.SenderID, Text: text, Photos: photos, Post: f.Post, SharedPhoto: f.Photo, CreatedAt: f.CreatedAt, Depth: f.Depth,
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
	case e.Chat != nil:
		ej.Chat = toChatJSON(e.Chat)
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
	case ej.Chat != nil:
		chat, err := ej.Chat.event()
		if err != nil {
			return uuid.Nil, service.Event{}, errs.Wrap("ej.Chat.event", err)
		}

		return ej.UserID, service.Event{Chat: chat}, nil
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
