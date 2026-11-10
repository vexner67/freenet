package inmem

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/room/internal/room"
	"github.com/vexner67/freenet/room/internal/service"
)

type State struct {
	mu, sweeping sync.Mutex
	clock        service.Clock
	ttl          time.Duration
	expires      map[uuid.UUID]time.Time
	members      map[uuid.UUID]map[uuid.UUID]service.Member
	current      map[uuid.UUID]uuid.UUID
	empty        map[uuid.UUID]time.Time
	closed       map[uuid.UUID]bool
	playback     map[uuid.UUID]room.Playback
	chat         map[uuid.UUID][]room.Chat
}

func NewState(clock service.Clock, ttls ...time.Duration) *State {
	ttl := 24 * time.Hour
	if len(ttls) > 0 {
		ttl = ttls[0]
	}
	return &State{clock: clock, ttl: ttl, expires: map[uuid.UUID]time.Time{}, members: map[uuid.UUID]map[uuid.UUID]service.Member{}, current: map[uuid.UUID]uuid.UUID{}, empty: map[uuid.UUID]time.Time{}, closed: map[uuid.UUID]bool{}, playback: map[uuid.UUID]room.Playback{}, chat: map[uuid.UUID][]room.Chat{}}
}
func (s *State) Join(ctx context.Context, id, user uuid.UUID, now time.Time, maxMembers, maxLive int) (service.Transition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune()
	if err := ctx.Err(); err != nil {
		return service.Transition{}, err
	}
	s.touch(id)
	if s.closed[id] {
		return service.Transition{}, room.ErrClosed
	}
	s.touch(id)
	old := s.current[user]
	if old == id {
		m := s.members[id][user]
		m.HeartbeatAt = now
		s.members[id][user] = m
		return service.Transition{}, nil
	}
	if len(s.members[id]) >= maxMembers {
		return service.Transition{}, room.ErrFull
	}
	live := 0
	for _, members := range s.members {
		if len(members) > 0 {
			live++
		}
	}
	// Switching out of a room alone frees its live slot atomically.
	if len(s.members[id]) == 0 && live >= maxLive && (old == uuid.Nil || len(s.members[old]) != 1) {
		return service.Transition{}, room.ErrTooManyRooms
	}
	change := service.Transition{PreviousRoom: old, Joined: true}
	if old != uuid.Nil {
		change.PreviousMember, _ = s.leave(old, user, now)
	}
	if s.members[id] == nil {
		s.members[id] = map[uuid.UUID]service.Member{}
	}
	s.members[id][user] = service.Member{UserID: user, JoinedAt: now, HeartbeatAt: now}
	s.current[user] = id
	delete(s.empty, id)
	return change, nil
}
func (s *State) leave(id, user uuid.UUID, now time.Time) (service.Member, bool) {
	member, ok := s.members[id][user]
	if !ok {
		return service.Member{}, false
	}
	delete(s.members[id], user)
	if s.current[user] == id {
		delete(s.current, user)
	}
	if len(s.members[id]) == 0 {
		s.empty[id] = now
	}
	return member, true
}
func (s *State) Leave(_ context.Context, id, user uuid.UUID, now time.Time) (service.Member, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune()
	s.touch(id)
	m, ok := s.leave(id, user, now)
	return m, ok, nil
}
func (s *State) Expire(_ context.Context, id, user uuid.UUID, before, now time.Time) (service.Member, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune()
	s.touch(id)
	m, ok := s.members[id][user]
	if !ok || m.HeartbeatAt.After(before) {
		return service.Member{}, false, nil
	}
	m, ok = s.leave(id, user, now)
	return m, ok, nil
}
func (s *State) Heartbeat(_ context.Context, id, user uuid.UUID, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune()
	s.touch(id)
	m, ok := s.members[id][user]
	if !ok || s.current[user] != id || !m.HeartbeatAt.Add(service.PresenceTTL).After(now) {
		return room.ErrNotMember
	}
	m.HeartbeatAt = now
	s.members[id][user] = m
	return nil
}
func (s *State) Members(_ context.Context, id uuid.UUID) ([]service.Member, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune()
	rows := make([]service.Member, 0, len(s.members[id]))
	for _, m := range s.members[id] {
		rows = append(rows, m)
	}
	slices.SortFunc(rows, func(a, b service.Member) int { return strings.Compare(a.UserID.String(), b.UserID.String()) })
	return rows, nil
}
func (s *State) Current(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]uuid.UUID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune()
	result := make(map[uuid.UUID]uuid.UUID)
	for _, id := range ids {
		rid := s.current[id]
		if m, ok := s.members[rid][id]; ok && m.HeartbeatAt.Add(service.PresenceTTL).After(s.clock.Now()) {
			result[id] = rid
		}
	}
	return result, nil
}
func (s *State) Live(_ context.Context) ([]uuid.UUID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune()
	result := make([]uuid.UUID, 0)
	for id, m := range s.members {
		if len(m) > 0 {
			result = append(result, id)
		}
	}
	slices.SortFunc(result, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
	return result, nil
}
func (s *State) EmptySince(_ context.Context, id uuid.UUID) (time.Time, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune()
	return s.empty[id], nil
}
func (s *State) Close(_ context.Context, id uuid.UUID, now time.Time) ([]service.Member, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune()
	s.touch(id)
	members := make([]service.Member, 0, len(s.members[id]))
	for _, m := range s.members[id] {
		members = append(members, m)
		s.leave(id, m.UserID, now)
	}
	s.closed[id] = true
	delete(s.playback, id)
	delete(s.chat, id)
	return members, nil
}
func (s *State) Playback(_ context.Context, id uuid.UUID) (room.Playback, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune()
	return s.playback[id], nil
}
func (s *State) SetPlayback(_ context.Context, id, actor uuid.UUID, version uint64, p room.Playback) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune()
	s.touch(id)
	if s.closed[id] {
		return room.ErrClosed
	}
	if s.current[actor] != id {
		return room.ErrNotMember
	}
	if s.playback[id].Version() != version {
		return room.ErrStalePlayback
	}
	s.playback[id] = p
	return nil
}
func (s *State) AppendChat(_ context.Context, id uuid.UUID, c room.Chat) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune()
	s.touch(id)
	if s.closed[id] {
		return room.ErrClosed
	}
	if s.current[c.AuthorID()] != id {
		return room.ErrNotMember
	}
	s.chat[id] = append(s.chat[id], c)
	if len(s.chat[id]) > 50 {
		s.chat[id] = s.chat[id][len(s.chat[id])-50:]
	}
	return nil
}
func (s *State) Chat(_ context.Context, id uuid.UUID) ([]room.Chat, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune()
	return append([]room.Chat(nil), s.chat[id]...), nil
}
func (s *State) SweepLocked(ctx context.Context, fn func(context.Context) error) error {
	if !s.sweeping.TryLock() {
		return nil
	}
	defer s.sweeping.Unlock()
	return fn(ctx)
}

func (s *State) touch(id uuid.UUID) { s.expires[id] = s.clock.Now().Add(s.ttl) }
func (s *State) prune() {
	now := s.clock.Now()
	for id, until := range s.expires {
		if until.After(now) {
			continue
		}
		for user := range s.members[id] {
			if s.current[user] == id {
				delete(s.current, user)
			}
		}
		delete(s.members, id)
		delete(s.empty, id)
		delete(s.closed, id)
		delete(s.playback, id)
		delete(s.chat, id)
		delete(s.expires, id)
	}
}

func (s *State) ResetPlayback(_ context.Context, id uuid.UUID, version uint64, p room.Playback) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune()
	s.touch(id)
	if s.closed[id] {
		return room.ErrClosed
	}
	if s.playback[id].Version() != version {
		return room.ErrStalePlayback
	}
	s.playback[id] = p
	return nil
}
