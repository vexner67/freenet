package inmem

import (
	"bytes"
	"context"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/message/internal/message"
	"github.com/vexner67/freenet/pkg/errs"
)

// The chats of the store: a chat, its people in the order they joined
// and its messages. Saving a message brings everyone in the chat up to
// it at once, as the single statement does in PostgreSQL.

func (s *Store) Chats() *ChatRepository {
	return &ChatRepository{s: s}
}

func (s *Store) ChatMembers() *ChatMemberRepository {
	return &ChatMemberRepository{s: s}
}

func (s *Store) ChatMessages() *ChatMessageRepository {
	return &ChatMessageRepository{s: s}
}

// member is how userID stands in the chat, nil for someone never in it;
// the caller holds the lock.
func (s *Store) member(chatID, userID uuid.UUID) *message.Member {
	for _, m := range s.chatMembers[chatID] {
		if m.UserID == userID {
			return m
		}
	}

	return nil
}

// active counts who is in the chat now; the caller holds the lock.
func (s *Store) active(chatID uuid.UUID) int {
	n := 0

	for _, m := range s.chatMembers[chatID] {
		if m.IsActive() {
			n++
		}
	}

	return n
}

// view is the chat as m has it; the caller holds the lock.
func (s *Store) view(c *message.Chat, m *message.Member) message.ChatView {
	return message.ChatView{Chat: c, Me: *m, Members: s.active(c.ID()), LastMessage: s.chatMessages[m.SeenUpTo]}
}

type ChatRepository struct {
	s *Store
}

func (r *ChatRepository) Create(ctx context.Context, c *message.Chat) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	r.s.chats[c.ID()] = c

	return nil
}

// Lock only reads: the store has no transactions to hold a chat for.
func (r *ChatRepository) Lock(ctx context.Context, id uuid.UUID) (*message.Chat, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	c, ok := r.s.chats[id]
	if !ok {
		return nil, message.ErrChatNotFound
	}

	return c, nil
}

func (r *ChatRepository) Update(ctx context.Context, c *message.Chat) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	if _, ok := r.s.chats[c.ID()]; !ok {
		return message.ErrChatNotFound
	}

	r.s.chats[c.ID()] = c

	return nil
}

func (r *ChatRepository) View(ctx context.Context, chatID, userID uuid.UUID) (message.ChatView, error) {
	if err := ctx.Err(); err != nil {
		return message.ChatView{}, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	c, m := r.s.chats[chatID], r.s.member(chatID, userID)
	if c == nil || m == nil {
		return message.ChatView{}, message.ErrChatNotFound
	}

	return r.s.view(c, m), nil
}

func (r *ChatRepository) List(ctx context.Context, userID, before uuid.UUID, limit int) ([]message.ChatView, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	var views []message.ChatView

	for chatID, c := range r.s.chats {
		m := r.s.member(chatID, userID)
		// A chat with nothing left to show is not listed, as a dialog.
		if m == nil || m.SeenUpTo == uuid.Nil || !below(m.SeenUpTo, before) {
			continue
		}

		views = append(views, r.s.view(c, m))
	}

	slices.SortFunc(views, func(a, b message.ChatView) int {
		return bytes.Compare(b.Me.SeenUpTo[:], a.Me.SeenUpTo[:])
	})

	if len(views) > limit {
		views = views[:limit]
	}

	return views, nil
}

func (r *ChatRepository) CountUnread(ctx context.Context, userID uuid.UUID, limit int) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	count := 0

	for chatID := range r.s.chats {
		if m := r.s.member(chatID, userID); m != nil && m.HasUnread() && count < limit {
			count++
		}
	}

	return count, nil
}

func (r *ChatRepository) DeleteByUser(ctx context.Context, userID uuid.UUID) ([]*message.ChatMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	var carrying []*message.ChatMessage

	for id, m := range r.s.chatMessages {
		if m.SenderID() != userID && m.TargetID() != userID {
			continue
		}

		if len(m.Forwarded()) > 0 {
			carrying = append(carrying, m)
		}

		delete(r.s.chatMessages, id)
	}

	for chatID, c := range r.s.chats {
		members := slices.DeleteFunc(r.s.chatMembers[chatID], func(m *message.Member) bool { return m.UserID == userID })
		r.s.chatMembers[chatID] = members

		for _, m := range members {
			r.s.settleChat(chatID, m)
		}

		// A pinned message gone with its sender is pinned no more, as ON
		// DELETE SET NULL does.
		if _, ok := r.s.chatMessages[c.PinnedID()]; !ok && c.PinnedID() != uuid.Nil {
			c = c.Unpinned()
			r.s.chats[chatID] = c
		}

		if c.OwnerID() != userID {
			continue
		}

		// Whoever has been in the chat longest has it now.
		for _, m := range members {
			if m.IsActive() {
				stored := c.Stored()
				stored.OwnerID = m.UserID
				r.s.chats[chatID] = message.RestoreChat(stored)
				break
			}
		}
	}

	return carrying, nil
}

// settleChat brings m back to the messages the chat has left for them;
// the caller holds the write lock.
func (s *Store) settleChat(chatID uuid.UUID, m *message.Member) {
	seen, incoming := m.SeenUpTo, m.LastIncomingID
	m.SeenUpTo, m.LastIncomingID = uuid.Nil, uuid.Nil

	for id, msg := range s.chatMessages {
		if msg.ChatID() != chatID {
			continue
		}

		if bytes.Compare(id[:], seen[:]) <= 0 {
			m.SeenUpTo = later(m.SeenUpTo, id)
		}

		if msg.SenderID() != m.UserID && bytes.Compare(id[:], incoming[:]) <= 0 {
			m.LastIncomingID = later(m.LastIncomingID, id)
		}
	}
}

