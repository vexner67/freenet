package maker_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"image/png"
	"io"
	"testing"

	"github.com/vexner67/freenet/world/internal/maker"
)

func TestNew(t *testing.T) {
	t.Parallel()

	for _, bad := range []int{0, 256, 768, 1536} {
		if _, err := maker.New(bad); !errors.Is(err, maker.ErrTopWidth) {
			t.Errorf("New(%d) = %v", bad, err)
		}
	}
	if m, err := maker.New(4096); err != nil || m.Zooms() != 4 {
		t.Errorf("New(4096) = %v, %v", m, err)
	}
}

func TestMake(t *testing.T) {
	t.Parallel()

	m, err := maker.New(1024)
	if err != nil {
		t.Fatal(err)
	}

	made, err := m.Make(context.Background(), 42, 512)
	if err != nil {
		t.Fatal(err)
	}

	a := made.Atlas
	if a.Width != 512 || a.Height != 256 || len(a.Provinces) == 0 || a.Provinces[0].Name == "" {
		t.Fatalf("Atlas = %d×%d, %d provinces", a.Width, a.Height, len(a.Provinces))
	}

	zr, err := gzip.NewReader(bytes.NewReader(made.Raster))
	if err != nil {
		t.Fatal(err)
	}
	cells, err := io.ReadAll(zr)
	if err != nil || len(cells) != 2*512*256 {
		t.Fatalf("the raster has %d bytes, %v", len(cells), err)
	}

	// Zoom 0: 2×1 tiles; zoom 1: 4×2.
	if len(made.Tiles) != 2+8 {
		t.Fatalf("%d tiles", len(made.Tiles))
	}
	for _, tile := range made.Tiles {
		img, err := png.Decode(bytes.NewReader(tile.PNG))
		if err != nil || img.Bounds().Dx() != maker.TileSize || img.Bounds().Dy() != maker.TileSize {
			t.Fatalf("tile %d/%d/%d: %v", tile.Z, tile.X, tile.Y, err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = m.Make(ctx, 42, 512); !errors.Is(err, context.Canceled) {
		t.Fatalf("Make after cancel: %v", err)
	}
}
