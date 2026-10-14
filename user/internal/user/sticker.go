package user

import (
	"errors"
	"slices"
)

var ErrUnknownStickerPack = errors.New("unknown sticker pack")

// StickerPackID names a pack of the catalogue of stickers.
type StickerPackID string

// Sticker names a sticker of the catalogue: what a message shows.
type Sticker string

// StickerPack is a pack of openvk's "Магазин стикеров": stickers that
// come together and cost Price votes once, 0 for a free one.
type StickerPack struct {
	ID       StickerPackID
	Price    int64
	Stickers []Sticker
}

// stickerCatalog is all there is. openvk's packs are drawn, priced and
// put on sale by their authors and administrators; here the catalogue is
// fixed, as that of gifts: the smiles are free, so that anyone has
// stickers, and the rest costs votes. The pictures are the site's: it
// knows a sticker by this name.
var stickerCatalog = []StickerPack{
	{ID: "smiles", Stickers: []Sticker{
		"smile_happy", "smile_laugh", "smile_wink", "smile_love", "smile_cool", "smile_wow", "smile_sad", "smile_angry",
	}},
	{ID: "cats", Price: 2, Stickers: []Sticker{"cat_hi", "cat_love", "cat_lol", "cat_wow", "cat_sleep", "cat_angry"}},
	{ID: "bears", Price: 4, Stickers: []Sticker{"bear_hi", "bear_hug", "bear_love", "bear_ok", "bear_party", "bear_sad"}},
}

// StickerCatalog are the packs of stickers, in the order they are shown.
func StickerCatalog() []StickerPack {
	catalog := make([]StickerPack, len(stickerCatalog))
	for i, p := range stickerCatalog {
		catalog[i] = StickerPack{ID: p.ID, Price: p.Price, Stickers: slices.Clone(p.Stickers)}
	}

	return catalog
}

// NewStickerPackID takes only a pack of the catalogue.
func NewStickerPackID(s string) (StickerPackID, error) {
	for _, p := range stickerCatalog {
		if p.ID == StickerPackID(s) {
			return p.ID, nil
		}
	}

	return "", ErrUnknownStickerPack
}

func (id StickerPackID) String() string {
	return string(id)
}

// Price is what the pack costs, in votes; 0 for a free one.
func (id StickerPackID) Price() int64 {
	for _, p := range stickerCatalog {
		if p.ID == id {
			return p.Price
		}
	}

	return 0
}

func (s Sticker) String() string {
	return string(s)
}

// OwnedPack is a pack someone has taken, free or bought. Added tells
// whether it is among their stickers: openvk's "Убрать набор" puts it
// away, and a bought pack comes back free.
type OwnedPack struct {
	Pack  StickerPackID
	Added bool
}
