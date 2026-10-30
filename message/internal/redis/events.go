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

var errNoEvent = errors.New("event is neither a message, a read mark, a deletion, an edit, typing, a call nor of a chat")

// EventCodec is a pubsub.Codec for service.Event.
type EventCodec struct{}

type eventJSON struct {
	UserID  uuid.UUID    `json:"user_id"`
	Message *messageJSON `json:"message,omitempty"`
	Read    *readJSON    `json:"read,omitempty"`
	Deleted *deletedJSON `json:"deleted,omitempty"`
	Edited  *messageJSON `json:"edited,omitempty"`
	Chat    *chatJSON    `json:"chat,omitempty"`
	Typing  *typingJSON  `json:"typing,omitempty"`
	Call    *callJSON    `json:"call,omitempty"`
}

// callJSON is a sealed signal of the call with peer_id.
type callJSON struct {
	PeerID uuid.UUID `json:"peer_id"`
	Sealed string    `json:"sealed"`
}

// typingJSON: user_id is writing, in chat_id or, when it is the nil UUID,
// in the dialog with whoever hears it.
type typingJSON struct {
	UserID uuid.UUID `json:"user_id"`
	ChatID uuid.UUID `json:"chat_id"`
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
	Sticker     string          `json:"sticker,omitempty"`
	Documents   []uuid.UUID     `json:"documents,omitempty"`
	ReplyTo     uuid.UUID       `json:"reply_to"`
	Quote       *quoteJSON      `json:"quote,omitempty"`
	Reactions   []reactionJSON  `json:"reactions,omitempty"`
}

// quoteJSON is what an answer shows of the message it answers.
type quoteJSON struct {
	ID          uuid.UUID `json:"id"`
	SenderID    uuid.UUID `json:"sender_id"`
	Text        string    `json:"text,omitempty"`
	Photos      int       `json:"photos,omitempty"`
	Forwarded   int       `json:"forwarded,omitempty"`
	Documents   int       `json:"documents,omitempty"`
	Sticker     string    `json:"sticker,omitempty"`
	Post        bool      `json:"post,omitempty"`
	SharedPhoto bool      `json:"shared_photo,omitempty"`
	Deleted     bool      `json:"deleted,omitempty"`
	Found       bool      `json:"found,omitempty"`
}

type reactionJSON struct {
	UserID   uuid.UUID `json:"user_id"`
	Reaction string    `json:"reaction"`
}

func toQuoteJSON(q message.Quote, ok bool) *quoteJSON {
	if !ok {
		return nil
	}

	return &quoteJSON{
		ID: q.ID, SenderID: q.SenderID, Text: q.Text.String(), Photos: q.Photos, Forwarded: q.Forwarded, Documents: q.Documents,
		Sticker: q.Sticker.String(), Post: q.Post, SharedPhoto: q.SharedPhoto, Deleted: q.Deleted, Found: q.Found,
	}
}

// quote rebuilds the quote of an answer, checked as any input is.
func (q *quoteJSON) quote() (message.Quote, error) {
	if !q.Found {
		return message.Lost(q.ID), nil
	}

	text, err := message.NewText(q.Text)
	if err != nil {
		return message.Quote{}, errs.Wrap("message.NewText", err)
	}

	return message.Quote{
		ID: q.ID, SenderID: q.SenderID, Text: text, Photos: q.Photos, Forwarded: q.Forwarded, Documents: q.Documents,
		Sticker: message.Sticker(q.Sticker), Post: q.Post, SharedPhoto: q.SharedPhoto, Deleted: q.Deleted, Found: true,
	}, nil
}

func toReactionsJSON(reactions message.Reactions) []reactionJSON {
	out := make([]reactionJSON, len(reactions))
	for i, r := range reactions {
		out[i] = reactionJSON{UserID: r.UserID, Reaction: string(r.Reaction)}
	}

	return out
}

func reactionsOf(raw []reactionJSON) (message.Reactions, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	out := make(message.Reactions, len(raw))
	for i, r := range raw {
		reaction, err := message.NewReaction(r.Reaction)
		if err != nil {
			return nil, errs.Wrap("message.NewReaction", err)
		}

		out[i] = message.Reacted{UserID: r.UserID, Reaction: reaction}
	}

	return out, nil
}

// dressed is what the answer and the reactions add to a message rebuilt.
type dressed[M any] interface {
	Quoting(q message.Quote) M
	Reacting(r message.Reactions) M
}

func dress[M dressed[M]](m M, quote *quoteJSON, raw []reactionJSON) (M, error) {
	if quote != nil {
		q, err := quote.quote()
		if err != nil {
			return m, err
		}

		m = m.Quoting(q)
	}

	reactions, err := reactionsOf(raw)
	if err != nil {
		return m, err
	}

	return m.Reacting(reactions), nil
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
		Sticker:     m.Sticker().String(),
		Documents:   m.Documents(),
		ReplyTo:     m.ReplyTo(),
		Quote:       toQuoteJSON(m.Quote()),
		Reactions:   toReactionsJSON(m.Reactions()),
	}
}

var errUnknownAction = errors.New("unknown chat action")