type ChatMemberRepository struct {
	s *Store
}

func (r *ChatMemberRepository) Get(ctx context.Context, chatID, userID uuid.UUID) (message.Member, bool, error) {
	if err := ctx.Err(); err != nil {
		return message.Member{}, false, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	m := r.s.member(chatID, userID)
	if m == nil {
		return message.Member{}, false, nil
	}

	return *m, true, nil
}

func (r *ChatMemberRepository) Active(ctx context.Context, chatID uuid.UUID) ([]message.Member, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	var active []message.Member

	for _, m := range r.s.chatMembers[chatID] {
		if m.IsActive() {
			active = append(active, *m)
		}
	}

	return active, nil
}

func (r *ChatMemberRepository) Set(ctx context.Context, chatID, userID uuid.UUID, state message.MemberState, now time.Time) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	if _, ok := r.s.chats[chatID]; !ok {
		return message.ErrChatNotFound
	}

	if m := r.s.member(chatID, userID); m != nil {
		m.State = state
		return nil
	}

	r.s.chatMembers[chatID] = append(r.s.chatMembers[chatID], &message.Member{
		UserID: userID, State: state, JoinedAt: now.UTC().Truncate(time.Microsecond),
	})

	return nil
}

func (r *ChatMemberRepository) MarkRead(ctx context.Context, chatID, userID, upTo uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	if m := r.s.member(chatID, userID); m != nil {
		m.ReadUpTo = later(m.ReadUpTo, upTo)
	}

	return nil
}

type ChatMessageRepository struct {
	s *Store
}

// Messages are immutable, so sharing pointers with callers is safe.
func (r *ChatMessageRepository) Save(ctx context.Context, m *message.ChatMessage) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	r.s.chatMessages[m.ID()] = m

	for _, member := range r.s.chatMembers[m.ChatID()] {
		if !member.IsActive() {
			continue
		}

		member.SeenUpTo = later(member.SeenUpTo, m.ID())
		if member.UserID != m.SenderID() {
			member.LastIncomingID = later(member.LastIncomingID, m.ID())
		}
	}

	return nil
}

func (r *ChatMessageRepository) Get(ctx context.Context, id uuid.UUID) (*message.ChatMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	m, ok := r.s.chatMessages[id]
	if !ok {
		return nil, message.ErrNotFound
	}

	return m, nil
}

func (r *ChatMessageRepository) ListBefore(ctx context.Context, chatID, before, upTo uuid.UUID, limit int) ([]*message.ChatMessage, error) {
	found, err := r.of(ctx, chatID, func(id uuid.UUID) bool {
		return below(id, before) && bytes.Compare(id[:], upTo[:]) <= 0
	})
	if err != nil {
		return nil, errs.Wrap("r.of", err)
	}

	slices.Reverse(found)

	return found[:min(limit, len(found))], nil
}

func (r *ChatMessageRepository) ListAfter(ctx context.Context, chatID, after, upTo uuid.UUID, limit int) ([]*message.ChatMessage, error) {
	found, err := r.of(ctx, chatID, func(id uuid.UUID) bool {
		return bytes.Compare(id[:], after[:]) > 0 && bytes.Compare(id[:], upTo[:]) <= 0
	})
	if err != nil {
		return nil, errs.Wrap("r.of", err)
	}

	return found[:min(limit, len(found))], nil
}

// of are the messages of the chat whose IDs match, oldest first.
func (r *ChatMessageRepository) of(ctx context.Context, chatID uuid.UUID, match func(uuid.UUID) bool) ([]*message.ChatMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	var found []*message.ChatMessage

	for id, m := range r.s.chatMessages {
		if m.ChatID() == chatID && match(id) {
			found = append(found, m)
		}
	}

	slices.SortFunc(found, func(a, b *message.ChatMessage) int {
		x, y := a.ID(), b.ID()
		return bytes.Compare(x[:], y[:])
	})

	return found, nil
}

func (r *ChatMessageRepository) Search(ctx context.Context, userID, chatID uuid.UUID, q message.Query, before uuid.UUID, limit int) ([]*message.ChatMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	var found []*message.ChatMessage
	for id, m := range r.s.chatMessages {
		if chatID != uuid.Nil && m.ChatID() != chatID {
			continue
		}

		me := r.s.member(m.ChatID(), userID)
		if me != nil && me.Sees(id) && q.MatchesChat(m) && below(id, before) {
			found = append(found, m)
		}
	}

	// Newest first: UUIDv7 bytes sort by time, as in PostgreSQL.
	slices.SortFunc(found, func(x, y *message.ChatMessage) int {
		xi, yi := x.ID(), y.ID()
		return bytes.Compare(yi[:], xi[:])
	})

	return found[:min(limit, len(found))], nil
}

func (r *ChatMessageRepository) Update(ctx context.Context, m *message.ChatMessage) error {
	return r.replace(ctx, m, false)
}

func (r *ChatMessageRepository) Retract(ctx context.Context, m *message.ChatMessage) error {
	return r.replace(ctx, m, true)
}

// replace puts m in place of the message it was; taken back twice is
// message.ErrNotFound.
func (r *ChatMessageRepository) replace(ctx context.Context, m *message.ChatMessage, retract bool) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	old, ok := r.s.chatMessages[m.ID()]
	if !ok || (retract && old.IsDeleted()) {
		return message.ErrNotFound
	}

	r.s.chatMessages[m.ID()] = m

	return nil
}
