package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/vexner67/freenet/world/internal/inmem"
	"github.com/vexner67/freenet/world/internal/service"
	"github.com/vexner67/freenet/world/internal/world"
)

// line is an atlas of a strip of land 10 provinces long, 0 to 9, each of
// 100 thousand people.
func line() *world.Atlas {
	a := &world.Atlas{Width: 64, Height: 32}
	for i := range 10 {
		p := world.Province{ID: i, Name: "P", Terrain: world.TerrainPlains, Population: 100}
		if i > 0 {
			p.Links = append(p.Links, world.Link{To: i - 1})
		}
		if i < 9 {
			p.Links = append(p.Links, world.Link{To: i + 1})
		}
		a.Provinces = append(a.Provinces, p)
	}

	return a
}

type fakeMaker struct{ err error }

func (m fakeMaker) Make(context.Context, uint64, int) (service.MadeMap, error) {
	if m.err != nil {
		return service.MadeMap{}, m.err
	}

	return service.MadeMap{Atlas: line(), Raster: []byte{1}, Tiles: []service.Tile{{Z: 0, X: 1, Y: 0, PNG: []byte{2}}}}, nil
}

type env struct {
	worlds *service.WorldService
	store  *inmem.Store
}

func newEnv(t *testing.T, maker service.MapMaker) env {
	t.Helper()

	store := inmem.NewStore()
	worlds := service.NewWorldService(service.WorldDeps{
		Worlds: store, Maps: store, Maker: maker, Tx: inmem.TxManager{},
	})

	return env{worlds: worlds, store: store}
}

// open makes a world with its map and opens it.
func (e env) open(t *testing.T) *world.World {
	t.Helper()

	ctx := context.Background()
	w, err := e.worlds.Create(ctx, 0, 2048)
	if err != nil {
		t.Fatal(err)
	}
	if made, err := e.worlds.Generate(ctx); err != nil || !made {
		t.Fatalf("Generate = %v, %v", made, err)
	}
	if w, err = e.worlds.Open(ctx, w.ID()); err != nil {
		t.Fatal(err)
	}

	return w
}

func TestWorldService(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	e := newEnv(t, fakeMaker{})

	if _, err := e.worlds.Create(ctx, 1, 100); !errors.Is(err, world.ErrGrid) {
		t.Fatalf("Create on a bad grid: %v", err)
	}
	if made, err := e.worlds.Generate(ctx); err != nil || made {
		t.Fatalf("Generate of nothing = %v, %v", made, err)
	}
	if _, err := e.worlds.Current(ctx); !errors.Is(err, world.ErrNoWorld) {
		t.Fatalf("Current without a world: %v", err)
	}

	first := e.open(t)
	if w, err := e.worlds.Current(ctx); err != nil || w.ID() != first.ID() {
		t.Fatalf("Current = %v, %v", w, err)
	}
	if a, err := e.worlds.Atlas(ctx, first.ID()); err != nil || len(a.Provinces) != 10 {
		t.Fatalf("Atlas = %v, %v", a, err)
	}
	if tile, err := e.worlds.Tile(ctx, first.ID(), 0, 1, 0); err != nil || len(tile) != 1 {
		t.Fatalf("Tile = %v, %v", tile, err)
	}
	if _, err := e.worlds.Tile(ctx, first.ID(), 3, 0, 0); !errors.Is(err, world.ErrTileNotFound) {
		t.Fatalf("Tile of nothing: %v", err)
	}
	if r, err := e.worlds.Raster(ctx, first.ID()); err != nil || len(r) != 1 {
		t.Fatalf("Raster = %v, %v", r, err)
	}

	// A second world opened ends the first.
	second := e.open(t)
	ws, err := e.worlds.Worlds(ctx)
	if err != nil || len(ws) != 2 || ws[0].ID() != second.ID() || ws[1].Status() != world.StatusEnded {
		t.Fatalf("Worlds = %v, %v", ws, err)
	}
	if _, err = e.worlds.Open(ctx, first.ID()); !errors.Is(err, world.ErrNotReady) {
		t.Fatalf("Open of an ended world: %v", err)
	}

	if err = e.worlds.End(ctx); err != nil {
		t.Fatal(err)
	}
	if err = e.worlds.End(ctx); !errors.Is(err, world.ErrNoWorld) {
		t.Fatalf("End without a world: %v", err)
	}
}

func TestGenerateFails(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	e := newEnv(t, fakeMaker{err: errors.New("out of memory")})
	w, err := e.worlds.Create(ctx, 5, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if made, err := e.worlds.Generate(ctx); err == nil || !made {
		t.Fatalf("Generate = %v, %v", made, err)
	}
	if got, _ := e.store.World(ctx, w.ID()); got.Status() != world.StatusFailed {
		t.Fatalf("the world is %s", got.Status())
	}
}
