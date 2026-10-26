package inmem

import (
	"context"
	"maps"
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
	pets   map[uuid.UUID]owned
	fights []user.PetFight
	stats  user.PetShopStats
}

func NewPetRepository(users *UserRepository) *PetRepository {
	r := &PetRepository{users: users, shop: user.DefaultPetShop(), pets: map[uuid.UUID]owned{}}
	users.onDelete(r.forget)

	return r
}

func (r *PetRepository) forget(id uuid.UUID) {
	r.mu.Lock()
	defer r.mu.Unlock()

	maps.DeleteFunc(r.pets, func(_ uuid.UUID, o owned) bool { return o.owner == id })
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

// owned is a pet with its owner.
type owned struct {
	owner uuid.UUID
	state user.PetState
}

func (r *PetRepository) Pets(ctx context.Context, userID uuid.UUID) ([]*user.Pet, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	var pets []*user.Pet
	for _, p := range r.pets {
		if p.owner == userID {
			pets = append(pets, user.RestorePet(p.state))
		}
	}
	slices.SortFunc(pets, func(a, b *user.Pet) int { return compareUUID(a.ID(), b.ID()) })

	return pets, nil
}

func (r *PetRepository) Pet(ctx context.Context, userID, petID uuid.UUID) (*user.Pet, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	p, ok := r.pets[petID]
	if !ok || p.owner != userID {
		return nil, user.ErrPetNotFound
	}

	return user.RestorePet(p.state), nil
}

// LockPet is Pet: there are no transactions in memory.
func (r *PetRepository) LockPet(ctx context.Context, userID, petID uuid.UUID) (*user.Pet, error) {
	return r.Pet(ctx, userID, petID)
}

func (r *PetRepository) AddPet(ctx context.Context, userID uuid.UUID, p *user.Pet) error {
	if _, err := r.users.GetByID(ctx, userID); err != nil {
		return errs.Wrap("r.users.GetByID", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	kept := 0
	for _, o := range r.pets {
		if o.owner == userID {
			kept++
		}
	}
	if kept >= user.MaxPets {
		return user.ErrTooManyPets
	}
	r.pets[p.ID()] = owned{owner: userID, state: p.State()}

	return nil
}

func (r *PetRepository) SavePet(ctx context.Context, p *user.Pet) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	o, ok := r.pets[p.ID()]
	if !ok {
		return user.ErrPetNotFound
	}
	o.state = p.State()
	r.pets[p.ID()] = o

	return nil
}

func (r *PetRepository) DeletePet(ctx context.Context, userID, petID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if o, ok := r.pets[petID]; !ok || o.owner != userID {
		return user.ErrPetNotFound
	}
	delete(r.pets, petID)

	return nil
}

// shown tells whether the owner of a pet is neither deleted nor banned at
// now.
func (r *PetRepository) shown(ctx context.Context, userID uuid.UUID, now time.Time) bool {
	u, err := r.users.GetByID(ctx, userID)

	return err == nil && !u.IsDeactivated() && !u.IsBannedAt(now)
}

// ranked are the pets of owners shown at now, the highest rating first.
func (r *PetRepository) ranked(ctx context.Context, now time.Time) []owned {
	r.mu.Lock()
	pets := make([]owned, 0, len(r.pets))
	for _, o := range r.pets {
		pets = append(pets, o)
	}
	r.mu.Unlock()

	pets = slices.DeleteFunc(pets, func(o owned) bool { return !r.shown(ctx, o.owner, now) })
	slices.SortFunc(pets, func(a, b owned) int {
		if a.state.Rating != b.state.Rating {
			return b.state.Rating - a.state.Rating
		}

		return compareUUID(a.state.ID, b.state.ID)
	})

	return pets
}

// Opponent is the nearest in rating in memory: no randomness, for tests.
func (r *PetRepository) Opponent(ctx context.Context, userID uuid.UUID, rating int, now time.Time) (owner, pet uuid.UUID, err error) {
	if err = ctx.Err(); err != nil {
		return uuid.Nil, uuid.Nil, errs.Wrap("ctx.Err", err)
	}

	gap := -1
	for _, o := range r.ranked(ctx, now) {
		if o.owner == userID {
			continue
		}
		if d := max(o.state.Rating-rating, rating-o.state.Rating); gap < 0 || d < gap {
			owner, pet, gap = o.owner, o.state.ID, d
		}
	}

	if gap < 0 {
		return uuid.Nil, uuid.Nil, user.ErrNoPetOpponent
	}

	return owner, pet, nil
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
