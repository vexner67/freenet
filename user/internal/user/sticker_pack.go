package user

import (
	"errors"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	// MaxStickerPackNameLength and MaxStickerPackAboutLength bound what a
	// pack is called and how it is described.
	MaxStickerPackNameLength  = 64
	MaxStickerPackAboutLength = 500
	// MaxStickerPackPrice is the most a pack may cost, in votes.
	MaxStickerPackPrice = MaxAmount
	// MaxPackStickers is how many stickers a pack of a person holds.
	MaxPackStickers = 40
	// MaxStickerPacks is how many packs one person makes.
	MaxStickerPacks = 20
)

var (
	ErrStickerPackNameEmpty    = errors.New("sticker pack name is empty")
	ErrStickerPackNameTooLong  = errors.New("sticker pack name is too long")
	ErrStickerPackAboutTooLong = errors.New("sticker pack description is too long")
	ErrStickerPackPrice        = errors.New("invalid sticker pack price")
	ErrStickerPackEmpty        = errors.New("a sticker pack needs a sticker")
	ErrTooManyPackStickers     = errors.New("too many stickers in the pack")
	ErrTooManyStickerPacks     = errors.New("too many sticker packs")
	// ErrCannotChangeStickerPack: a pack is changed by its author only.
	ErrCannotChangeStickerPack = errors.New("only the author can change a sticker pack")
	// ErrStickerPackTaken: others have taken the pack, and what they took
	// stays theirs — its stickers are not removed, nor is it deleted.
	ErrStickerPackTaken = errors.New("the sticker pack is taken by others")
)

// customPackID is how a pack of a person is named: its UUID in 32 lower
// hexadecimal digits, as its stickers are, which a message takes as a
// sticker name and the packs of the catalogue are not.
var customPackID = regexp.MustCompile(`^[0-9a-f]{32}$`)

// StickerPackName is what a pack of a person is called: one line.
type StickerPackName string

func NewStickerPackName(s string) (StickerPackName, error) {
	s = strings.Join(strings.Fields(s), " ")

	switch {
	case s == "":
		return "", ErrStickerPackNameEmpty
	case utf8.RuneCountInString(s) > MaxStickerPackNameLength:
		return "", ErrStickerPackNameTooLong
	}

	return StickerPackName(s), nil
}

func (n StickerPackName) String() string {
	return string(n)
}

// StickerPackAbout is the description of a pack, possibly empty.
type StickerPackAbout string

func NewStickerPackAbout(s string) (StickerPackAbout, error) {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > MaxStickerPackAboutLength {
		return "", ErrStickerPackAboutTooLong
	}

	return StickerPackAbout(s), nil
}

func (a StickerPackAbout) String() string {
	return string(a)
}

// StickerPackDraft is what the author says of a pack: what it is called,
// what it costs, in votes (0 for a free one), and whether the "Магазин
// стикеров" shows it.
type StickerPackDraft struct {
	Name      StickerPackName
	About     StickerPackAbout
	Price     int64
	Published bool
}

// CustomStickerPack is a pack a person made of their photos (openvk's
// "Студия стикеров"): each sticker is a photo, known by its ID.
type CustomStickerPack struct {
	id       uuid.UUID
	author   uuid.UUID
	draft    StickerPackDraft
	stickers []uuid.UUID
}

// NewCustomStickerPack is a pack of authorID of the photos given, in
// their order.
func NewCustomStickerPack(authorID uuid.UUID, d StickerPackDraft, photoIDs []uuid.UUID) (*CustomStickerPack, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	return newCustomStickerPack(id, authorID, d, photoIDs)
}

