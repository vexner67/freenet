package inmem

import (
	"bytes"
	"cmp"
	"context"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

func (r *GiftRepository) Stocks(ctx context.Context) (map[user.GiftKind]user.GiftStock, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	stocks := make(map[user.GiftKind]user.GiftStock, len(r.stock))
	for kind, s := range r.stock {
		stocks[kind] = s
	}

	return stocks, nil
}

func (r *GiftRepository) SetSupply(ctx context.Context, kind user.GiftKind, supply user.GiftSupply) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	s := r.stock[kind]
	s.Supply = supply
	r.stock[kind] = s

	return nil
}

func (r *GiftRepository) Sell(ctx context.Context, kind user.GiftKind) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	s := r.stock[kind]
	if s.Left() == 0 {
		return user.ErrGiftSoldOut
	}

	s.Sold++
	r.stock[kind] = s

	return nil
}

func (r *GiftRepository) Mint(ctx context.Context, kind user.GiftKind) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	s := r.stock[kind]
	s.Minted++
	r.stock[kind] = s

	return s.Minted, nil
}

// Lock is Get: transactions do not run here.
func (r *GiftRepository) Lock(ctx context.Context, id uuid.UUID) (*user.Gift, error) {
	return r.Get(ctx, id)
}

func (r *GiftRepository) Update(ctx context.Context, g *user.Gift) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.gifts[g.ID()]; !ok {
		return user.ErrGiftNotFound
	}

	r.gifts[g.ID()] = g

	return nil
}

func (r *GiftRepository) Market(ctx context.Context, f user.MarketFilter, after *user.MarketKey, limit int) ([]*user.Gift, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	var found []*user.Gift
	for _, g := range r.gifts {
		t := g.Traits()
		if g.IsOnSale() && matches(string(f.Kind), string(g.Kind())) &&
			matches(f.Model, t.Model) && matches(f.Backdrop, t.Backdrop) && matches(f.Symbol, t.Symbol) {
			found = append(found, g)
		}
	}
	r.mu.RUnlock()

	now := time.Now()
	found = slices.DeleteFunc(found, func(g *user.Gift) bool {
		owner, err := r.users.GetByID(ctx, g.OwnerID())
		return err != nil || owner.IsDeactivated() || owner.IsBannedAt(now)
	})

	order := func(a, b user.MarketKey) int {
		return cmp.Or(cmp.Compare(a.Value, b.Value), bytes.Compare(a.ID[:], b.ID[:]))
	}
	if f.Sort == user.MarketDearest {
		order = func(a, b user.MarketKey) int {
			return cmp.Or(cmp.Compare(b.Value, a.Value), bytes.Compare(b.ID[:], a.ID[:]))
		}
	}

	slices.SortFunc(found, func(a, b *user.Gift) int { return order(f.Of(a), f.Of(b)) })

	if after != nil {
		found = slices.DeleteFunc(found, func(g *user.Gift) bool { return order(f.Of(g), *after) <= 0 })
	}

	return found[:min(len(found), limit)], nil
}

func matches(want, have string) bool {
	return want == "" || want == have
}

func (r *GiftRepository) SaveEvent(ctx context.Context, e user.GiftEvent) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.history = append(r.history, e)

	return nil
}

func (r *GiftRepository) History(ctx context.Context, giftID uuid.UUID, limit int) ([]user.GiftEvent, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var events []user.GiftEvent

	for i := len(r.history) - 1; i >= 0 && len(events) < limit; i-- {
		if r.history[i].GiftID == giftID {
			events = append(events, r.history[i])
		}
	}

	return events, nil
}

// forgetEvents unsigns the lines of the history of an erased page.
func forgetEvents(history []user.GiftEvent, userID uuid.UUID) []user.GiftEvent {
	for i, e := range history {
		if e.FromID == userID {
			history[i].FromID = uuid.Nil
		}

		if e.ToID == userID {
			history[i].ToID = uuid.Nil
		}
	}

	return history
}

func (r *GiftRepository) SaveAuction(ctx context.Context, a *user.GiftAuction) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.auctions[a.GiftID()] = a

	return nil
}

func (r *GiftRepository) Auction(ctx context.Context, giftID uuid.UUID) (*user.GiftAuction, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	a, ok := r.auctions[giftID]
	if !ok {
		return nil, user.ErrAuctionNotFound
	}

	return a, nil
}

func (r *GiftRepository) LockAuction(ctx context.Context, giftID uuid.UUID) (*user.GiftAuction, error) {
	return r.Auction(ctx, giftID)
}

func (r *GiftRepository) DeleteAuction(ctx context.Context, giftID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.auctions, giftID)

	return nil
}

func auctionOrder(a, b service.AuctionKey) int {
	return cmp.Or(a.EndsAt.Compare(b.EndsAt), bytes.Compare(a.GiftID[:], b.GiftID[:]))
}

func keyOf(a *user.GiftAuction) service.AuctionKey {
	return service.AuctionKey{EndsAt: a.EndsAt(), GiftID: a.GiftID()}
}

func (r *GiftRepository) Auctions(ctx context.Context, now time.Time, after *service.AuctionKey, limit int) ([]*user.GiftAuction, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var found []*user.GiftAuction

	for _, a := range r.auctions {
		if !a.Ended(now) && (after == nil || auctionOrder(keyOf(a), *after) > 0) {
			found = append(found, a)
		}
	}

	slices.SortFunc(found, func(a, b *user.GiftAuction) int { return auctionOrder(keyOf(a), keyOf(b)) })

	return found[:min(len(found), limit)], nil
}

func (r *GiftRepository) DueAuctions(ctx context.Context, now time.Time, limit int) ([]uuid.UUID, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var due []*user.GiftAuction

	for _, a := range r.auctions {
		if a.Ended(now) {
			due = append(due, a)
		}
	}

	slices.SortFunc(due, func(a, b *user.GiftAuction) int { return auctionOrder(keyOf(a), keyOf(b)) })

	ids := make([]uuid.UUID, 0, min(len(due), limit))
	for _, a := range due[:min(len(due), limit)] {
		ids = append(ids, a.GiftID())
	}

	return ids, nil
}

func (r *GiftRepository) AuctionedIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	auctioned := make(map[uuid.UUID]bool)

	for _, id := range ids {
		if _, ok := r.auctions[id]; ok {
			auctioned[id] = true
		}
	}

	return auctioned, nil
}
