// Package inmem keeps the worlds in process memory, for development and
// tests: everything is lost on restart.
package inmem

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/world/internal/service"
	"github.com/vexner67/freenet/world/internal/world"
)

// TxManager runs fn as it is: memory has no transactions.
type TxManager struct{}

func (TxManager) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

type mapKey struct {
	world   uuid.UUID
	z, x, y int
}

// Store keeps the worlds and their maps.
type Store struct {
	mu      sync.Mutex
	worlds  map[uuid.UUID]world.Stored
	leases  map[uuid.UUID]time.Time
	atlases map[uuid.UUID]*world.Atlas
	rasters map[uuid.UUID][]byte
	tiles   map[mapKey][]byte
}

func NewStore() *Store {
	return &Store{
		worlds: map[uuid.UUID]world.Stored{}, leases: map[uuid.UUID]time.Time{}, atlases: map[uuid.UUID]*world.Atlas{},
		rasters: map[uuid.UUID][]byte{}, tiles: map[mapKey][]byte{},
	}
}

func stored(w *world.World) world.Stored {
	return world.Stored{
		ID: w.ID(), Seed: w.Seed(), Grid: w.Grid(), Status: w.Status(),
		CreatedAt: w.CreatedAt(), OpenedAt: w.OpenedAt(), EndedAt: w.EndedAt(),
	}
}

func (s *Store) SaveWorld(ctx context.Context, w *world.World) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.worlds[w.ID()] = stored(w)

	return nil
}

func (s *Store) World(ctx context.Context, id uuid.UUID) (*world.World, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	w, ok := s.worlds[id]
	if !ok {
		return nil, world.ErrNotFound
	}

	return world.Restore(w), nil
}

func (s *Store) OpenWorld(ctx context.Context) (*world.World, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, w := range s.worlds {
		if w.Status == world.StatusOpen {
			return world.Restore(w), nil
		}
	}

	return nil, world.ErrNoWorld
}

func (s *Store) Worlds(ctx context.Context, limit int) ([]*world.World, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	all := make([]world.Stored, 0, len(s.worlds))
	for _, w := range s.worlds {
		all = append(all, w)
	}
	slices.SortFunc(all, func(a, b world.Stored) int { return slices.Compare(b.ID[:], a.ID[:]) })

	out := make([]*world.World, 0, min(limit, len(all)))
	for _, w := range all[:min(limit, len(all))] {
		out = append(out, world.Restore(w))
	}

	return out, nil
}

func (s *Store) ClaimGeneration(ctx context.Context, now time.Time, lease time.Duration) (*world.World, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for id, w := range s.worlds {
		if w.Status == world.StatusGenerating && !s.leases[id].After(now) {
			s.leases[id] = now.Add(lease)
			return world.Restore(w), nil
		}
	}

	return nil, world.ErrNotFound
}

func (s *Store) SaveMap(ctx context.Context, worldID uuid.UUID, m service.MadeMap) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.atlases[worldID] = m.Atlas
	s.rasters[worldID] = m.Raster
	for _, t := range m.Tiles {
		s.tiles[mapKey{worldID, t.Z, t.X, t.Y}] = t.PNG
	}

	return nil
}

func (s *Store) Atlas(ctx context.Context, worldID uuid.UUID) (*world.Atlas, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	a, ok := s.atlases[worldID]
	if !ok {
		return nil, world.ErrNotFound
	}

	counted := *a
	counted.Zooms = 0
	for k := range s.tiles {
		if k.world == worldID && k.z >= counted.Zooms {
			counted.Zooms = k.z + 1
		}
	}

	return &counted, nil
}

func (s *Store) Raster(ctx context.Context, worldID uuid.UUID) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	r, ok := s.rasters[worldID]
	if !ok {
		return nil, world.ErrNotFound
	}

	return r, nil
}

func (s *Store) Tile(ctx context.Context, worldID uuid.UUID, z, x, y int) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	t, ok := s.tiles[mapKey{worldID, z, x, y}]
	if !ok {
		return nil, world.ErrTileNotFound
	}

	return t, nil
}