// message rebuilds the message of a chat event. It comes from Redis, so
// it is checked as any input is.
func (m *chatMessageJSON) message() (*message.ChatMessage, error) {
	switch message.Action(m.Action) {
	case message.ActionNone, message.ActionCreate, message.ActionTitle, message.ActionInvite,
		message.ActionKick, message.ActionLeave, message.ActionReturn, message.ActionPin, message.ActionUnpin,
		message.ActionPhoto, message.ActionPhotoRemove:
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

	content, err := contentOf(m.Text, m.Photos, m.Post, m.SharedPhoto, m.Forwarded, m.Sticker)
	if content, err = documented(content, err, m.Documents); err != nil {
		return nil, errs.Wrap("contentOf", err)
	}

	stored.Content = content.Answering(m.ReplyTo)

	shown, err := dress(message.RestoreChatMessage(stored), m.Quote, m.Reactions)
	if err != nil {
		return nil, errs.Wrap("dress", err)
	}

	return shown, nil
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
	Sealed    []byte         `json:"sealed,omitempty"`
	Sticker   string         `json:"sticker,omitempty"`
	Documents []uuid.UUID    `json:"documents,omitempty"`
	ReplyTo   uuid.UUID      `json:"reply_to"`
	Quote     *quoteJSON     `json:"quote,omitempty"`
	Reactions []reactionJSON `json:"reactions,omitempty"`
}

type forwardedJSON struct {
	SenderID  uuid.UUID   `json:"sender_id"`
	Text      string      `json:"text"`
	Photos    []uuid.UUID `json:"photos,omitempty"`
	Post      uuid.UUID   `json:"post"`
	Photo     uuid.UUID   `json:"shared_photo"`
	CreatedAt time.Time   `json:"created_at"`
	Depth     int         `json:"depth,omitempty"`
	Sticker   string      `json:"sticker,omitempty"`
	Documents []uuid.UUID `json:"documents,omitempty"`
}

func toForwardedJSON(carried message.Forwards) []forwardedJSON {
	forwarded := make([]forwardedJSON, len(carried))
	for i, f := range carried {
		forwarded[i] = forwardedJSON{
			SenderID: f.SenderID, Text: f.Text.String(), Photos: f.Photos, Post: f.Post, Photo: f.SharedPhoto, CreatedAt: f.CreatedAt, Depth: f.Depth,
			Sticker: f.Sticker.String(), Documents: f.Documents,
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
		Sticker:     m.Sticker().String(),
		Documents:   m.Documents(),
		ReplyTo:     m.ReplyTo(),
		Quote:       toQuoteJSON(m.Quote()),
		Reactions:   toReactionsJSON(m.Reactions()),
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

	content, err := sealedContentOf(sealed, m.Photos, m.Post, m.SharedPhoto, m.Forwarded, m.Sticker)
	if content, err = documented(content, err, m.Documents); err != nil {
		return nil, errs.Wrap("sealedContentOf", err)
	}

	stored.Content = content.Answering(m.ReplyTo)

	shown, err := dress(message.Restore(stored), m.Quote, m.Reactions)
	if err != nil {
		return nil, errs.Wrap("dress", err)
	}

	return shown, nil
}

// sealedContentOf rebuilds what a message of a dialog says and shows,
// checking it as a new one.
func sealedContentOf(sealed message.Sealed, photoIDs []uuid.UUID, post, sharedPhoto uuid.UUID, carried []forwardedJSON, sticker string) (message.Content, error) {
	if sticker != "" {
		return stickerContentOf(sticker)
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
		content, err := message.NewSealedPhotoShare(sealed, sharedPhoto)

		return content, errs.Wrap("message.NewSealedPhotoShare", err)
	}

	content, err := message.NewSealedContent(sealed, photos, post, forwarded)

	return content, errs.Wrap("message.NewSealedContent", err)
}

// contentOf rebuilds what a message of a chat says and shows, checking it
// as a new one.
func contentOf(said string, photoIDs []uuid.UUID, post, sharedPhoto uuid.UUID, carried []forwardedJSON, sticker string) (message.Content, error) {
	if sticker != "" {
		return stickerContentOf(sticker)
	}

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

// documented is content, or the error of making it, with the documents
// of a message: with them it may say nothing else.
func documented(content message.Content, err error, ids []uuid.UUID) (message.Content, error) {
	if len(ids) == 0 {
		return content, err
	}

	documents, derr := message.NewDocuments(ids)
	if derr != nil {
		return message.Content{}, errs.Wrap("message.NewDocuments", derr)
	}

	if errors.Is(err, message.ErrTextEmpty) {
		content, err = message.NewDocumentContent(documents)
		return content, errs.Wrap("message.NewDocumentContent", err)
	}

	if err != nil {
		return message.Content{}, err
	}

	content, err = content.Attaching(documents)

	return content, errs.Wrap("content.Attaching", err)
}

// stickerContentOf rebuilds a message that is a sticker.
func stickerContentOf(name string) (message.Content, error) {
	sticker, err := message.NewSticker(name)
	if err != nil {
		return message.Content{}, errs.Wrap("message.NewSticker", err)
	}

	content, err := message.NewStickerContent(sticker)

	return content, errs.Wrap("message.NewStickerContent", err)
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

		if f.Sticker != "" {
			if carried[i].Sticker, err = message.NewSticker(f.Sticker); err != nil {
				return nil, errs.Wrap("message.NewSticker", err)
			}
		}

		if carried[i].Documents, err = message.NewDocuments(f.Documents); err != nil {
			return nil, errs.Wrap("message.NewDocuments", err)
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
	case e.Typing != nil:
		ej.Typing = &typingJSON{UserID: e.Typing.UserID, ChatID: e.Typing.ChatID}
	case e.Call != nil:
		ej.Call = &callJSON{PeerID: e.Call.PeerID, Sealed: e.Call.Sealed}
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
	case ej.Typing != nil:
		return ej.UserID, service.Event{Typing: &service.Typing{UserID: ej.Typing.UserID, ChatID: ej.Typing.ChatID}}, nil
	case ej.Call != nil:
		return ej.UserID, service.Event{Call: &service.CallSignal{PeerID: ej.Call.PeerID, Sealed: ej.Call.Sealed}}, nil
	default:
		return uuid.Nil, service.Event{}, errNoEvent
	}
}
