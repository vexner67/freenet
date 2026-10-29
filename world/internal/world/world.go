// Package world is the domain of «Мир»: the worlds made of a seed and
// their maps.
package world

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNotFound     = errors.New("world not found")
	ErrNoWorld      = errors.New("no world is open")
	ErrGrid         = errors.New("invalid grid of the map")
	ErrNotReady     = errors.New("the world is not ready")
	ErrNotOpen      = errors.New("the world is not open")
	ErrTileNotFound = errors.New("tile not found")
)

// Status is where a world is in its life.
type Status string

const (
	// StatusGenerating: its map is being made.
	StatusGenerating Status = "generating"
	// StatusReady: the map is made; the administrators may open it.
	StatusReady Status = "ready"
	// StatusOpen: the site shows it. One world at most is open.
	StatusOpen   Status = "open"
	StatusEnded  Status = "ended"
	StatusFailed Status = "failed"
)

// Grids are the widths of the grid a map may be made on: the full one is
// 4096 cells, but 2048 needs a quarter of the memory and time.
var Grids = []int{1024, 2048, 4096}

// World is a world of «Мир»: a seed and its map.
type World struct {
	id        uuid.UUID
	seed      uint64
	grid      int
	status    Status
	createdAt time.Time
	openedAt  time.Time
	endedAt   time.Time
}

// New is a world whose map is still to be made.
func New(seed uint64, grid int, now time.Time) (*World, error) {
	if !validGrid(grid) {
		return nil, ErrGrid
	}

	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	return &World{id: id, seed: seed, grid: grid, status: StatusGenerating, createdAt: trim(now)}, nil
}

func validGrid(grid int) bool {
	for _, g := range Grids {
		if g == grid {
			return true
		}
	}

	return false
}

// Stored is a world as storage keeps it.
type Stored struct {
	ID        uuid.UUID
	Seed      uint64
	Grid      int
	Status    Status
	CreatedAt time.Time
	OpenedAt  time.Time
	EndedAt   time.Time
}

func Restore(s Stored) *World {
	return &World{
		id: s.ID, seed: s.Seed, grid: s.Grid, status: s.Status,
		createdAt: s.CreatedAt.UTC(), openedAt: s.OpenedAt.UTC(), endedAt: s.EndedAt.UTC(),
	}
}

func (w *World) ID() uuid.UUID        { return w.id }
func (w *World) Seed() uint64         { return w.seed }
func (w *World) Grid() int            { return w.grid }
func (w *World) Status() Status       { return w.status }
func (w *World) CreatedAt() time.Time { return w.createdAt }
func (w *World) OpenedAt() time.Time  { return w.openedAt }
func (w *World) EndedAt() time.Time   { return w.endedAt }

// Made: the map is made.
func (w *World) Made() {
	if w.status == StatusGenerating {
		w.status = StatusReady
	}
}

// Failed: the map could not be made.
func (w *World) Failed() {
	if w.status == StatusGenerating {
		w.status = StatusFailed
	}
}

// Open shows the world on the site; ErrNotReady unless its map is made.
func (w *World) Open(now time.Time) error {
	if w.status != StatusReady {
		return ErrNotReady
	}

	w.status, w.openedAt = StatusOpen, trim(now)

	return nil
}

// End closes the world for good; ErrNotOpen unless it is open.
func (w *World) End(now time.Time) error {
	if w.status != StatusOpen {
		return ErrNotOpen
	}

	w.status, w.endedAt = StatusEnded, trim(now)

	return nil
}

// trim keeps what PostgreSQL keeps, so a world reads back equal.
func trim(t time.Time) time.Time {
	return t.UTC().Truncate(time.Microsecond)
}
