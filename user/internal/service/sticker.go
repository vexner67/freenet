package service

import (
	"context"
	"slices"

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

// StickerPackRepository keeps the packs people make of their photos.
type StickerPackRepository interface {
	// Create saves a new pack; user.ErrTooManyStickerPacks when its author
	// has made user.MaxStickerPacks, user.ErrNotFound for one who is gone.
	Create(ctx context.Context, p *user.CustomStickerPack) error
	// Get returns the pack, held in a transaction against a delete or a
	// removal of stickers until it ends; Lock holds it against all else
	// as well. user.ErrUnknownStickerPack when there is none.
	Get(ctx context.Context, id uuid.UUID) (*user.CustomStickerPack, error)
	Lock(ctx context.Context, id uuid.UUID) (*user.CustomStickerPack, error)
	// Update saves what Edited made of the pack.
	Update(ctx context.Context, p *user.CustomStickerPack) error
	// Delete takes the pack away, from its author's stickers too.
	Delete(ctx context.Context, id uuid.UUID) error
	// TakenByOthers tells whether anyone but its author has taken it.
	TakenByOthers(ctx context.Context, p *user.CustomStickerPack) (bool, error)
	// Visible are the packs viewerID may see: published ones of pages that
	// are not deleted, their own and those they have taken. uuid.Nil sees
	// the published ones.
	Visible(ctx context.Context, viewerID uuid.UUID) ([]*user.CustomStickerPack, error)
	// ByAuthor are the packs authorID made, newest first, with how many
	// others have taken each.
	ByAuthor(ctx context.Context, authorID uuid.UUID) ([]AuthoredPack, error)
	// Authors tells who made each of the stickers that is a photo of a
	// pack; the others are left out.
	Authors(ctx context.Context, photoIDs []uuid.UUID) (map[uuid.UUID]uuid.UUID, error)
}

// AuthoredPack is a pack in the "Студия стикеров" of its author, with
// Holders — how many others have taken it ("Всего установок").
type AuthoredPack struct {
	Pack    *user.CustomStickerPack
	Holders int
}

// StickerPackView is a pack of the catalogue as a user sees it: whether
// they have taken it and whether it is among their stickers.
type StickerPackView struct {
	Pack   user.StickerPack
	Bought bool
	Added  bool
}

// StickerPackEdit is what the author changes in a pack: what it says and
// costs, the photos added after its stickers and the stickers removed.
type StickerPackEdit struct {
	Draft   user.StickerPackDraft
	Added   []uuid.UUID
	Removed []uuid.UUID
}

// StickerService is openvk's "Стикеры": packs of stickers taken from the
// "Магазин стикеров", free or for votes, and sent in messages, and the
// "Студия стикеров", where people make packs of their photos and put them
// in the shop; the votes paid for such a pack go to its author.
type StickerService struct {
	stickers StickerRepository
	packs    StickerPackRepository
	wallets  WalletRepository
	tx       TxManager
}

func NewStickerService(stickers StickerRepository, packs StickerPackRepository, wallets WalletRepository, tx TxManager) *StickerService {
	return &StickerService{
		stickers: stickers,
		packs:    packs,
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

	custom, err := s.packs.Visible(ctx, userID)
	if err != nil {
		return nil, errs.Wrap("s.packs.Visible", err)
	}

	catalog := user.StickerCatalog()
	for _, p := range custom {
		catalog = append(catalog, p.Pack())
	}

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
// it: user.ErrNotEnoughVotes, and the pack stays untaken. The votes for a
// pack of a person go to its author. A pack of a person that is not in
// the shop is user.ErrUnknownStickerPack to all but its author and those
// who have taken it.
func (s *StickerService) Add(ctx context.Context, userID uuid.UUID, pack user.StickerPackID) error {
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		price, authorID, err := s.priceOf(ctx, userID, pack)
		if err != nil {
			return errs.Wrap("s.priceOf", err)
		}

		taken, err := s.stickers.Add(ctx, userID, pack)
		if err != nil {
			return errs.Wrap("s.stickers.Add", err)
		}

		switch {
		case !taken || price == 0 || authorID == userID:
			return nil
		case authorID == uuid.Nil:
			_, err = s.wallets.Spend(ctx, userID, price)
			return errs.Wrap("s.wallets.Spend", err)
		default:
			_, err = s.wallets.Move(ctx, userID, authorID, price)
			return errs.Wrap("s.wallets.Move", err)
		}
	})

	return errs.Wrap("s.tx.WithinTx", err)
}

// priceOf is what the pack costs userID and who is paid for it, uuid.Nil
// for the catalogue. A pack of a person is held until the transaction
// ends, so that it is not deleted under the purchase.
func (s *StickerService) priceOf(ctx context.Context, userID uuid.UUID, pack user.StickerPackID) (int64, uuid.UUID, error) {
	id, custom := pack.Custom()
	if !custom {
		return pack.Price(), uuid.Nil, nil
	}

	p, err := s.packs.Get(ctx, id)
	if err != nil {
		return 0, uuid.Nil, errs.Wrap("s.packs.Get", err)
	}

	if !p.Draft().Published && p.AuthorID() != userID {
		owned, err := s.stickers.List(ctx, userID)
		if err != nil {
			return 0, uuid.Nil, errs.Wrap("s.stickers.List", err)
		}

		if !slices.ContainsFunc(owned, func(o user.OwnedPack) bool { return o.Pack == pack }) {
			return 0, uuid.Nil, user.ErrUnknownStickerPack
		}
	}

	return p.Draft().Price, p.AuthorID(), nil
}

// Authored are the packs authorID made, for their "Студия стикеров".
func (s *StickerService) Authored(ctx context.Context, authorID uuid.UUID) ([]AuthoredPack, error) {
	packs, err := s.packs.ByAuthor(ctx, authorID)
	if err != nil {
		return nil, errs.Wrap("s.packs.ByAuthor", err)
	}

	return packs, nil
}

// Create makes a pack of photos of authorID, among whose stickers it is at
// once. That the photos are the author's, the caller has checked.
func (s *StickerService) Create(ctx context.Context, authorID uuid.UUID, d user.StickerPackDraft, photoIDs []uuid.UUID) (*user.CustomStickerPack, error) {
	p, err := user.NewCustomStickerPack(authorID, d, photoIDs)
	if err != nil {
		return nil, errs.Wrap("user.NewCustomStickerPack", err)
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.packs.Create(ctx, p); err != nil {
			return errs.Wrap("s.packs.Create", err)
		}

		_, err := s.stickers.Add(ctx, authorID, p.ID())

		return errs.Wrap("s.stickers.Add", err)
	})
	if err != nil {
		return nil, errs.Wrap("s.tx.WithinTx", err)
	}

	return p, nil
}

