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
	mu       sync.RWMutex
	gifts    map[uuid.UUID]*user.Gift
	prices   user.GiftPrices
	stock    map[user.GiftKind]user.GiftStock
	auctions map[uuid.UUID]*user.GiftAuction
	history  []user.GiftEvent
	users    *UserRepository
}

func NewGiftRepository(users *UserRepository) *GiftRepository {
	r := &GiftRepository{
		gifts:    make(map[uuid.UUID]*user.Gift),
		prices:   make(user.GiftPrices),
		stock:    make(map[user.GiftKind]user.GiftStock),
		auctions: make(map[uuid.UUID]*user.GiftAuction),
		users:    users,
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
			delete(r.auctions, id)
		case g.SenderID() == userID:
			unsigned := user.RestoreGift(g.ID(), g.RecipientID(), uuid.Nil, g.Kind(), g.Caption(), g.Anonymous())
			r.gifts[id] = unsigned.RestoreCollectible(g.Number(), g.Traits(), g.SalePrice())
		}
	}

	for id, a := range r.auctions {
		if a.BidderID() == userID {
			r.auctions[id] = user.RestoreGiftAuction(a.GiftID(), a.SellerID(), a.Start(), a.EndsAt(), a.TopBid(), uuid.Nil, a.Bids())
		}
	}

	r.history = forgetEvents(r.history, userID)
}

// Gifts are immutable, so sharing pointers with callers is safe.
func (r *GiftRepository) Save(ctx context.Context, g *user.Gift) error {
	for _, id := range []uuid.UUID{g.RecipientID(), g.SenderID()} {
		if id == uuid.Nil {
			continue // the site holds it, or it came from nobody
		}

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
	delete(r.auctions, id)

	r.history = slices.DeleteFunc(r.history, func(e user.GiftEvent) bool { return e.GiftID == id })

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

// Prices are a copy: the caller may keep it.
func (r *GiftRepository) Prices(ctx context.Context) (user.GiftPrices, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	prices := make(user.GiftPrices, len(r.prices))
	for kind, price := range r.prices {
		prices[kind] = price
	}

	return prices, nil
}

func (r *GiftRepository) SetPrice(ctx context.Context, kind user.GiftKind, price user.GiftPrice) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.prices[kind] = price

	return nil
}
