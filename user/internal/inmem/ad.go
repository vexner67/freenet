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

// AdRepository keeps the ads of the global feed in memory, and counts
// what the readers do with them.
type AdRepository struct {
	mu  sync.RWMutex
	ads map[uuid.UUID]*user.Ad
	// counts are the views and clicks of each reader of each ad.
	counts map[uuid.UUID]map[uuid.UUID]*adCount
}

type adCount struct {
	views, clicks int64
}

func NewAdRepository() *AdRepository {
	return &AdRepository{
		ads:    make(map[uuid.UUID]*user.Ad),
		counts: make(map[uuid.UUID]map[uuid.UUID]*adCount),
	}
}

func (r *AdRepository) Save(ctx context.Context, ad *user.Ad) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.ads) >= user.MaxAds {
		return user.ErrTooManyAds
	}

	r.ads[ad.ID()] = ad

	return nil
}

// Lock only reads: the in-memory store has no transactions.
func (r *AdRepository) Lock(ctx context.Context, id uuid.UUID) (*user.Ad, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	ad, ok := r.ads[id]
	if !ok {
		return nil, user.ErrAdNotFound
	}

	return ad, nil
}

func (r *AdRepository) Update(ctx context.Context, ad *user.Ad) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.ads[ad.ID()]; !ok {
		return user.ErrAdNotFound
	}

	r.ads[ad.ID()] = ad

	return nil
}

func (r *AdRepository) Delete(ctx context.Context, id uuid.UUID) (*user.Ad, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	ad, ok := r.ads[id]
	if !ok {
		return nil, user.ErrAdNotFound
	}

	delete(r.ads, id)
	delete(r.counts, id)

	return ad, nil
}

func (r *AdRepository) List(ctx context.Context, all bool) ([]*user.Ad, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	ads := make([]*user.Ad, 0, len(r.ads))
	for _, ad := range r.ads {
		if all || ad.IsActive() {
			ads = append(ads, ad)
		}
	}

	// Newest first: UUIDv7 grow with time.
	slices.SortFunc(ads, func(a, b *user.Ad) int {
		bID, aID := b.ID(), a.ID()

		return bytes.Compare(bID[:], aID[:])
	})

	return ads, nil
}

func (r *AdRepository) Record(ctx context.Context, adID, readerID uuid.UUID, event user.AdEvent) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if ad, ok := r.ads[adID]; !ok || !ad.IsActive() {
		return user.ErrAdNotFound
	}

	readers := r.counts[adID]
	if readers == nil {
		readers = make(map[uuid.UUID]*adCount)
		r.counts[adID] = readers
	}

	c := readers[readerID]
	if c == nil {
		c = &adCount{}
		readers[readerID] = c
	}

	switch event {
	case user.AdViewed:
		c.views++
	case user.AdClicked:
		c.clicks++
	}

	return nil
}

func (r *AdRepository) Stats(ctx context.Context) (map[uuid.UUID]user.AdStats, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := make(map[uuid.UUID]user.AdStats, len(r.counts))
	for adID, readers := range r.counts {
		var s user.AdStats
		for _, c := range readers {
			s.Views += c.views
			s.Clicks += c.clicks
			if c.views > 0 {
				s.Viewers++
			}
			if c.clicks > 0 {
				s.Clickers++
			}
		}
		stats[adID] = s
	}

	return stats, nil
}
