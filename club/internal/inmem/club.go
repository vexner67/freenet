// Package inmem keeps clubs in process memory, for development and tests.
package inmem

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/club/internal/club"
	"github.com/vexner67/freenet/club/internal/service"
	"github.com/vexner67/freenet/pkg/errs"
)

// Store keeps clubs and their members together, so joining a club counts
// it at once, as the single statement does in PostgreSQL. Its repositories
// are Clubs and Members.
type Store struct {
	mu      sync.RWMutex
	clubs   map[uuid.UUID]club.Stored
	members map[uuid.UUID]*member
}

// member is one membership, keyed by its ID.
type member struct {
	clubID, userID uuid.UUID
	role           club.Role
}

func NewStore() *Store {
	return &Store{
		clubs:   make(map[uuid.UUID]club.Stored),
		members: make(map[uuid.UUID]*member),
	}
}

func (s *Store) Clubs() *ClubRepository {
	return &ClubRepository{s: s}
}

func (s *Store) Members() *MemberRepository {
	return &MemberRepository{s: s}
}

// club restores a stored club with its count; the caller holds a lock.
func (s *Store) club(id uuid.UUID) (*club.Club, bool) {
	stored, ok := s.clubs[id]
	if !ok {
		return nil, false
	}

	for _, m := range s.members {
		if m.clubID == id {
			stored.Members++
		}
	}

	return club.Restore(stored), true
}

// membership finds the membership of userID in clubID; the caller holds a
// lock.
func (s *Store) membership(clubID, userID uuid.UUID) (uuid.UUID, *member) {
	for id, m := range s.members {
		if m.clubID == clubID && m.userID == userID {
			return id, m
		}
	}

	return uuid.Nil, nil
}

// join adds a membership; the caller holds the write lock.
func (s *Store) join(clubID, userID uuid.UUID, role club.Role) error {
	id, err := uuid.NewV7()
	if err != nil {
		return errs.Wrap("uuid.NewV7", err)
	}

	s.members[id] = &member{clubID: clubID, userID: userID, role: role}

	return nil
}

type ClubRepository struct {
	s *Store
}

func (r *ClubRepository) Create(ctx context.Context, c *club.Club, ownerID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	r.s.clubs[c.ID()] = stored(c)

	return r.s.join(c.ID(), ownerID, club.RoleOwner)
}

func stored(c *club.Club) club.Stored {
	return club.Stored{
		ID:          c.ID(),
		Name:        c.Name(),
		Description: c.Description(),
		Address:     c.Address(),
		Wall:        c.Wall(),
		CreatedAt:   c.CreatedAt(),
	}
}

func (r *ClubRepository) Get(ctx context.Context, id uuid.UUID) (*club.Club, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	c, ok := r.s.club(id)
	if !ok {
		return nil, club.ErrNotFound
	}

	return c, nil
}

func (r *ClubRepository) GetByAddress(ctx context.Context, address club.Address) (*club.Club, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	for id, c := range r.s.clubs {
		if c.Address != "" && strings.EqualFold(c.Address.String(), address.String()) {
			found, _ := r.s.club(id)
			return found, nil
		}
	}

	return nil, club.ErrNotFound
}

func (r *ClubRepository) GetMany(ctx context.Context, ids []uuid.UUID) ([]*club.Club, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	var found []*club.Club
	for _, id := range ids {
		if c, ok := r.s.club(id); ok {
			found = append(found, c)
		}
	}

	return found, nil
}

func (r *ClubRepository) Update(ctx context.Context, c *club.Club) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	if _, ok := r.s.clubs[c.ID()]; !ok {
		return club.ErrNotFound
	}

	for id, other := range r.s.clubs {
		if id != c.ID() && c.Address() != "" && strings.EqualFold(other.Address.String(), c.Address().String()) {
			return club.ErrAddressTaken
		}
	}

	r.s.clubs[c.ID()] = stored(c)

	return nil
}

type MemberRepository struct {
	s *Store
}

func (r *MemberRepository) Role(ctx context.Context, clubID, userID uuid.UUID) (club.Role, error) {
	if err := ctx.Err(); err != nil {
		return club.RoleNone, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	if _, m := r.s.membership(clubID, userID); m != nil {
		return m.role, nil
	}

	return club.RoleNone, nil
}

func (r *MemberRepository) Owner(ctx context.Context, clubID uuid.UUID) (uuid.UUID, error) {
	if err := ctx.Err(); err != nil {
		return uuid.Nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	for _, m := range r.s.members {
		if m.clubID == clubID && m.role == club.RoleOwner {
			return m.userID, nil
		}
	}

	return uuid.Nil, nil
}

func (r *MemberRepository) Join(ctx context.Context, clubID, userID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	if _, ok := r.s.clubs[clubID]; !ok {
		return club.ErrNotFound
	}

	if _, m := r.s.membership(clubID, userID); m != nil {
		return nil
	}

	return r.s.join(clubID, userID, club.RoleMember)
}

func (r *MemberRepository) Leave(ctx context.Context, clubID, userID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	if id, m := r.s.membership(clubID, userID); m != nil && m.role != club.RoleOwner {
		delete(r.s.members, id)
	}

	return nil
}

func (r *MemberRepository) SetRole(ctx context.Context, clubID, userID uuid.UUID, role club.Role) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	_, m := r.s.membership(clubID, userID)
	if m == nil || m.role == club.RoleOwner {
		return club.ErrNotMember
	}

	m.role = role

	return nil
}

// sorted returns the memberships that match, the newest first; the caller
// holds a lock.
func (s *Store) sorted(match func(*member) bool, before uuid.UUID, limit int) []uuid.UUID {
	var ids []uuid.UUID
	for id, m := range s.members {
		if match(m) && (before == uuid.Nil || bytes.Compare(id[:], before[:]) < 0) {
			ids = append(ids, id)
		}
	}

	// UUIDv7 bytes sort by time, as in PostgreSQL.
	slices.SortFunc(ids, func(a, b uuid.UUID) int { return bytes.Compare(b[:], a[:]) })

	return ids[:min(limit, len(ids))]
}

func (r *MemberRepository) List(ctx context.Context, clubID, before uuid.UUID, limit int, adminsOnly bool) ([]service.Member, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	match := func(m *member) bool { return m.clubID == clubID && (!adminsOnly || m.role.Manages()) }

	var found []service.Member
	for _, id := range r.s.sorted(match, before, limit) {
		m := r.s.members[id]
		found = append(found, service.Member{ID: id, UserID: m.userID, Role: m.role})
	}

	return found, nil
}

func (r *MemberRepository) ListClubs(ctx context.Context, userID, before uuid.UUID, limit int) ([]service.Membership, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	var found []service.Membership
	for _, id := range r.s.sorted(func(m *member) bool { return m.userID == userID }, before, limit) {
		m := r.s.members[id]
		c, _ := r.s.club(m.clubID)
		found = append(found, service.Membership{ID: id, Club: c, Role: m.role})
	}

	return found, nil
}

func (r *MemberRepository) ClubIDs(ctx context.Context, userID uuid.UUID, limit int) ([]uuid.UUID, error) {
	clubs, err := r.ListClubs(ctx, userID, uuid.Nil, limit)
	if err != nil {
		return nil, err
	}

	ids := make([]uuid.UUID, len(clubs))
	for i, m := range clubs {
		ids[i] = m.Club.ID()
	}

	return ids, nil
}

func (r *MemberRepository) DeleteByUser(ctx context.Context, userID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	for id, m := range r.s.members {
		if m.userID == userID {
			delete(r.s.members, id)
		}
	}

	return nil
}