// Edit changes a pack of userID and returns it and the photos it no
// longer has, whose files the caller deletes. Stickers are not removed
// from a pack others have taken: user.ErrStickerPackTaken.
func (s *StickerService) Edit(ctx context.Context, userID, packID uuid.UUID, e StickerPackEdit) (*user.CustomStickerPack, []uuid.UUID, error) {
	var (
		edited *user.CustomStickerPack
		gone   []uuid.UUID
	)

	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		p, err := s.packs.Lock(ctx, packID)
		if err != nil {
			return errs.Wrap("s.packs.Lock", err)
		}

		if edited, gone, err = p.Edited(userID, e.Draft, e.Added, e.Removed); err != nil {
			return errs.Wrap("p.Edited", err)
		}

		if len(gone) > 0 {
			if err = s.checkUntaken(ctx, p); err != nil {
				return errs.Wrap("s.checkUntaken", err)
			}
		}

		return errs.Wrap("s.packs.Update", s.packs.Update(ctx, edited))
	})
	if err != nil {
		return nil, nil, errs.Wrap("s.tx.WithinTx", err)
	}

	return edited, gone, nil
}

// Delete takes a pack of userID away and returns its photos, whose files
// the caller deletes; a pack others have taken stays:
// user.ErrStickerPackTaken.
func (s *StickerService) Delete(ctx context.Context, userID, packID uuid.UUID) ([]uuid.UUID, error) {
	var photos []uuid.UUID

	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		p, err := s.packs.Lock(ctx, packID)
		if err != nil {
			return errs.Wrap("s.packs.Lock", err)
		}

		if err = p.CheckChange(userID); err != nil {
			return errs.Wrap("p.CheckChange", err)
		}

		if err = s.checkUntaken(ctx, p); err != nil {
			return errs.Wrap("s.checkUntaken", err)
		}

		photos = p.Photos()

		return errs.Wrap("s.packs.Delete", s.packs.Delete(ctx, packID))
	})
	if err != nil {
		return nil, errs.Wrap("s.tx.WithinTx", err)
	}

	return photos, nil
}

func (s *StickerService) checkUntaken(ctx context.Context, p *user.CustomStickerPack) error {
	taken, err := s.packs.TakenByOthers(ctx, p)
	if err != nil {
		return errs.Wrap("s.packs.TakenByOthers", err)
	}

	if taken {
		return user.ErrStickerPackTaken
	}

	return nil
}

// Authors tells who made each of the stickers that is a photo of a pack:
// whose photo it is. Stickers of the catalogue and unknown ones are left
// out.
func (s *StickerService) Authors(ctx context.Context, stickers []user.Sticker) (map[user.Sticker]uuid.UUID, error) {
	var photos []uuid.UUID
	for _, st := range stickers {
		if id, ok := st.Photo(); ok {
			photos = append(photos, id)
		}
	}

	if len(photos) == 0 {
		return map[user.Sticker]uuid.UUID{}, nil
	}

	authors, err := s.packs.Authors(ctx, photos)
	if err != nil {
		return nil, errs.Wrap("s.packs.Authors", err)
	}

	byName := make(map[user.Sticker]uuid.UUID, len(authors))
	for photo, author := range authors {
		byName[user.StickerOf(photo)] = author
	}

	return byName, nil
}

// Remove puts the pack away from the stickers of userID ("Убрать набор");
// a bought one is added back free.
func (s *StickerService) Remove(ctx context.Context, userID uuid.UUID, pack user.StickerPackID) error {
	return errs.Wrap("s.stickers.Remove", s.stickers.Remove(ctx, userID, pack))
}
