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

type datingAnswer struct {
	like bool
	at   time.Time
}

// DatingRepository keeps "Знакомства" in memory; a deleted user takes
// their card and answers with them, as the foreign keys do.
type DatingRepository struct {
	users *UserRepository

	mu      sync.Mutex
	cards   map[uuid.UUID]user.DatingCard
	answers map[[2]uuid.UUID]datingAnswer
}

func NewDatingRepository(users *UserRepository) *DatingRepository {
	r := &DatingRepository{users: users, cards: map[uuid.UUID]user.DatingCard{}, answers: map[[2]uuid.UUID]datingAnswer{}}
	users.onDelete(r.forget)

	return r
}

func (r *DatingRepository) forget(id uuid.UUID) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.cards, id)
	for k := range r.answers {
		if k[0] == id || k[1] == id {
			delete(r.answers, k)
		}
	}
}

func (r *DatingRepository) Card(ctx context.Context, userID uuid.UUID) (user.DatingCard, error) {
	if err := ctx.Err(); err != nil {
		return user.DatingCard{}, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	c, ok := r.cards[userID]
	if !ok {
		return user.DatingCard{}, user.ErrNotFound
	}

	return c, nil
}

func (r *DatingRepository) SaveCard(ctx context.Context, c user.DatingCard) error {
	if _, err := r.users.GetByID(ctx, c.UserID); err != nil {
		return errs.Wrap("r.users.GetByID", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.cards[c.UserID] = c

	return nil
}

func (r *DatingRepository) Candidates(ctx context.Context, userID uuid.UUID, limit int) ([]user.DatingCard, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	var cards []user.DatingCard
	for id, c := range r.cards {
		if _, answered := r.answers[[2]uuid.UUID{userID, id}]; c.On && id != userID && !answered {
			cards = append(cards, c)
		}
	}
	slices.SortFunc(cards, func(a, b user.DatingCard) int {
		if c := b.UpdatedAt.Compare(a.UpdatedAt); c != 0 {
			return c
		}

		return slices.Compare(b.UserID[:], a.UserID[:])
	})

	return cards[:min(limit, len(cards))], nil
}

func (r *DatingRepository) Answer(ctx context.Context, fromID, toID uuid.UUID, like bool, at time.Time) error {
	if _, err := r.users.GetByID(ctx, toID); err != nil {
		return errs.Wrap("r.users.GetByID", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	key := [2]uuid.UUID{fromID, toID}
	if _, ok := r.answers[key]; ok {
		return user.ErrDatingTwice
	}
	r.answers[key] = datingAnswer{like: like, at: at.UTC()}

	return nil
}

func (r *DatingRepository) Liked(ctx context.Context, fromID, toID uuid.UUID) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	return r.answers[[2]uuid.UUID{fromID, toID}].like, nil
}

func (r *DatingRepository) Matches(ctx context.Context, userID uuid.UUID) ([]user.DatingMatch, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	var matches []user.DatingMatch
	for k, mine := range r.answers {
		if k[0] != userID || !mine.like {
			continue
		}
		if theirs := r.answers[[2]uuid.UUID{k[1], userID}]; theirs.like {
			at := mine.at
			if theirs.at.After(at) {
				at = theirs.at
			}
			matches = append(matches, user.DatingMatch{UserID: k[1], At: at})
		}
	}
	slices.SortFunc(matches, func(a, b user.DatingMatch) int { return b.At.Compare(a.At) })

	return matches, nil
}
