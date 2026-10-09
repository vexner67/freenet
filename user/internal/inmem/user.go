package inmem

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// UserRepository keeps copies of users, so a caller mutating a returned
// user changes nothing until it calls Update, as with a real database.
type UserRepository struct {
	mu         sync.RWMutex
	users      map[uuid.UUID]user.User
	byEmail    map[string]uuid.UUID
	byNickname map[string]uuid.UUID
	// cascade drops what other repositories keep of a deleted user, as
	// the foreign keys do in PostgreSQL.
	cascade []func(uuid.UUID)
}

func NewUserRepository() *UserRepository {
	return &UserRepository{
		users:      make(map[uuid.UUID]user.User),
		byEmail:    make(map[string]uuid.UUID),
		byNickname: make(map[string]uuid.UUID),
	}
}

func (r *UserRepository) Save(ctx context.Context, u *user.User) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.byEmail[emailKey(u.Email())]; ok {
		return user.ErrAlreadyExists
	}

	if r.nicknameTakenByOther(u) {
		return user.ErrNicknameTaken
	}

	r.put(u)

	return nil
}

func (r *UserRepository) Update(ctx context.Context, u *user.User) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	old, ok := r.users[u.ID()]
	if !ok {
		return user.ErrNotFound
	}

	if r.nicknameTakenByOther(u) {
		return user.ErrNicknameTaken
	}

	if nickname := old.Profile().Nickname; nickname != "" {
		delete(r.byNickname, nicknameKey(nickname))
	}

	// As in PostgreSQL, Update does not store visits: a user read before
	// a newer one keeps it.
	stored := *u
	stored.MarkSeen(old.LastSeen())
	r.put(&stored)

	return nil
}

func (r *UserRepository) MarkSeen(ctx context.Context, id uuid.UUID, at time.Time) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if u, ok := r.users[id]; ok {
		u.MarkSeen(at)
		r.users[id] = u
	}

	return nil
}

func (r *UserRepository) GetByID(ctx context.Context, id uuid.UUID) (*user.User, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	u, ok := r.users[id]
	if !ok {
		return nil, user.ErrNotFound
	}

	return &u, nil
}

func (r *UserRepository) GetByNickname(ctx context.Context, nickname user.Nickname) (*user.User, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	id, ok := r.byNickname[nicknameKey(nickname)]
	if !ok {
		return nil, user.ErrNotFound
	}

	u := r.users[id]

	return &u, nil
}

func (r *UserRepository) GetByIDs(ctx context.Context, ids []uuid.UUID) ([]*user.User, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	found := make([]*user.User, 0, len(ids))
	for _, id := range ids {
		if u, ok := r.users[id]; ok {
			found = append(found, &u)
		}
	}

	return found, nil
}

func (r *UserRepository) Search(ctx context.Context, q user.SearchQuery, before uuid.UUID, limit int) ([]*user.User, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var found []*user.User
	for id, u := range r.users {
		if u.IsProfileCompleted() && !u.IsDeactivated() && q.Matches(&u) && (before == uuid.Nil || bytes.Compare(id[:], before[:]) < 0) {
			found = append(found, &u)
		}
	}

	// Newest first, as in PostgreSQL.
	slices.SortFunc(found, func(a, b *user.User) int {
		ai, bi := a.ID(), b.ID()
		return bytes.Compare(bi[:], ai[:])
	})

	if len(found) > limit {
		found = found[:limit]
	}

	return found, nil
}

func (r *UserRepository) GetIDByEmail(ctx context.Context, email user.Email) (uuid.UUID, error) {
	if err := ctx.Err(); err != nil {
		return uuid.Nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	id, ok := r.byEmail[emailKey(email)]
	if !ok {
		return uuid.Nil, user.ErrNotFound
	}

	return id, nil
}

// put stores a copy of u and indexes it; the caller holds the write lock.
func (r *UserRepository) put(u *user.User) {
	r.users[u.ID()] = *u
	r.byEmail[emailKey(u.Email())] = u.ID()

	if nickname := u.Profile().Nickname; nickname != "" {
		r.byNickname[nicknameKey(nickname)] = u.ID()
	}
}

func (r *UserRepository) nicknameTakenByOther(u *user.User) bool {
	nickname := u.Profile().Nickname
	if nickname == "" {
		return false
	}

	id, ok := r.byNickname[nicknameKey(nickname)]

	return ok && id != u.ID()
}

// emailKey and nicknameKey make lookups case-insensitive, matching the
// lower(...) unique indexes in PostgreSQL.
func emailKey(email user.Email) string {
	return strings.ToLower(email.String())
}

func nicknameKey(nickname user.Nickname) string {
	return strings.ToLower(nickname.String())
}

// onDelete adds what a deleted user's ID takes with it.
func (r *UserRepository) onDelete(fn func(uuid.UUID)) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.cascade = append(r.cascade, fn)
}

func (r *UserRepository) DeleteDue(ctx context.Context, now time.Time, limit int) ([]uuid.UUID, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()

	var due []uuid.UUID
	for id, u := range r.users {
		if len(due) == limit {
			break
		}

		if d := u.Deactivation(); u.IsDeactivated() && !d.PurgeAt.After(now) {
			due = append(due, id)
		}
	}

	for _, id := range due {
		u := r.users[id]
		delete(r.users, id)
		delete(r.byEmail, emailKey(u.Email()))
		if nickname := u.Profile().Nickname; nickname != "" {
			delete(r.byNickname, nicknameKey(nickname))
		}
	}

	// The relationships naming them name nobody, as ON DELETE SET NULL.
	for id, u := range r.users {
		if p := u.Profile(); slices.Contains(due, p.Partner) {
			p.Partner = uuid.Nil
			_ = u.UpdateProfile(p) // the profile was valid before
			r.users[id] = u
		}
	}

	cascade := r.cascade
	r.mu.Unlock()

	// Outside the lock: the other repositories read users under theirs.
	for _, id := range due {
		for _, fn := range cascade {
			fn(id)
		}
	}

	return due, nil
}
