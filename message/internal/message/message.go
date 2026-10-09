// Package message is the domain of private messages: what two people
// write to each other and how far each of them has read.
package message

import (
	"bytes"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	// maxTextLen is as long as a post of the wall may be, openvk's 60000;
	// openvk itself does not limit a message at all.
	maxTextLen = 60000
	maxPhotos  = 10

	// deleteWindow is how long the sender may take a message back from
	// both sides, as "delete for everyone" in VK: long enough to take back
	// a message sent in haste, short enough that the peer can rely on what
	// was said before.
	deleteWindow = 24 * time.Hour
)

var (
	// ErrTextEmpty: neither text nor photos. The message keeps the words
	// it had before messages took photos: clients know it.
	ErrTextEmpty      = errors.New("message text is empty")
	ErrTextTooLong    = errors.New("message text is too long")
	ErrToYourself     = errors.New("you cannot write to yourself")
	ErrNotFound       = errors.New("message not found")
	ErrTooManyPhotos  = errors.New("a message has at most 10 photos")
	ErrDuplicatePhoto = errors.New("the same photo is attached twice")
	ErrInvalidPhoto   = errors.New("invalid photo id")
	ErrCannotDelete   = errors.New("only the sender can delete the message")
	ErrDeleteExpired  = errors.New("the message can no longer be deleted")
)

// Text is what a message says: trimmed, with "\n" line breaks. It may be
// empty when the message has photos (see NewContent).
type Text string

func NewText(s string) (Text, error) {
	// Normalise line breaks so length and storage do not depend on the client.
	s = strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))

	if utf8.RuneCountInString(s) > maxTextLen {
		return "", ErrTextTooLong
	}

	return Text(s), nil
}

func (t Text) String() string {
	return string(t)
}

// Photos are the IDs of a message's photos in the media service, in the
// order they are shown. Whose photos they are the gateway checks with the
// media service when the message is sent.
type Photos []uuid.UUID

func NewPhotos(ids []uuid.UUID) (Photos, error) {
	if len(ids) > maxPhotos {
		return nil, ErrTooManyPhotos
	}

	seen := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		if id == uuid.Nil {
			return nil, ErrInvalidPhoto
		}
		if seen[id] {
			return nil, ErrDuplicatePhoto
		}
		seen[id] = true
	}

	if len(ids) == 0 {
		return nil, nil
	}

	return Photos(ids), nil
}

// Content is what a message shows: text, photos, a post shared into the
// dialog (openvk's "Поделиться" "в переписку") or some of them, never
// nothing. The post lives in the post service: the message keeps its ID,
// and a deleted post stays "Запись удалена.".
type Content struct {
	text   Text
	photos Photos
	post   uuid.UUID
}

// NewContent: post is uuid.Nil for none; whether it exists the caller has
// checked with the post service.
func NewContent(text Text, photos Photos, post uuid.UUID) (Content, error) {
	if text == "" && len(photos) == 0 && post == uuid.Nil {
		return Content{}, ErrTextEmpty
	}

	return Content{text: text, photos: photos, post: post}, nil
}

// RestoreContent rebuilds content from storage without validation.
func RestoreContent(text Text, photos Photos, post uuid.UUID) Content {
	return Content{text: text, photos: photos, post: post}
}

// Post is the post shared into the dialog, uuid.Nil for none.
func (c Content) Post() uuid.UUID {
	return c.post
}

func (c Content) Text() Text {
	return c.text
}

func (c Content) Photos() Photos {
	return c.photos
}

type Message struct {
	id          uuid.UUID
	senderID    uuid.UUID
	recipientID uuid.UUID
	content     Content
	createdAt   time.Time
}

// New creates a message. Its UUIDv7 ID grows with time, so a dialog is
// ordered and paged by ID, and "read up to" is one ID.
func New(senderID, recipientID uuid.UUID, content Content, now time.Time) (*Message, error) {
	if senderID == recipientID {
		return nil, ErrToYourself
	}

	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	return &Message{
		id:          id,
		senderID:    senderID,
		recipientID: recipientID,
		content:     content,
		// PostgreSQL keeps microseconds; trim now so a message reads back equal.
		createdAt: now.UTC().Truncate(time.Microsecond),
	}, nil
}

// Stored is a message as storage keeps it.
type Stored struct {
	ID          uuid.UUID
	SenderID    uuid.UUID
	RecipientID uuid.UUID
	Content     Content
	CreatedAt   time.Time
}

// Restore rebuilds a message from storage without validation or new IDs.
func Restore(s Stored) *Message {
	return &Message{
		id:          s.ID,
		senderID:    s.SenderID,
		recipientID: s.RecipientID,
		content:     s.Content,
		createdAt:   s.CreatedAt,
	}
}

func (m *Message) ID() uuid.UUID {
	return m.id
}

func (m *Message) SenderID() uuid.UUID {
	return m.senderID
}

func (m *Message) RecipientID() uuid.UUID {
	return m.recipientID
}

func (m *Message) Text() Text {
	return m.content.text
}

func (m *Message) Photos() Photos {
	return m.content.photos
}

// Post is the post shared in the message, uuid.Nil for none.
func (m *Message) Post() uuid.UUID {
	return m.content.post
}

func (m *Message) CreatedAt() time.Time {
	return m.createdAt
}

// DeletableUntil is when the sender can no longer delete the message.
func (m *Message) DeletableUntil() time.Time {
	return m.createdAt.Add(deleteWindow)
}

// CheckDelete tells whether requesterID may delete the message, from both
// sides, at now: only its sender, for deleteWindow after sending.
func (m *Message) CheckDelete(requesterID uuid.UUID, now time.Time) error {
	if requesterID != m.senderID {
		return ErrCannotDelete
	}

	if now.After(m.DeletableUntil()) {
		return ErrDeleteExpired
	}

	return nil
}

// ReadMarks are how far each side of a dialog has read: Mine is the newest
// message from the peer I have read, Theirs the newest of mine the peer
// has read; uuid.Nil is nothing read.
type ReadMarks struct {
	Mine   uuid.UUID
	Theirs uuid.UUID
}

// IsRead tells whether m has been read by its recipient; me is the user
// whose dialog this is.
func (r ReadMarks) IsRead(m *Message, me uuid.UUID) bool {
	mark := r.Mine
	if m.SenderID() == me {
		mark = r.Theirs
	}

	id := m.ID()

	return bytes.Compare(id[:], mark[:]) <= 0
}

// Dialog is one user's view of their conversation with PeerID.
type Dialog struct {
	PeerID      uuid.UUID
	LastMessage *Message
	// LastIncomingID is the newest message from the peer; uuid.Nil if the
	// peer has not written yet.
	LastIncomingID uuid.UUID
	Marks          ReadMarks
}

// HasUnread tells whether the peer wrote something the user has not read.
func (d Dialog) HasUnread() bool {
	return bytes.Compare(d.LastIncomingID[:], d.Marks.Mine[:]) > 0
}
