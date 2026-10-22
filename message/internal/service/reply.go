package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/message/internal/message"
	"github.com/vexner67/freenet/pkg/errs"
)

// ReactionRepository keeps the reactions to the messages of dialogs or,
// another one, of chats: one of each person to a message.
type ReactionRepository interface {
	// Set puts r as userID's reaction to the message, in place of the one
	// they had; message.ErrNotFound when the message is gone.
	Set(ctx context.Context, messageID, userID uuid.UUID, r message.Reaction, now time.Time) error
	// Remove takes userID's reaction away; nothing to take is fine.
	Remove(ctx context.Context, messageID, userID uuid.UUID) error
	// Of returns the reactions to each of the messages that has any, the
	// oldest first.
	Of(ctx context.Context, messageIDs []uuid.UUID) (map[uuid.UUID]message.Reactions, error)
	// Forget takes away every reaction of a user whose page is erased.
	Forget(ctx context.Context, userID uuid.UUID) error
}

// shownIDs are the IDs of messages that say something: what was taken
// back has no reactions to read.
func shownIDs[M interface {
	ID() uuid.UUID
	IsDeleted() bool
}](messages []M) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(messages))
	for _, m := range messages {
		if !m.IsDeleted() {
			ids = append(ids, m.ID())
		}
	}

	return ids
}

// answered are the distinct messages those of a page answer.
func answered[M interface{ ReplyTo() uuid.UUID }](messages []M) []uuid.UUID {
	seen := make(map[uuid.UUID]bool)
	ids := make([]uuid.UUID, 0)

	for _, m := range messages {
		if id := m.ReplyTo(); id != uuid.Nil && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}

	return ids
}

// dressed are messages of a dialog as a page shows them: with the quotes
// of the messages they answer and the reactions to them.
func (s *MessageService) dressed(ctx context.Context, messages []*message.Message) ([]*message.Message, error) {
	if len(messages) == 0 {
		return messages, nil
	}

	reactions, err := s.reactions.Of(ctx, shownIDs(messages))
	if err != nil {
		return nil, errs.Wrap("s.reactions.Of", err)
	}

	quoted, err := s.messageRepo.GetMany(ctx, answered(messages))
	if err != nil {
		return nil, errs.Wrap("s.messageRepo.GetMany", err)
	}

	byID := make(map[uuid.UUID]*message.Message, len(quoted))
	for _, q := range quoted {
		byID[q.ID()] = q
	}

	out := make([]*message.Message, len(messages))
	for i, m := range messages {
		m = m.Reacting(reactions[m.ID()])

		if id := m.ReplyTo(); id != uuid.Nil {
			// The message answered is of the same dialog: the answer was
			// checked when sent.
			if q := byID[id]; q != nil && q.Between(m.SenderID()) && q.Between(m.RecipientID()) {
				m = m.Quoting(message.QuoteOf(q))
			} else {
				m = m.Quoting(message.Lost(id))
			}
		}

		out[i] = m
	}

	return out, nil
}

// dressedOne is dressed for one message.
func (s *MessageService) dressedOne(ctx context.Context, m *message.Message) (*message.Message, error) {
	out, err := s.dressed(ctx, []*message.Message{m})
	if err != nil {
		return nil, err
	}

	return out[0], nil
}

// answering is the content of a draft answering a message of the dialog
// of senderID with recipientID, and the quote it shows; c alone when it
// answers none.
func (s *MessageService) answering(ctx context.Context, senderID, recipientID uuid.UUID, d Draft, c message.Content) (message.Content, *message.Quote, error) {
	if d.ReplyTo == uuid.Nil {
		return c, nil, nil
	}

	m, err := s.messageRepo.Get(ctx, d.ReplyTo)
	if errors.Is(err, message.ErrNotFound) {
		return message.Content{}, nil, message.ErrReplyNotFound
	}
	if err != nil {
		return message.Content{}, nil, errs.Wrap("s.messageRepo.Get", err)
	}

	if !m.Between(recipientID) {
		return message.Content{}, nil, message.ErrReplyNotFound
	}

	if err = m.CheckAnswer(senderID); err != nil {
		return message.Content{}, nil, errs.Wrap("m.CheckAnswer", err)
	}

	q := message.QuoteOf(m)

	return c.Answering(m.ID()), &q, nil
}

