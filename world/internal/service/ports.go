// Package service holds the use cases of «Мир»: the worlds the
// administrators make and open, and the states people found in them.
package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/world/internal/world"
)

// What the user service says of a page asked for votes.
var (
	ErrNotEnoughVotes = errors.New("not enough votes")
	ErrNotSettled     = errors.New("the page is too new to found a state")
	ErrPageGone       = errors.New("this page is deleted")
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

// GameRepository keeps the game of every open world.
type GameRepository interface {
	CreateGame(ctx context.Context, worldID uuid.UUID, g *world.Game) error
	// Game is world.ErrNotFound; lock holds it until the transaction
	// ends, so that two changes do not overwrite each other.
	Game(ctx context.Context, worldID uuid.UUID, lock bool) (*world.Game, error)
	SaveGame(ctx context.Context, worldID uuid.UUID, g *world.Game) error
}

// EventRepository keeps the news of the worlds.
type EventRepository interface {
	AddEvents(ctx context.Context, worldID uuid.UUID, events []world.Event) error
	// Events are up to limit events of the world, of the state unless it
	// is uuid.Nil, with IDs below before (uuid.Nil: from the newest),
	// the newest first.
	Events(ctx context.Context, worldID, stateID, before uuid.UUID, limit int) ([]world.Event, error)
}

// RulesRepository keeps the rules of «Мир».
type RulesRepository interface {
	// Rules are world.DefaultRules until set.
	Rules(ctx context.Context) (world.Rules, error)
	SetRules(ctx context.Context, r world.Rules) error
}

// VoteOp is an operation of votes of a page in the user service; OpID
// makes it happen once.
type VoteOp struct {
	OpID   uuid.UUID
	UserID uuid.UUID
	Votes  int64
	Note   string
	// Settled asks that the page be old enough to found a state.
	Settled bool
}

// Wallets take and pay the votes of pages.
type Wallets interface {
	// Charge returns the votes left; ErrNotEnoughVotes, ErrNotSettled,
	// ErrPageGone.
	Charge(ctx context.Context, op VoteOp) (int64, error)
	Pay(ctx context.Context, op VoteOp) (int64, error)
}

// Clubs make the groups of the states.
type Clubs interface {
	// CreateStateClub returns the ID of the group; asked again, it makes
	// none.
	CreateStateClub(ctx context.Context, stateID, ownerID uuid.UUID, name, description string) (uuid.UUID, error)
}
