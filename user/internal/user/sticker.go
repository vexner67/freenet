package user

import (
	"errors"
	"slices"

	"github.com/google/uuid"
)

var ErrUnknownStickerPack = errors.New("unknown sticker pack")

// StickerPackID names a pack of the catalogue of stickers or one a person
// made (CustomStickerPack).
type StickerPackID string

// Sticker names a sticker of the catalogue: what a message shows.
type Sticker string

// StickerPack is a pack of openvk's "Магазин стикеров": stickers that
// come together and cost Price votes once, 0 for a free one. A pack of
// the catalogue has no Author: the site names it and draws its stickers;
// one a person made carries its name and is shown in the shop only when
// Published.
type StickerPack struct {
	ID        StickerPackID
	Price     int64
	Stickers  []Sticker
	Author    uuid.UUID
	Name      string
	About     string
	Published bool
}

// stickerCatalog is the packs of the site itself. openvk's packs are
// drawn, priced and put on sale by their authors and administrators; here
// people make them in "Студия стикеров", and the site's own packs — the
// smiles, the cats and the bears — were taken away by the owner. A pack of
// the catalogue would carry pictures of the site, which knows its stickers
// by name.
var stickerCatalog []StickerPack

// StickerCatalog are the packs of stickers, in the order they are shown.
func StickerCatalog() []StickerPack {
	catalog := make([]StickerPack, len(stickerCatalog))
	for i, p := range stickerCatalog {
		catalog[i] = StickerPack{ID: p.ID, Price: p.Price, Stickers: slices.Clone(p.Stickers), Published: true}
	}

	return catalog
}

// NewStickerPackID takes a pack of the catalogue or the name of one a
// person made; whether there is such a pack, only the packs kept tell.
func NewStickerPackID(s string) (StickerPackID, error) {
	for _, p := range stickerCatalog {
		if p.ID == StickerPackID(s) {
			return p.ID, nil
		}
	}

	if _, ok := parseHex(s); ok {
		return StickerPackID(s), nil
	}

	return "", ErrUnknownStickerPack
}

func (id StickerPackID) String() string {
	return string(id)
}

// Price is what a pack of the catalogue costs, in votes; 0 for a free one
// and for a pack of a person, whose price is kept with it.
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