// React puts userID's reaction r to message id of their dialog, or takes
// theirs away for "". Both sides that have the message hear what it is
// now, as of an edit.
func (s *MessageService) React(ctx context.Context, userID, id uuid.UUID, r message.Reaction) (*message.Message, error) {
	m, err := s.messageRepo.Get(ctx, id)
	if err != nil {
		return nil, errs.Wrap("s.messageRepo.Get", err)
	}

	if err = m.CheckReact(userID); err != nil {
		return nil, errs.Wrap("m.CheckReact", err)
	}

	if r == "" {
		err = errs.Wrap("s.reactions.Remove", s.reactions.Remove(ctx, id, userID))
	} else {
		err = errs.Wrap("s.reactions.Set", s.reactions.Set(ctx, id, userID, r, s.now()))
	}
	if err != nil {
		return nil, err
	}

	if m, err = s.dressedOne(ctx, m); err != nil {
		return nil, errs.Wrap("s.dressedOne", err)
	}

	for _, side := range m.Sides() {
		if m.VisibleTo(side) {
			s.events.Publish(ctx, side, Event{Edited: m})
		}
	}

	return m, nil
}

// dressed are messages of a chat as a page shows them, as
// MessageService.dressed: a message answered is of the same chat.
func (s *ChatService) dressed(ctx context.Context, messages []*message.ChatMessage) ([]*message.ChatMessage, error) {
	if len(messages) == 0 {
		return messages, nil
	}

	reactions, err := s.reactions.Of(ctx, shownIDs(messages))
	if err != nil {
		return nil, errs.Wrap("s.reactions.Of", err)
	}

	quoted, err := s.messageRepo.GetMany(ctx, answered(messages))
	if err != nil {
		return nil, errs.Wrap("s.messageRepo.GetMany", err)
	}

	byID := make(map[uuid.UUID]*message.ChatMessage, len(quoted))
	for _, q := range quoted {
		byID[q.ID()] = q
	}

	out := make([]*message.ChatMessage, len(messages))
	for i, m := range messages {
		m = m.Reacting(reactions[m.ID()])

		if id := m.ReplyTo(); id != uuid.Nil {
			if q := byID[id]; q != nil && q.ChatID() == m.ChatID() {
				m = m.Quoting(message.ChatQuoteOf(q))
			} else {
				m = m.Quoting(message.Lost(id))
			}
		}

		out[i] = m
	}

	return out, nil
}

func (s *ChatService) dressedOne(ctx context.Context, m *message.ChatMessage) (*message.ChatMessage, error) {
	out, err := s.dressed(ctx, []*message.ChatMessage{m})
	if err != nil {
		return nil, err
	}

	return out[0], nil
}

// chatAnswering is the content of a draft answering a message of chatID,
// which senderID is in, and the quote it shows; c alone when it answers
// none.
func (s *ChatService) chatAnswering(ctx context.Context, chatID, senderID uuid.UUID, d Draft, c message.Content) (message.Content, *message.Quote, error) {
	if d.ReplyTo == uuid.Nil {
		return c, nil, nil
	}

	me, err := s.member(ctx, chatID, senderID)
	if err != nil {
		return message.Content{}, nil, errs.Wrap("s.member", err)
	}

	m, err := s.messageRepo.Get(ctx, d.ReplyTo)
	if errors.Is(err, message.ErrNotFound) {
		return message.Content{}, nil, message.ErrReplyNotFound
	}
	if err != nil {
		return message.Content{}, nil, errs.Wrap("s.messageRepo.Get", err)
	}

	if err = m.CheckAnswer(chatID, me); err != nil {
		return message.Content{}, nil, errs.Wrap("m.CheckAnswer", err)
	}

	q := message.ChatQuoteOf(m)

	return c.Answering(m.ID()), &q, nil
}

// React puts userID's reaction r to message id of a chat they are in, or
// takes theirs away for "". Everyone in the chat hears what it is now.
func (s *ChatService) React(ctx context.Context, userID, id uuid.UUID, r message.Reaction) (*message.ChatMessage, error) {
	m, me, err := s.shown(ctx, userID, id)
	if err != nil {
		return nil, errs.Wrap("s.shown", err)
	}

	if err = m.CheckReact(me); err != nil {
		return nil, errs.Wrap("m.CheckReact", err)
	}

	if r == "" {
		err = errs.Wrap("s.reactions.Remove", s.reactions.Remove(ctx, id, userID))
	} else {
		err = errs.Wrap("s.reactions.Set", s.reactions.Set(ctx, id, userID, r, s.now()))
	}
	if err != nil {
		return nil, err
	}

	if m, err = s.dressedOne(ctx, m); err != nil {
		return nil, errs.Wrap("s.dressedOne", err)
	}

	s.tell(ctx, Event{Chat: &ChatEvent{Edited: m}})

	return m, nil
}
