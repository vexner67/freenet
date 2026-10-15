package inmem

import (
	"bytes"
	"context"
	"slices"
	"sync"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/notification/internal/push"
	"github.com/vexner67/freenet/pkg/errs"
)

// SubscriptionRepository keeps one subscription per browser and the
// newest push.MaxSubscriptions of a user, as PostgreSQL does.
type SubscriptionRepository struct {
	mu   sync.RWMutex
	subs map[push.Endpoint]*push.Subscription
}

func NewSubscriptionRepository() *SubscriptionRepository {
	return &SubscriptionRepository{
		subs: make(map[push.Endpoint]*push.Subscription),
	}
}

func (r *SubscriptionRepository) Save(ctx context.Context, s *push.Subscription) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.subs[s.Endpoint()] = s

	if mine := r.of(s.UserID()); len(mine) > push.MaxSubscriptions {
		for _, old := range mine[push.MaxSubscriptions:] {
			delete(r.subs, old.Endpoint())
		}
	}

	return nil
}

func (r *SubscriptionRepository) Delete(ctx context.Context, userID uuid.UUID, endpoint push.Endpoint) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if s, ok := r.subs[endpoint]; ok && s.UserID() == userID {
		delete(r.subs, endpoint)
	}

	return nil
}

func (r *SubscriptionRepository) Forget(ctx context.Context, endpoint push.Endpoint) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.subs, endpoint)

	return nil
}

func (r *SubscriptionRepository) List(ctx context.Context, userID uuid.UUID) ([]*push.Subscription, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.of(userID), nil
}

func (r *SubscriptionRepository) DeleteByUser(ctx context.Context, userID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for endpoint, s := range r.subs {
		if s.UserID() == userID {
			delete(r.subs, endpoint)
		}
	}

	return nil
}

// of returns the subscriptions of a user, newest first; r.mu is held.
func (r *SubscriptionRepository) of(userID uuid.UUID) []*push.Subscription {
	var mine []*push.Subscription

	for _, s := range r.subs {
		if s.UserID() == userID {
			mine = append(mine, s)
		}
	}

	slices.SortFunc(mine, func(a, b *push.Subscription) int {
		bid, aid := b.ID(), a.ID()

		return bytes.Compare(bid[:], aid[:])
	})

	return mine
}
