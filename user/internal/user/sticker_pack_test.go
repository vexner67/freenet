package user_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestStickerPackName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want user.StickerPackName
		err  error
	}{
		{in: "  Мои\n котики ", want: "Мои котики"},
		{in: " \t", err: user.ErrStickerPackNameEmpty},
		{in: strings.Repeat("я", user.MaxStickerPackNameLength), want: user.StickerPackName(strings.Repeat("я", user.MaxStickerPackNameLength))},
		{in: strings.Repeat("я", user.MaxStickerPackNameLength+1), err: user.ErrStickerPackNameTooLong},
	}

	for _, tt := range tests {
		got, err := user.NewStickerPackName(tt.in)
		if got != tt.want || !errors.Is(err, tt.err) {
			t.Errorf("NewStickerPackName(%q) = %q, %v; want %q, %v", tt.in, got, err, tt.want, tt.err)
		}
	}

	if _, err := user.NewStickerPackAbout(strings.Repeat("я", user.MaxStickerPackAboutLength+1)); !errors.Is(err, user.ErrStickerPackAboutTooLong) {
		t.Errorf("a long description: %v", err)
	}
}

func TestCustomStickerPack(t *testing.T) {
	t.Parallel()

	author, other := uuid.New(), uuid.New()
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	draft := user.StickerPackDraft{Name: "Котики", Price: 5, Published: true}

	p, err := user.NewCustomStickerPack(author, draft, []uuid.UUID{a, b, a})
	if err != nil {
		t.Fatal(err)
	}

	// A photo is one sticker, and the pack is named as its stickers are.
	if photos := p.Photos(); len(photos) != 2 || photos[0] != a || photos[1] != b {
		t.Fatalf("Photos = %v", photos)
	}

	pack := p.Pack()
	if pack.Author != author || pack.Price != 5 || pack.Name != "Котики" || !pack.Published || len(pack.Stickers) != 2 {
		t.Fatalf("Pack = %+v", pack)
	}
	if id, err := user.NewStickerPackID(pack.ID.String()); err != nil || id != pack.ID {
		t.Fatalf("NewStickerPackID(%s) = %s, %v", pack.ID, id, err)
	}
	if id, ok := pack.ID.Custom(); !ok || id != p.UUID() {
		t.Fatalf("Custom = %s, %v", id, ok)
	}
	if photo, ok := pack.Stickers[1].Photo(); !ok || photo != b {
		t.Fatalf("Photo = %s, %v", photo, ok)
	}
	if _, ok := user.Sticker("cat_hi").Photo(); ok {
		t.Fatal("a sticker of the catalogue is a photo")
	}

	for _, tt := range []struct {
		name   string
		draft  user.StickerPackDraft
		photos []uuid.UUID
		err    error
	}{
		{name: "no stickers", draft: draft, err: user.ErrStickerPackEmpty},
		{name: "a negative price", draft: user.StickerPackDraft{Name: "x", Price: -1}, photos: []uuid.UUID{a}, err: user.ErrStickerPackPrice},
		{name: "too dear", draft: user.StickerPackDraft{Name: "x", Price: user.MaxStickerPackPrice + 1}, photos: []uuid.UUID{a}, err: user.ErrStickerPackPrice},
		{name: "no name", photos: []uuid.UUID{a}, err: user.ErrStickerPackNameEmpty},
		{name: "too many", draft: draft, photos: manyIDs(user.MaxPackStickers + 1), err: user.ErrTooManyPackStickers},
	} {
		if _, err := user.NewCustomStickerPack(author, tt.draft, tt.photos); !errors.Is(err, tt.err) {
			t.Errorf("%s: %v, want %v", tt.name, err, tt.err)
		}
	}

	// Edited adds after the stickers and tells what went.
	changed := user.StickerPackDraft{Name: "Кошки"}
	edited, gone, err := p.Edited(author, changed, []uuid.UUID{c, b}, []uuid.UUID{a, uuid.New()})
	if err != nil {
		t.Fatal(err)
	}
	if photos := edited.Photos(); len(photos) != 2 || photos[0] != b || photos[1] != c || len(gone) != 1 || gone[0] != a {
		t.Fatalf("Edited = %v, gone %v", photos, gone)
	}
	if edited.Draft() != changed || edited.UUID() != p.UUID() || len(p.Photos()) != 2 || p.Photos()[0] != a {
		t.Fatalf("Edited changed the pack: %+v, %v", edited.Draft(), p.Photos())
	}

	if _, _, err = p.Edited(other, draft, nil, nil); !errors.Is(err, user.ErrCannotChangeStickerPack) {
		t.Fatalf("Edited by another: %v", err)
	}
	if _, _, err = p.Edited(author, draft, nil, []uuid.UUID{a, b}); !errors.Is(err, user.ErrStickerPackEmpty) {
		t.Fatalf("Edited without stickers: %v", err)
	}
}

func manyIDs(n int) []uuid.UUID {
	ids := make([]uuid.UUID, n)
	for i := range ids {
		ids[i] = uuid.New()
	}

	return ids
}
