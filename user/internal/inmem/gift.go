package inmem

import (
	"bytes"
	"context"
	"slices"
	"sync"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// GiftRepository keeps gifts in memory and behaves as the table does: a
// gift goes with the page it was sent to and loses its sender with theirs.
type GiftRepository struct {
	mu    sync.RWMutex
	gifts map[uuid.UUID]*user.Gift
	users *UserRepository
}

func NewGiftRepository(users *UserRepository) *GiftRepository {
	r := &GiftRepository{
		gifts: make(map[uuid.UUID]*user.Gift),
		users: users,
	}
	users.onDelete(r.forget)

	return r
}

// forget drops the gifts sent to a deleted user and unsigns those they
// sent.
func (r *GiftRepository) forget(userID uuid.UUID) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for id, g := range r.gifts {
		switch {
		case g.RecipientID() == userID:
			delete(r.gifts, id)
		case g.SenderID() == userID:
			r.gifts[id] = user.RestoreGift(g.ID(), g.RecipientID(), uuid.Nil, g.Kind(), g.Caption(), g.Anonymous())
		}
	}
}

// Gifts are immutable, so sharing pointers with callers is safe.
func (r *GiftRepository) Save(ctx context.Context, g *user.Gift) error {
	for _, id := range []uuid.UUID{g.RecipientID(), g.SenderID()} {
		if _, err := r.users.GetByID(ctx, id); err != nil {
			return errs.Wrap("r.users.GetByID", err)
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.gifts[g.ID()] = g

	return nil
}

func (r *GiftRepository) Get(ctx context.Context, id uuid.UUID) (*user.Gift, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	g, ok := r.gifts[id]
	if !ok {
		return nil, user.ErrGiftNotFound
	}

	return g, nil
}

func (r *GiftRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.gifts, id)

	return nil
}

func (r *GiftRepository) List(ctx context.Context, recipientID, before uuid.UUID, limit int) ([]*user.Gift, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var found []*user.Gift

	for id, g := range r.gifts {
		if g.RecipientID() == recipientID && (before == uuid.Nil || bytes.Compare(id[:], before[:]) < 0) {
			found = append(found, g)
		}
	}

	slices.SortFunc(found, func(a, b *user.Gift) int {
		aID, bID := a.ID(), b.ID()
		return bytes.Compare(bID[:], aID[:])
	})

	return found[:min(len(found), limit)], nil
}

func (r *GiftRepository) Count(ctx context.Context, recipientID uuid.UUID) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0

	for _, g := range r.gifts {
		if g.RecipientID() == recipientID {
			count++
		}
	}

	return count, nil
}
