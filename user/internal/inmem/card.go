package inmem

import (
	"context"
	"sync"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// CardRepository keeps the virtual cards of pages in memory.
type CardRepository struct {
	mu    sync.RWMutex
	cards map[uuid.UUID]user.BankCard
}

func NewCardRepository() *CardRepository {
	return &CardRepository{cards: map[uuid.UUID]user.BankCard{}}
}

func (r *CardRepository) Card(ctx context.Context, userID uuid.UUID) (user.BankCard, error) {
	if err := ctx.Err(); err != nil {
		return user.BankCard{}, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	c, ok := r.cards[userID]
	if !ok {
		return user.BankCard{}, user.ErrNoCard
	}

	return c, nil
}

func (r *CardRepository) AddCard(ctx context.Context, userID uuid.UUID, c user.BankCard) (user.BankCard, error) {
	if err := ctx.Err(); err != nil {
		return user.BankCard{}, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if had, ok := r.cards[userID]; ok {
		return had, nil
	}
	for _, other := range r.cards {
		if other.Number() == c.Number() {
			return user.BankCard{}, user.ErrCardNumberTaken
		}
	}
	r.cards[userID] = c

	return c, nil
}

func (r *CardRepository) SetCardCover(ctx context.Context, userID uuid.UUID, cover user.CardCover) (user.BankCard, error) {
	if err := ctx.Err(); err != nil {
		return user.BankCard{}, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	c, ok := r.cards[userID]
	if !ok {
		return user.BankCard{}, user.ErrNoCard
	}
	c = user.RestoreBankCard(c.Number(), cover, c.IssuedAt())
	r.cards[userID] = c

	return c, nil
}