func newCustomStickerPack(id, authorID uuid.UUID, d StickerPackDraft, photoIDs []uuid.UUID) (*CustomStickerPack, error) {
	if d.Price < 0 || d.Price > MaxStickerPackPrice {
		return nil, ErrStickerPackPrice
	}

	if d.Name == "" {
		return nil, ErrStickerPackNameEmpty
	}

	var stickers []uuid.UUID
	for _, photo := range photoIDs {
		if !slices.Contains(stickers, photo) {
			stickers = append(stickers, photo)
		}
	}

	switch {
	case len(stickers) == 0:
		return nil, ErrStickerPackEmpty
	case len(stickers) > MaxPackStickers:
		return nil, ErrTooManyPackStickers
	}

	return &CustomStickerPack{id: id, author: authorID, draft: d, stickers: stickers}, nil
}

// RestoreCustomStickerPack is a pack as stored.
func RestoreCustomStickerPack(id, authorID uuid.UUID, d StickerPackDraft, photoIDs []uuid.UUID) *CustomStickerPack {
	return &CustomStickerPack{id: id, author: authorID, draft: d, stickers: slices.Clone(photoIDs)}
}

func (p *CustomStickerPack) UUID() uuid.UUID {
	return p.id
}

// ID is the name of the pack among the others.
func (p *CustomStickerPack) ID() StickerPackID {
	return StickerPackID(hex(p.id))
}

func (p *CustomStickerPack) AuthorID() uuid.UUID {
	return p.author
}

func (p *CustomStickerPack) Draft() StickerPackDraft {
	return p.draft
}

// Photos are the photos its stickers are, in the order they are shown.
func (p *CustomStickerPack) Photos() []uuid.UUID {
	return slices.Clone(p.stickers)
}

// CheckChange tells whether userID may change or delete the pack.
func (p *CustomStickerPack) CheckChange(userID uuid.UUID) error {
	if userID != p.author {
		return ErrCannotChangeStickerPack
	}

	return nil
}

// Edited is the pack as its author says now, with the photos added after
// the others and those removed gone; removed are the photos it no longer
// has, whose files go. A photo it has is not added again, one it does not
// have is not removed.
func (p *CustomStickerPack) Edited(userID uuid.UUID, d StickerPackDraft, added, removed []uuid.UUID) (edited *CustomStickerPack, gone []uuid.UUID, err error) {
	if err = p.CheckChange(userID); err != nil {
		return nil, nil, err
	}

	var stickers []uuid.UUID
	for _, id := range p.stickers {
		if slices.Contains(removed, id) {
			gone = append(gone, id)
			continue
		}

		stickers = append(stickers, id)
	}

	if edited, err = newCustomStickerPack(p.id, p.author, d, append(stickers, added...)); err != nil {
		return nil, nil, err
	}

	return edited, gone, nil
}

// Pack is the pack as the catalogue shows it.
func (p *CustomStickerPack) Pack() StickerPack {
	stickers := make([]Sticker, len(p.stickers))
	for i, id := range p.stickers {
		stickers[i] = StickerOf(id)
	}

	return StickerPack{
		ID:        p.ID(),
		Price:     p.draft.Price,
		Stickers:  stickers,
		Author:    p.author,
		Name:      p.draft.Name.String(),
		About:     p.draft.About.String(),
		Published: p.draft.Published,
	}
}

// StickerOf is the sticker a photo of a pack is.
func StickerOf(photoID uuid.UUID) Sticker {
	return Sticker(hex(photoID))
}

// Photo is the photo a sticker of a person's pack is; false for a sticker
// of the catalogue.
func (s Sticker) Photo() (uuid.UUID, bool) {
	return parseHex(string(s))
}

// Custom is the pack of a person the ID names; false for the catalogue.
func (id StickerPackID) Custom() (uuid.UUID, bool) {
	return parseHex(string(id))
}

func hex(id uuid.UUID) string {
	return strings.ReplaceAll(id.String(), "-", "")
}

func parseHex(s string) (uuid.UUID, bool) {
	if !customPackID.MatchString(s) {
		return uuid.Nil, false
	}

	id, err := uuid.Parse(s)

	return id, err == nil
}
