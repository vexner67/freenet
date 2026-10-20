package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// AdRepository keeps the ads of the global feed.
type AdRepository interface {
	// Save adds a new ad; user.ErrTooManyAds when the site has
	// user.MaxAds already.
	Save(ctx context.Context, ad *user.Ad) error
	// Lock reads an ad to change it, and keeps others from changing it
	// until the transaction ends; user.ErrAdNotFound.
	Lock(ctx context.Context, id uuid.UUID) (*user.Ad, error)
	// Update writes an ad read by Lock.
	Update(ctx context.Context, ad *user.Ad) error
	// Delete returns the ad it took away; user.ErrAdNotFound.
	Delete(ctx context.Context, id uuid.UUID) (*user.Ad, error)
	// List returns the ads, newest first; only those shown unless all.
	List(ctx context.Context, all bool) ([]*user.Ad, error)
}

// AdStatsRepository counts what the readers do with the ads.
type AdStatsRepository interface {
	// Record counts event of readerID on a shown ad; user.ErrAdNotFound
	// when it is not shown.
	Record(ctx context.Context, adID, readerID uuid.UUID, event user.AdEvent) error
	// Stats are the counts of every ad that has any.
	Stats(ctx context.Context) (map[uuid.UUID]user.AdStats, error)
}

// AdService is the advertising of the global feed: what the administrators
// of the site promote, as Twitter slips promoted tweets between the
// others. They make, change, show, hide and delete the ads; everyone
// reads the shown ones.
type AdService struct {
	userRepo UserRepository
	ads      AdRepository
	stats    AdStatsRepository
	tx       TxManager
	admins   user.Admins
}

func NewAdService(users UserRepository, ads AdRepository, stats AdStatsRepository, tx TxManager, admins user.Admins) *AdService {
	return &AdService{userRepo: users, ads: ads, stats: stats, tx: tx, admins: admins}
}

// Shown are the ads the global feed shows, newest first.
func (s *AdService) Shown(ctx context.Context) ([]*user.Ad, error) {
	ads, err := s.ads.List(ctx, false)

	return ads, errs.Wrap("s.ads.List", err)
}

// All are every ad, shown or hidden, with how each does, for the admin
// panel; an ad nobody has seen has no stats. user.ErrNotAdmin unless
// adminID runs the site.
func (s *AdService) All(ctx context.Context, adminID uuid.UUID) ([]*user.Ad, map[uuid.UUID]user.AdStats, error) {
	if err := checkAdmin(ctx, s.userRepo, s.admins, adminID); err != nil {
		return nil, nil, errs.Wrap("checkAdmin", err)
	}

	ads, err := s.ads.List(ctx, true)
	if err != nil {
		return nil, nil, errs.Wrap("s.ads.List", err)
	}

	stats, err := s.stats.Stats(ctx)
	if err != nil {
		return nil, nil, errs.Wrap("s.stats.Stats", err)
	}

	return ads, stats, nil
}

// Record counts that readerID saw a shown ad in the feed or followed its
// link; user.ErrAdNotFound when it is not shown.
func (s *AdService) Record(ctx context.Context, readerID, adID uuid.UUID, event user.AdEvent) error {
	return errs.Wrap("s.stats.Record", s.stats.Record(ctx, adID, readerID, event))
}

// Create makes an ad whose cover, if any, adminID uploaded;
// user.ErrNotAdmin, user.ErrTooManyAds.
func (s *AdService) Create(ctx context.Context, adminID uuid.UUID, d user.AdDraft) (*user.Ad, error) {
	if err := checkAdmin(ctx, s.userRepo, s.admins, adminID); err != nil {
		return nil, errs.Wrap("checkAdmin", err)
	}

	if !d.Photo.IsZero() {
		d.Photo.OwnerID = adminID
	}

	ad, err := user.NewAd(d)
	if err != nil {
		return nil, errs.Wrap("user.NewAd", err)
	}

	if err = s.ads.Save(ctx, ad); err != nil {
		return nil, errs.Wrap("s.ads.Save", err)
	}

	return ad, nil
}

// Edit changes an ad; a new cover is one adminID uploaded. It returns the
// cover the ad no longer shows (zero when it keeps it), whose file is to
// go. user.ErrNotAdmin, user.ErrAdNotFound.
func (s *AdService) Edit(ctx context.Context, adminID, id uuid.UUID, c user.AdChange) (*user.Ad, user.AdPhoto, error) {
	if err := checkAdmin(ctx, s.userRepo, s.admins, adminID); err != nil {
		return nil, user.AdPhoto{}, errs.Wrap("checkAdmin", err)
	}

	if c.Photo != nil && !c.Photo.IsZero() {
		c.Photo.OwnerID = adminID
	}

	var (
		edited  *user.Ad
		dropped user.AdPhoto
	)

	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		ad, err := s.ads.Lock(ctx, id)
		if err != nil {
			return errs.Wrap("s.ads.Lock", err)
		}

		if edited, dropped, err = ad.Edited(c); err != nil {
			return errs.Wrap("ad.Edited", err)
		}

		return errs.Wrap("s.ads.Update", s.ads.Update(ctx, edited))
	})
	if err != nil {
		return nil, user.AdPhoto{}, errs.Wrap("s.tx.WithinTx", err)
	}

	return edited, dropped, nil
}

// Delete takes an ad away and returns its cover, whose file is to go;
// user.ErrNotAdmin, user.ErrAdNotFound.
func (s *AdService) Delete(ctx context.Context, adminID, id uuid.UUID) (user.AdPhoto, error) {
	if err := checkAdmin(ctx, s.userRepo, s.admins, adminID); err != nil {
		return user.AdPhoto{}, errs.Wrap("checkAdmin", err)
	}

	ad, err := s.ads.Delete(ctx, id)
	if err != nil {
		return user.AdPhoto{}, errs.Wrap("s.ads.Delete", err)
	}

	return ad.Photo(), nil
}
