package inmem

import (
	"context"
	"slices"
	"sync"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// StickerRepository keeps the packs users have taken in memory; they go
// with their user, as in the table.
type StickerRepository struct {
	mu    sync.Mutex
	packs map[uuid.UUID][]user.OwnedPack
	users *UserRepository
}

func NewStickerRepository(users *UserRepository) *StickerRepository {
	r := &StickerRepository{
		packs: make(map[uuid.UUID][]user.OwnedPack),
		users: users,
	}
	users.onDelete(r.forget)

	return r
}

func (r *StickerRepository) forget(userID uuid.UUID) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.packs, userID)
}

func (r *StickerRepository) Add(ctx context.Context, userID uuid.UUID, pack user.StickerPackID) (bool, error) {
	if _, err := r.users.GetByID(ctx, userID); err != nil {
		return false, errs.Wrap("r.users.GetByID", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	owned := r.packs[userID]
	if i := slices.IndexFunc(owned, func(o user.OwnedPack) bool { return o.Pack == pack }); i >= 0 {
		owned[i].Added = true
		return false, nil
	}

	r.packs[userID] = append(owned, user.OwnedPack{Pack: pack, Added: true})

	return true, nil
}

func (r *StickerRepository) Remove(ctx context.Context, userID uuid.UUID, pack user.StickerPackID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for i, o := range r.packs[userID] {
		if o.Pack == pack {
			r.packs[userID][i].Added = false
		}
	}

	return nil
}

func (r *StickerRepository) List(ctx context.Context, userID uuid.UUID) ([]user.OwnedPack, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	return slices.Clone(r.packs[userID]), nil
}

// holders are those who have taken the pack.
func (r *StickerRepository) holders(pack user.StickerPackID) []uuid.UUID {
	r.mu.Lock()
	defer r.mu.Unlock()

	var ids []uuid.UUID
	for userID, owned := range r.packs {
		if slices.ContainsFunc(owned, func(o user.OwnedPack) bool { return o.Pack == pack }) {
			ids = append(ids, userID)
		}
	}

	return ids
}

// drop takes the pack away from everyone, as its row in the table goes.
func (r *StickerRepository) drop(pack user.StickerPackID) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for userID, owned := range r.packs {
		r.packs[userID] = slices.DeleteFunc(owned, func(o user.OwnedPack) bool { return o.Pack == pack })
	}
}
