package inmem

import (
	"bytes"
	"context"
	"slices"
	"sync"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/message/internal/message"
	"github.com/vexner67/freenet/pkg/errs"
)

// Store keeps messages and both sides' dialogs together, so saving a
// message updates the dialogs at once, as the single statement does in
// PostgreSQL. Its repositories are Messages and Dialogs.
type Store struct {
	mu       sync.RWMutex
	messages map[uuid.UUID]*message.Message
	dialogs  map[side]*dialogState
}

// side is one user's view of a dialog.
type side struct {
	userID, peerID uuid.UUID
}

type dialogState struct {
	lastMessageID  uuid.UUID
	lastIncomingID uuid.UUID
	readUpTo       uuid.UUID
}

func NewStore() *Store {
	return &Store{
		messages: make(map[uuid.UUID]*message.Message),
		dialogs:  make(map[side]*dialogState),
	}
}

func (s *Store) Messages() *MessageRepository {
	return &MessageRepository{s: s}
}

func (s *Store) Dialogs() *DialogRepository {
	return &DialogRepository{s: s}
}

// dialog returns the state of a side, creating it; the caller holds the
// write lock.
func (s *Store) dialog(userID, peerID uuid.UUID) *dialogState {
	key := side{userID, peerID}
	if s.dialogs[key] == nil {
		s.dialogs[key] = &dialogState{}
	}

	return s.dialogs[key]
}

func later(a, b uuid.UUID) uuid.UUID {
	if bytes.Compare(a[:], b[:]) > 0 {
		return a
	}

	return b
}

func below(id, bound uuid.UUID) bool {
	return bound == uuid.Nil || bytes.Compare(id[:], bound[:]) < 0
}

type MessageRepository struct {
	s *Store
}

// Messages are immutable, so sharing pointers with callers is safe.
func (r *MessageRepository) Save(ctx context.Context, m *message.Message) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	r.s.messages[m.ID()] = m

	sent := r.s.dialog(m.SenderID(), m.RecipientID())
	sent.lastMessageID = later(sent.lastMessageID, m.ID())

	received := r.s.dialog(m.RecipientID(), m.SenderID())
	received.lastMessageID = later(received.lastMessageID, m.ID())
	received.lastIncomingID = later(received.lastIncomingID, m.ID())

	return nil
}

func (r *MessageRepository) Get(ctx context.Context, id uuid.UUID) (*message.Message, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	m, ok := r.s.messages[id]
	if !ok {
		return nil, message.ErrNotFound
	}

	return m, nil
}

func (r *MessageRepository) ListBefore(ctx context.Context, a, b, before uuid.UUID, limit int) ([]*message.Message, error) {
	found, err := r.between(ctx, a, b, func(id uuid.UUID) bool { return below(id, before) })
	if err != nil {
		return nil, err
	}

	slices.Reverse(found)

	return found[:min(limit, len(found))], nil
}

func (r *MessageRepository) ListAfter(ctx context.Context, a, b, after uuid.UUID, limit int) ([]*message.Message, error) {
	found, err := r.between(ctx, a, b, func(id uuid.UUID) bool { return bytes.Compare(id[:], after[:]) > 0 })
	if err != nil {
		return nil, err
	}

	return found[:min(limit, len(found))], nil
}

// between returns the messages of a and b that match, oldest first.
func (r *MessageRepository) between(ctx context.Context, a, b uuid.UUID, match func(uuid.UUID) bool) ([]*message.Message, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	var found []*message.Message
	for id, m := range r.s.messages {
		pair := (m.SenderID() == a && m.RecipientID() == b) || (m.SenderID() == b && m.RecipientID() == a)
		if pair && match(id) {
			found = append(found, m)
		}
	}

	// UUIDv7 bytes sort by time, as in PostgreSQL.
	slices.SortFunc(found, func(x, y *message.Message) int {
		xi, yi := x.ID(), y.ID()
		return bytes.Compare(xi[:], yi[:])
	})

	return found, nil
}

func (r *MessageRepository) DeleteByUser(ctx context.Context, userID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	for id, m := range r.s.messages {
		if m.SenderID() == userID || m.RecipientID() == userID {
			delete(r.s.messages, id)
		}
	}

	for key := range r.s.dialogs {
		if key.userID == userID || key.peerID == userID {
			delete(r.s.dialogs, key)
		}
	}

	return nil
}

type DialogRepository struct {
	s *Store
}

func (r *DialogRepository) List(ctx context.Context, userID, before uuid.UUID, limit int) ([]message.Dialog, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	var found []message.Dialog
	for key, d := range r.s.dialogs {
		if key.userID != userID || !below(d.lastMessageID, before) {
			continue
		}

		found = append(found, message.Dialog{
			PeerID:         key.peerID,
			LastMessage:    r.s.messages[d.lastMessageID],
			LastIncomingID: d.lastIncomingID,
			Marks:          r.marks(userID, key.peerID),
		})
	}

	// The most recently written first, as in PostgreSQL.
	slices.SortFunc(found, func(x, y message.Dialog) int {
		xi, yi := x.LastMessage.ID(), y.LastMessage.ID()
		return bytes.Compare(yi[:], xi[:])
	})

	return found[:min(limit, len(found))], nil
}

func (r *DialogRepository) CountUnread(ctx context.Context, userID uuid.UUID, limit int) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	count := 0
	for key, d := range r.s.dialogs {
		if key.userID == userID && bytes.Compare(d.lastIncomingID[:], d.readUpTo[:]) > 0 {
			count++
		}
	}

	return min(count, limit), nil
}

func (r *DialogRepository) ReadMarks(ctx context.Context, userID, peerID uuid.UUID) (message.ReadMarks, error) {
	if err := ctx.Err(); err != nil {
		return message.ReadMarks{}, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	return r.marks(userID, peerID), nil
}

func (r *DialogRepository) HasIncoming(ctx context.Context, userID, peerID uuid.UUID) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	d := r.s.dialogs[side{userID, peerID}]

	return d != nil && d.lastIncomingID != uuid.Nil, nil
}

// marks reads both sides' marks; the caller holds a lock.
func (r *DialogRepository) marks(userID, peerID uuid.UUID) message.ReadMarks {
	var marks message.ReadMarks

	if d := r.s.dialogs[side{userID, peerID}]; d != nil {
		marks.Mine = d.readUpTo
	}
	if d := r.s.dialogs[side{peerID, userID}]; d != nil {
		marks.Theirs = d.readUpTo
	}

	return marks
}

func (r *DialogRepository) MarkRead(ctx context.Context, userID, peerID, upTo uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	d := r.s.dialog(userID, peerID)
	d.readUpTo = later(d.readUpTo, upTo)

	return nil
}
