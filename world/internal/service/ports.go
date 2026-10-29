// Package service holds the use cases of «Мир»: the worlds the
// administrators make and open, and their maps.
package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/world/internal/world"
)

// TxManager runs fn in one transaction, which the repositories take from
// ctx.
type TxManager interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// WorldRepository keeps the worlds.
type WorldRepository interface {
	// SaveWorld writes the world, new or changed.
	SaveWorld(ctx context.Context, w *world.World) error
	// World is the world id; world.ErrNotFound.
	World(ctx context.Context, id uuid.UUID) (*world.World, error)
	// OpenWorld is the world open now; world.ErrNoWorld.
	OpenWorld(ctx context.Context) (*world.World, error)
	// Worlds are the newest limit worlds, the newest first.
	Worlds(ctx context.Context, limit int) ([]*world.World, error)
	// ClaimGeneration takes a world whose map is to be made and nobody
	// makes, so that no other replica does for lease; world.ErrNotFound
	// when there is none.
	ClaimGeneration(ctx context.Context, now time.Time, lease time.Duration) (*world.World, error)
}

// Tile is a picture of the map: column x, row y of zoom z.
type Tile struct {
	Z, X, Y int
	PNG     []byte
}

// MadeMap is what a map is made into: its provinces, the raster of which
// cell is which province, and the pictures.
type MadeMap struct {
	Atlas *world.Atlas
	// Raster is the province of every cell, row by row, as little-endian
	// uint16, gzipped.
	Raster []byte
	Tiles  []Tile
}

// MapMaker makes the map of a seed on a grid; it takes a while.
type MapMaker interface {
	Make(ctx context.Context, seed uint64, grid int) (MadeMap, error)
}

// MapRepository keeps the maps of the worlds.
type MapRepository interface {
	SaveMap(ctx context.Context, worldID uuid.UUID, m MadeMap) error
	// Atlas is world.ErrNotFound for a world without a map.
	Atlas(ctx context.Context, worldID uuid.UUID) (*world.Atlas, error)
	Raster(ctx context.Context, worldID uuid.UUID) ([]byte, error)
	// Tile is world.ErrTileNotFound.
	Tile(ctx context.Context, worldID uuid.UUID, z, x, y int) ([]byte, error)
}
