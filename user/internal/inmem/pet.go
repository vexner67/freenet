package inmem

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// PetRepository keeps the pets, their fights and the shop in memory; a
// deleted user takes their pet and fights, as the foreign keys do.
type PetRepository struct {
	users *UserRepository

	mu     sync.Mutex
	shop   user.PetShop
	pets   map[uuid.UUID]user.PetState
	fights []user.PetFight
	stats  user.PetShopStats
}

func NewPetRepository(users *UserRepository) *PetRepository {
	r := &PetRepository{users: users, shop: user.DefaultPetShop(), pets: map[uuid.UUID]user.PetState{}}
	users.onDelete(r.forget)

	return r
}

func (r *PetRepository) forget(id uuid.UUID) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.pets, id)
	r.fights = slices.DeleteFunc(r.fights, func(f user.PetFight) bool { return f.AttackerID == id || f.DefenderID == id })
}

func (r *PetRepository) Shop(ctx context.Context) (user.PetShop, error) {
	if err := ctx.Err(); err != nil {
		return user.PetShop{}, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	return r.shop, nil
}

func (r *PetRepository) SetShop(ctx context.Context, shop user.PetShop) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.shop = shop

	return nil
}

func (r *PetRepository) Pet(ctx context.Context, userID uuid.UUID) (*user.Pet, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	s, ok := r.pets[userID]
	if !ok {
		return nil, user.ErrPetNotFound
	}

	return user.RestorePet(s), nil
}

// LockPet is Pet: there are no transactions in memory.
func (r *PetRepository) LockPet(ctx context.Context, userID uuid.UUID) (*user.Pet, error) {
	return r.Pet(ctx, userID)
}

func (r *PetRepository) AddPet(ctx context.Context, userID uuid.UUID, p *user.Pet) error {
	if _, err := r.users.GetByID(ctx, userID); err != nil {
		return errs.Wrap("r.users.GetByID", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.pets[userID]; ok {
		return user.ErrPetExists
	}
	r.pets[userID] = p.State()

	return nil
}

func (r *PetRepository) SavePet(ctx context.Context, userID uuid.UUID, p *user.Pet) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.pets[userID]; !ok {
		return user.ErrPetNotFound
	}
	r.pets[userID] = p.State()

	return nil
}

func (r *PetRepository) DeletePet(ctx context.Context, userID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.pets, userID)

	return nil
}

// shown tells whether the owner of a pet is neither deleted nor banned at
// now.
func (r *PetRepository) shown(ctx context.Context, userID uuid.UUID, now time.Time) bool {
	u, err := r.users.GetByID(ctx, userID)

	return err == nil && !u.IsDeactivated() && !u.IsBannedAt(now)
}

// ranked are the owners of pets shown at now, the highest rating first.
func (r *PetRepository) ranked(ctx context.Context, now time.Time) []uuid.UUID {
	r.mu.Lock()
	ids := make([]uuid.UUID, 0, len(r.pets))
	ratings := make(map[uuid.UUID]int, len(r.pets))
	for id, s := range r.pets {
		ids = append(ids, id)
		ratings[id] = s.Rating
	}
	r.mu.Unlock()

	ids = slices.DeleteFunc(ids, func(id uuid.UUID) bool { return !r.shown(ctx, id, now) })
	slices.SortFunc(ids, func(a, b uuid.UUID) int {
		if ratings[a] != ratings[b] {
			return ratings[b] - ratings[a]
		}

		return compareUUID(a, b)
	})

	return ids
}

// Opponent is the nearest in rating in memory: no randomness, for tests.
func (r *PetRepository) Opponent(ctx context.Context, userID uuid.UUID, rating int, now time.Time) (uuid.UUID, error) {
	if err := ctx.Err(); err != nil {
		return uuid.Nil, errs.Wrap("ctx.Err", err)
	}

	found, gap := uuid.Nil, -1
	for _, id := range r.ranked(ctx, now) {
		if id == userID {
			continue
		}

		p, err := r.Pet(ctx, id)
		if err != nil {
			continue
		}
		if d := max(p.Rating()-rating, rating-p.Rating()); gap < 0 || d < gap {
			found, gap = id, d
		}
	}

	if found == uuid.Nil {
		return uuid.Nil, user.ErrNoPetOpponent
	}

	return found, nil
}

func (r *PetRepository) AddFight(ctx context.Context, f user.PetFight) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.fights = append(r.fights, f)
	r.stats.Fights++

	return nil
}

func (r *PetRepository) Fights(ctx context.Context, userID uuid.UUID, limit int) ([]user.PetFight, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	var fights []user.PetFight
	for i := len(r.fights) - 1; i >= 0 && len(fights) < limit; i-- {
		if f := r.fights[i]; f.AttackerID == userID || f.DefenderID == userID {
			fights = append(fights, f)
		}
	}

	return fights, nil
}

func (r *PetRepository) Count(ctx context.Context, spent, won int64) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.stats.Spent += spent
	r.stats.Won += won

	return nil
}

func (r *PetRepository) Stats(ctx context.Context) (user.PetShopStats, error) {
	if err := ctx.Err(); err != nil {
		return user.PetShopStats{}, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	stats := r.stats
	stats.Pets = int64(len(r.pets))

	return stats, nil
}
