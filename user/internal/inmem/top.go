package inmem

import (
	"cmp"
	"context"
	"slices"
	"time"

	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// TopRepository counts the boards of "Рейтинги" over the users and pets
// in memory.
type TopRepository struct {
	users *UserRepository
	pets  *PetRepository
}

func NewTopRepository(users *UserRepository, pets *PetRepository) *TopRepository {
	return &TopRepository{users: users, pets: pets}
}

// top keeps the entries above zero, the greatest first, at most limit.
func top(entries []user.TopEntry, limit int) []user.TopEntry {
	entries = slices.DeleteFunc(entries, func(e user.TopEntry) bool { return e.Value <= 0 })
	slices.SortFunc(entries, func(a, b user.TopEntry) int {
		if c := cmp.Compare(b.Value, a.Value); c != 0 {
			return c
		}

		return compareUUID(a.UserID, b.UserID)
	})

	return entries[:min(len(entries), limit)]
}

// shown are the users neither deleted nor banned at now.
func (r *TopRepository) shown(now time.Time) []user.User {
	r.users.mu.RLock()
	defer r.users.mu.RUnlock()

	var shown []user.User
	for _, u := range r.users.users {
		if !u.IsDeactivated() && !u.IsBannedAt(now) {
			shown = append(shown, u)
		}
	}

	return shown
}

func (r *TopRepository) Rich(ctx context.Context, now time.Time, limit int) ([]user.TopEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	var entries []user.TopEntry
	for _, u := range r.shown(now) {
		entries = append(entries, user.TopEntry{UserID: u.ID(), Value: u.Wallet().Votes})
	}

	return top(entries, limit), nil
}

func (r *TopRepository) Earned(ctx context.Context, kinds []user.VoteKind, since, now time.Time, limit int) ([]user.TopEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	shown := r.shown(now)

	r.users.mu.RLock()
	defer r.users.mu.RUnlock()

	var entries []user.TopEntry
	for _, u := range shown {
		var sum int64
		for _, op := range r.users.operations[u.ID()] {
			if slices.Contains(kinds, op.Kind) && !op.At.Before(since) {
				sum += op.Amount
			}
		}
		entries = append(entries, user.TopEntry{UserID: u.ID(), Value: sum})
	}

	return top(entries, limit), nil
}

func (r *TopRepository) Pets(ctx context.Context, now time.Time, limit int) ([]user.TopEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	var entries []user.TopEntry
	for _, o := range r.pets.ranked(ctx, now) {
		if len(entries) == limit {
			break
		}

		entries = append(entries, user.TopEntry{UserID: o.owner, Value: int64(o.state.Rating), PetKind: o.state.Kind, PetName: o.state.Name})
	}

	return entries, nil
}
