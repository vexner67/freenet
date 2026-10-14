package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// StickerRepository keeps the packs of stickers each user has taken.
type StickerRepository interface {
	// Add puts the pack among the stickers of userID and tells whether it
	// was taken just now, rather than added back after "Убрать набор":
	// only then is it paid for. Two adds at once take it once.
	// user.ErrNotFound for a user who is gone.
	Add(ctx context.Context, userID uuid.UUID, pack user.StickerPackID) (bool, error)
	// Remove puts the pack away; it stays taken. Idempotent.
	Remove(ctx context.Context, userID uuid.UUID, pack user.StickerPackID) error
	// List returns the packs userID has taken.
	List(ctx context.Context, userID uuid.UUID) ([]user.OwnedPack, error)
}

// StickerPackView is a pack of the catalogue as a user sees it: whether
// they have taken it and whether it is among their stickers.
type StickerPackView struct {
	Pack   user.StickerPack
	Bought bool
	Added  bool
}

// StickerService is openvk's "Стикеры": packs of stickers taken from the
// "Магазин стикеров", free or for votes, and sent in messages.
type StickerService struct {
	stickers StickerRepository
	wallets  WalletRepository
	tx       TxManager
}

func NewStickerService(stickers StickerRepository, wallets WalletRepository, tx TxManager) *StickerService {
	return &StickerService{
		stickers: stickers,
		wallets:  wallets,
		tx:       tx,
	}
}

// Catalog is every pack as userID sees it; uuid.Nil sees them all
// untaken.
func (s *StickerService) Catalog(ctx context.Context, userID uuid.UUID) ([]StickerPackView, error) {
	var owned []user.OwnedPack

	if userID != uuid.Nil {
		var err error
		if owned, err = s.stickers.List(ctx, userID); err != nil {
			return nil, errs.Wrap("s.stickers.List", err)
		}
	}

	catalog := user.StickerCatalog()
	views := make([]StickerPackView, len(catalog))

	for i, p := range catalog {
		views[i] = StickerPackView{Pack: p}

		for _, o := range owned {
			if o.Pack == p.ID {
				views[i].Bought, views[i].Added = true, o.Added
			}
		}
	}

	return views, nil
}

// Add puts the pack among the stickers of userID. A pack with a price is
// paid for with their votes the first time, in the transaction that takes
// it: user.ErrNotEnoughVotes, and the pack stays untaken.
func (s *StickerService) Add(ctx context.Context, userID uuid.UUID, pack user.StickerPackID) error {
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		taken, err := s.stickers.Add(ctx, userID, pack)
		if err != nil {
			return errs.Wrap("s.stickers.Add", err)
		}

		if price := pack.Price(); taken && price > 0 {
			if _, err := s.wallets.Spend(ctx, userID, price); err != nil {
				return errs.Wrap("s.wallets.Spend", err)
			}
		}

		return nil
	})

	return errs.Wrap("s.tx.WithinTx", err)
}

// Remove puts the pack away from the stickers of userID ("Убрать набор");
// a bought one is added back free.
func (s *StickerService) Remove(ctx context.Context, userID uuid.UUID, pack user.StickerPackID) error {
	return errs.Wrap("s.stickers.Remove", s.stickers.Remove(ctx, userID, pack))
}
