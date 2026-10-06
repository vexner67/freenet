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

const maxTextLen = 4096

var (
	ErrTextEmpty   = errors.New("message text is empty")
	ErrTextTooLong = errors.New("message text is too long")
	ErrToYourself  = errors.New("you cannot write to yourself")
	ErrNotFound    = errors.New("message not found")
)

// Text is what a message says: trimmed, with "\n" line breaks.
type Text string

func NewText(s string) (Text, error) {
	// Normalise line breaks so length and storage do not depend on the client.
	s = strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))

	if s == "" {
		return "", ErrTextEmpty
	}

	if utf8.RuneCountInString(s) > maxTextLen {
		return "", ErrTextTooLong
	}

	return Text(s), nil
}

func (t Text) String() string {
	return string(t)
}

type Message struct {
	id          uuid.UUID
	senderID    uuid.UUID
	recipientID uuid.UUID
	text        Text
	createdAt   time.Time
}

// New creates a message. Its UUIDv7 ID grows with time, so a dialog is
// ordered and paged by ID, and "read up to" is one ID.
func New(senderID, recipientID uuid.UUID, text Text, now time.Time) (*Message, error) {
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
		text:        text,
		// PostgreSQL keeps microseconds; trim now so a message reads back equal.
		createdAt: now.UTC().Truncate(time.Microsecond),
	}, nil
}

// Restore rebuilds a message from storage without validation or new IDs.
func Restore(id, senderID, recipientID uuid.UUID, text Text, createdAt time.Time) *Message {
	return &Message{
		id:          id,
		senderID:    senderID,
		recipientID: recipientID,
		text:        text,
		createdAt:   createdAt,
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
	return m.text
}

func (m *Message) CreatedAt() time.Time {
	return m.createdAt
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
