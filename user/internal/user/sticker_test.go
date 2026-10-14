package user_test

import (
	"errors"
	"testing"

	"github.com/vexner67/freenet/user/internal/user"
)

func TestStickerCatalog(t *testing.T) {
	t.Parallel()

	seen := map[user.Sticker]bool{}
	for _, p := range user.StickerCatalog() {
		if len(p.Stickers) == 0 {
			t.Errorf("pack %s has no stickers", p.ID)
		}
		if id, err := user.NewStickerPackID(p.ID.String()); err != nil || id.Price() != p.Price {
			t.Errorf("NewStickerPackID(%s) = %s %d, %v", p.ID, id, id.Price(), err)
		}
		for _, s := range p.Stickers {
			if seen[s] {
				t.Errorf("sticker %s is in two packs", s)
			}
			seen[s] = true
		}
	}

	if _, err := user.NewStickerPackID("dogs"); !errors.Is(err, user.ErrUnknownStickerPack) {
		t.Errorf("an unknown pack: %v", err)
	}

	// The catalogue is a copy.
	user.StickerCatalog()[0].Stickers[0] = "x"
	if user.StickerCatalog()[0].Stickers[0] == "x" {
		t.Error("the catalogue changed")
	}
}
