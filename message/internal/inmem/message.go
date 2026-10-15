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

	keys map[uuid.UUID]message.Key

	// Chats of several people: see chat.go.
	chats        map[uuid.UUID]*message.Chat
	chatMembers  map[uuid.UUID][]*message.Member
	chatMessages map[uuid.UUID]*message.ChatMessage

	// Reactions to messages of dialogs and of chats: see reaction.go.
	reactions     map[uuid.UUID]message.Reactions
	chatReactions map[uuid.UUID]message.Reactions
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
		keys:     make(map[uuid.UUID]message.Key),

		chats:        make(map[uuid.UUID]*message.Chat),
		chatMembers:  make(map[uuid.UUID][]*message.Member),
		chatMessages: make(map[uuid.UUID]*message.ChatMessage),

		reactions:     make(map[uuid.UUID]message.Reactions),
		chatReactions: make(map[uuid.UUID]message.Reactions),
	}
}

func (s *Store) Messages() *MessageRepository {
	return &MessageRepository{s: s}
}

func (s *Store) Dialogs() *DialogRepository {
	return &DialogRepository{s: s}
}

func (s *Store) Keys() *KeyRepository {
	return &KeyRepository{s: s}
}

// KeyRepository keeps the users' keys as message_keys does.
type KeyRepository struct {
	s *Store
}

func (r *KeyRepository) Get(ctx context.Context, userID uuid.UUID) (message.Key, error) {
	if err := ctx.Err(); err != nil {
		return "", errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	key, ok := r.s.keys[userID]
	if !ok {
		return "", message.ErrNoKey
	}

	return key, nil
}

func (r *KeyRepository) Set(ctx context.Context, userID uuid.UUID, key message.Key) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	r.s.keys[userID] = key

	return nil
}

func (r *KeyRepository) Delete(ctx context.Context, userID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	delete(r.s.keys, userID)

	return nil
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

// Messages are immutable, so sharing pointers with callers is safe: an
// edited or hidden one is a new value.
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

func (r *MessageRepository) GetMany(ctx context.Context, ids []uuid.UUID) ([]*message.Message, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	var out []*message.Message
	for _, id := range ids {
		if m, ok := r.s.messages[id]; ok {
			out = append(out, m)
		}
	}

	return out, nil
}

// Lock is Get: nothing is held in memory, where there are no transactions.
func (r *MessageRepository) Lock(ctx context.Context, id uuid.UUID) (*message.Message, error) {
	return r.Get(ctx, id)
}

func (r *MessageRepository) Update(ctx context.Context, m *message.Message) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	if _, ok := r.s.messages[m.ID()]; !ok {
		return message.ErrNotFound
	}

	r.s.messages[m.ID()] = m

	return nil
}

// Retract keeps the message where it was: the dialogs still point at it.
func (r *MessageRepository) Retract(ctx context.Context, m *message.Message) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	if stored, ok := r.s.messages[m.ID()]; !ok || stored.IsDeleted() {
		return message.ErrNotFound
	}

	r.s.messages[m.ID()] = m

	return nil
}

// Hide brings the side of userID back to the newest message it still
// has, if the hidden one was its last, as the statement does in
// PostgreSQL.
func (r *MessageRepository) Hide(ctx context.Context, m *message.Message, userID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	stored, ok := r.s.messages[m.ID()]
	if !ok {
		return message.ErrNotFound
	}

	hidden, err := stored.HiddenFrom(userID)
	if err != nil {
		return errs.Wrap("stored.HiddenFrom", err)
	}

	r.s.messages[m.ID()] = hidden
	r.s.settle(m, userID)

	return nil
}

// settle brings the side of userID in the dialog of m back to the newest
// messages it has, if m was its last; the caller holds the write lock.
func (s *Store) settle(m *message.Message, userID uuid.UUID) {
	key := side{userID, m.SenderID()}
	if userID == m.SenderID() {
		key.peerID = m.RecipientID()
	}

	d := s.dialogs[key]
	if d == nil {
		return
	}

	if d.lastMessageID == m.ID() {
		d.lastMessageID = s.newest(key, func(*message.Message) bool { return true })
	}
	if d.lastIncomingID == m.ID() {
		d.lastIncomingID = s.newest(key, func(m *message.Message) bool { return m.SenderID() == key.peerID })
	}
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

// between returns the messages of a and b that a has and that match,
// oldest first.
func (r *MessageRepository) between(ctx context.Context, a, b uuid.UUID, match func(uuid.UUID) bool) ([]*message.Message, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	var found []*message.Message
	for id, m := range r.s.messages {
		pair := (m.SenderID() == a && m.RecipientID() == b) || (m.SenderID() == b && m.RecipientID() == a)
		if pair && m.VisibleTo(a) && match(id) {
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

// Delete brings a side back to the newest message left only if the
// deleted one was its last, as the statement does in PostgreSQL. A side
// with no messages left keeps its read mark and drops out of the list.
func (r *MessageRepository) Delete(ctx context.Context, m *message.Message) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	if _, ok := r.s.messages[m.ID()]; !ok {
		return message.ErrNotFound
	}

	delete(r.s.messages, m.ID())
	r.s.settle(m, m.SenderID())
	r.s.settle(m, m.RecipientID())

	return nil
}

// newest is the ID of the newest message of a dialog that its user has
// and that matches, uuid.Nil if none; the caller holds a lock.
func (s *Store) newest(key side, match func(*message.Message) bool) uuid.UUID {
	var id uuid.UUID

	for _, m := range s.messages {
		pair := (m.SenderID() == key.userID && m.RecipientID() == key.peerID) || (m.SenderID() == key.peerID && m.RecipientID() == key.userID)
		if pair && m.VisibleTo(key.userID) && match(m) {
			id = later(id, m.ID())
		}
	}

	return id
}

func (r *MessageRepository) DeleteByUser(ctx context.Context, userID uuid.UUID) ([]*message.Message, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	var withPhotos []*message.Message
	for id, m := range r.s.messages {
		if m.SenderID() == userID || m.RecipientID() == userID {
			delete(r.s.messages, id)
			if len(m.Photos()) > 0 || len(m.Forwarded()) > 0 {
				withPhotos = append(withPhotos, m)
			}
		}
	}

	for key := range r.s.dialogs {
		if key.userID == userID || key.peerID == userID {
			delete(r.s.dialogs, key)
		}
	}

	return withPhotos, nil
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
		// A dialog whose messages were all deleted has no last one.
		last := r.s.messages[d.lastMessageID]
		if key.userID != userID || last == nil || !below(d.lastMessageID, before) {
			continue
		}

		found = append(found, message.Dialog{
			PeerID:         key.peerID,
			LastMessage:    last,
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
