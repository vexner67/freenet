package inmem

import (
	"context"
	"sync"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

type purchaseKey struct {
	buyerID uuid.UUID
	postID  uuid.UUID
}

// PurchaseRepository keeps in memory the paid posts people opened and the
// messages paid for.
type PurchaseRepository struct {
	mu        sync.Mutex
	purchases map[purchaseKey]struct{}
	payments  map[uuid.UUID]user.MessagePayment
}

func NewPurchaseRepository() *PurchaseRepository {
	return &PurchaseRepository{
		purchases: make(map[purchaseKey]struct{}),
		payments:  make(map[uuid.UUID]user.MessagePayment),
	}
}

func (r *PurchaseRepository) Bought(ctx context.Context, buyerID uuid.UUID, postIDs []uuid.UUID) ([]uuid.UUID, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	var bought []uuid.UUID
	for _, id := range postIDs {
		if _, ok := r.purchases[purchaseKey{buyerID, id}]; ok {
			bought = append(bought, id)
		}
	}

	return bought, nil
}

func (r *PurchaseRepository) AddPurchase(ctx context.Context, buyerID, postID, _ uuid.UUID, _ int64) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	key := purchaseKey{buyerID, postID}
	if _, ok := r.purchases[key]; ok {
		return false, nil
	}

	r.purchases[key] = struct{}{}

	return true, nil
}

func (r *PurchaseRepository) SavePayment(ctx context.Context, p user.MessagePayment) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.payments[p.ID] = p

	return nil
}

func (r *PurchaseRepository) TakePayment(ctx context.Context, id, senderID uuid.UUID) (user.MessagePayment, error) {
	if err := ctx.Err(); err != nil {
		return user.MessagePayment{}, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	p, ok := r.payments[id]
	if !ok || p.SenderID != senderID {
		return user.MessagePayment{}, user.ErrPaymentNotFound
	}

	delete(r.payments, id)

	return p, nil
}
