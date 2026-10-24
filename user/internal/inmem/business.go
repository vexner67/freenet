package inmem

import (
	"context"
	"sync"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

type businessKey struct {
	userID uuid.UUID
	kind   user.BusinessKind
}

// BusinessRepository keeps the businesses of the site and of its pages in
// memory.
type BusinessRepository struct {
	mu         sync.RWMutex
	rules      user.BusinessRules
	businesses map[businessKey]user.Business
	spent      int64
	paid       int64
}

func NewBusinessRepository() *BusinessRepository {
	return &BusinessRepository{
		rules:      user.DefaultBusinessRules(),
		businesses: map[businessKey]user.Business{},
	}
}

func (r *BusinessRepository) Rules(ctx context.Context) (user.BusinessRules, error) {
	if err := ctx.Err(); err != nil {
		return user.BusinessRules{}, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.rules, nil
}

func (r *BusinessRepository) SetRules(ctx context.Context, rules user.BusinessRules) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.rules = rules

	return nil
}

func (r *BusinessRepository) Businesses(ctx context.Context, userID uuid.UUID) ([]user.Business, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var owned []user.Business
	for _, kind := range user.BusinessKinds {
		if b, ok := r.businesses[businessKey{userID, kind}]; ok {
			owned = append(owned, b)
		}
	}

	return owned, nil
}

func (r *BusinessRepository) Business(ctx context.Context, userID uuid.UUID, kind user.BusinessKind) (user.Business, error) {
	if err := ctx.Err(); err != nil {
		return user.Business{}, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	b, ok := r.businesses[businessKey{userID, kind}]
	if !ok {
		return user.Business{}, user.ErrBusinessNotFound
	}

	return b, nil
}

func (r *BusinessRepository) AddBusiness(ctx context.Context, userID uuid.UUID, b user.Business) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	key := businessKey{userID, b.Kind()}
	if _, ok := r.businesses[key]; ok {
		return user.ErrBusinessOwned
	}

	r.businesses[key] = b

	return nil
}

func (r *BusinessRepository) SaveBusiness(ctx context.Context, userID uuid.UUID, b user.Business) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	key := businessKey{userID, b.Kind()}
	if _, ok := r.businesses[key]; !ok {
		return user.ErrBusinessNotFound
	}

	r.businesses[key] = b

	return nil
}

func (r *BusinessRepository) DeleteBusiness(ctx context.Context, userID uuid.UUID, kind user.BusinessKind) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.businesses, businessKey{userID, kind})

	return nil
}

func (r *BusinessRepository) Count(ctx context.Context, spent, paid int64) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.spent += spent
	r.paid += paid

	return nil
}

func (r *BusinessRepository) Stats(ctx context.Context) (user.BusinessStats, error) {
	if err := ctx.Err(); err != nil {
		return user.BusinessStats{}, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return user.BusinessStats{Businesses: int64(len(r.businesses)), Spent: r.spent, Paid: r.paid}, nil
}
