package user_test

import (
	"errors"
	"testing"

	"github.com/vexner67/freenet/user/internal/user"
)

func TestStickerCatalog(t *testing.T) {
	t.Parallel()

	// The site's own packs are gone: stickers are those people made.
	if catalog := user.StickerCatalog(); len(catalog) != 0 {
		t.Errorf("StickerCatalog = %v", catalog)
	}
	for _, s := range []string{"smiles", "cats", "bears", "dogs"} {
		if _, err := user.NewStickerPackID(s); !errors.Is(err, user.ErrUnknownStickerPack) {
			t.Errorf("NewStickerPackID(%s): %v", s, err)
		}
	}

	id, err := user.NewStickerPackID("0198f0c0b2c47d3a9e1f2a3b4c5d6e7f")
	if err != nil || id.Price() != 0 {
		t.Errorf("a pack of a person = %s %d, %v", id, id.Price(), err)
	}
}
