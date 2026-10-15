package message

import (
	"errors"

	"github.com/google/uuid"
)

// ErrReplyNotFound: the message answered is not in the dialog or chat of
// the answer, is not shown to whoever answers, was taken back or tells of
// what happened in the chat.
var ErrReplyNotFound = errors.New("replied message not found")

// Answering is c answering message id ("Ответить", as on VK): it shows a
// quote of that message above its own. The message answered is earlier
// in the same dialog or chat; the caller has checked that.
func (c Content) Answering(id uuid.UUID) Content {
	c.replyTo = id
	return c
}

// ReplyTo is the message c answers, uuid.Nil for none.
func (c Content) ReplyTo() uuid.UUID {
	return c.replyTo
}

// Quote is what a message answering another shows of it: who wrote it
// and what it is now, in short. A message of a dialog says its words
// sealed, and the answer carries the words it quotes sealed itself, so
// Text is only a chat's. A message taken back says nothing, and one gone
// for good (deleted by both sides, erased with a page) is not found.
type Quote struct {
	ID       uuid.UUID
	SenderID uuid.UUID
	Text     Text
	// Photos, Forwarded and Documents are how many it shows.
	Photos      int
	Forwarded   int
	Documents   int
	Sticker     Sticker
	Post        bool
	SharedPhoto bool
	Deleted     bool
	// Found is false for a message gone for good: only ID is known.
	Found bool
}

// quoteOf is the quote of what senderID wrote as message id.
func quoteOf(id, senderID uuid.UUID, c Content, deleted bool) Quote {
	return Quote{
		ID:          id,
		SenderID:    senderID,
		Text:        c.text,
		Photos:      len(c.photos),
		Forwarded:   len(c.forwarded),
		Documents:   len(c.documents),
		Sticker:     c.sticker,
		Post:        c.post != uuid.Nil,
		SharedPhoto: c.sharedPhoto != uuid.Nil,
		Deleted:     deleted,
		Found:       true,
	}
}

// QuoteOf is the quote of m.
func QuoteOf(m *Message) Quote {
	return quoteOf(m.id, m.senderID, m.content, m.IsDeleted())
}

// ChatQuoteOf is the quote of m, a message of a chat.
func ChatQuoteOf(m *ChatMessage) Quote {
	return quoteOf(m.id, m.senderID, m.content, m.IsDeleted())
}

// Lost is the quote of message id, gone for good.
func Lost(id uuid.UUID) Quote {
	return Quote{ID: id}
}

// CheckAnswer tells whether userID may answer m in the dialog of the two
// sides of m: they have it and it says something.
func (m *Message) CheckAnswer(userID uuid.UUID) error {
	if !m.VisibleTo(userID) || m.IsDeleted() {
		return ErrReplyNotFound
	}

	return nil
}

// CheckAnswer tells whether a member of chatID may answer m: a message of
// that chat they are shown, written by a person and not taken back.
func (m *ChatMessage) CheckAnswer(chatID uuid.UUID, me Member) error {
	if m.chatID != chatID || !me.Sees(m.id) || m.IsAction() || m.IsDeleted() {
		return ErrReplyNotFound
	}

	return nil
}

// ReplyTo is the message m answers, uuid.Nil for none.
func (m *Message) ReplyTo() uuid.UUID {
	return m.content.replyTo
}

// Quote is what m shows of the message it answers; false when it answers
// none or the quote was not read with it.
func (m *Message) Quote() (Quote, bool) {
	if m.quote == nil {
		return Quote{}, false
	}

	return *m.quote, true
}

// Quoting is m showing q of the message it answers.
func (m *Message) Quoting(q Quote) *Message {
	quoted := *m
	quoted.quote = &q

	return &quoted
}

// ReplyTo is the message m answers, uuid.Nil for none.
func (m *ChatMessage) ReplyTo() uuid.UUID {
	return m.content.replyTo
}

// Quote is what m shows of the message it answers, as Message.Quote.
func (m *ChatMessage) Quote() (Quote, bool) {
	if m.quote == nil {
		return Quote{}, false
	}

	return *m.quote, true
}

// Quoting is m showing q of the message it answers.
func (m *ChatMessage) Quoting(q Quote) *ChatMessage {
	quoted := *m
	quoted.quote = &q

	return &quoted
}
