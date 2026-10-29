package service

import (
	"context"
	"errors"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/world/internal/world"
)

const (
	// MaxWorlds is how many worlds the administrators see.
	MaxWorlds = 20
	// generationLease is how long a replica may make a map before
	// another takes it over: a replica that stopped making leaves it.
	generationLease = 15 * time.Minute
)

// WorldService makes the worlds and opens them, and reads their maps. The
// administrators of the site call the changes: the gateway asks the user
// service first.
type WorldService struct {
	worlds WorldRepository
	maps   MapRepository
	maker  MapMaker
	tx     TxManager
	now    func() time.Time
	// wake tells the generation that a world waits for its map.
	wake chan struct{}

	mu sync.Mutex
	// atlases are the maps read so far: they never change.
	atlases map[uuid.UUID]*world.Atlas
}

// WorldDeps are what WorldService works with.
type WorldDeps struct {
	Worlds WorldRepository
	Maps   MapRepository
	Maker  MapMaker
	Tx     TxManager
}

func NewWorldService(d WorldDeps) *WorldService {
	return &WorldService{
		worlds: d.Worlds, maps: d.Maps, maker: d.Maker, tx: d.Tx, now: time.Now,
		wake: make(chan struct{}, 1), atlases: map[uuid.UUID]*world.Atlas{},
	}
}

// Create starts a world of seed (0: a random one) on the grid; its map is
// made in the background (Generate).
func (s *WorldService) Create(ctx context.Context, seed uint64, grid int) (*world.World, error) {
	for seed == 0 {
		seed = rand.Uint64() & (1<<63 - 1) // a seed PostgreSQL keeps as bigint
	}

	w, err := world.New(seed, grid, s.now())
	if err != nil {
		return nil, errs.Wrap("world.New", err)
	}

	if err = s.worlds.SaveWorld(ctx, w); err != nil {
		return nil, errs.Wrap("s.worlds.SaveWorld", err)
	}

	select {
	case s.wake <- struct{}{}:
	default:
	}

	return w, nil
}

// Wake is how the generation hears of a new world.
func (s *WorldService) Wake() <-chan struct{} {
	return s.wake
}

// Generate makes the map of a world waiting for one, if any, and says
// whether it did. A map that cannot be made fails its world.
func (s *WorldService) Generate(ctx context.Context) (bool, error) {
	w, err := s.worlds.ClaimGeneration(ctx, s.now(), generationLease)
	if errors.Is(err, world.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, errs.Wrap("s.worlds.ClaimGeneration", err)
	}

	m, err := s.maker.Make(ctx, w.Seed(), w.Grid())
	if err != nil {
		if ctx.Err() != nil {
			return false, errs.Wrap("s.maker.Make", err)
		}
		w.Failed()
		return true, errors.Join(errs.Wrap("s.maker.Make", err), errs.Wrap("s.worlds.SaveWorld", s.worlds.SaveWorld(ctx, w)))
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.maps.SaveMap(ctx, w.ID(), m); err != nil {
			return errs.Wrap("s.maps.SaveMap", err)
		}

		w.Made()

		return errs.Wrap("s.worlds.SaveWorld", s.worlds.SaveWorld(ctx, w))
	})

	return true, errs.Wrap("s.tx.WithinTx", err)
}

// Open opens the world id, whose map is made, and ends the one open
// before it.
func (s *WorldService) Open(ctx context.Context, id uuid.UUID) (*world.World, error) {
	var w *world.World

	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		var err error
		if w, err = s.worlds.World(ctx, id); err != nil {
			return errs.Wrap("s.worlds.World", err)
		}

		now := s.now()
		if err = w.Open(now); err != nil {
			return errs.Wrap("w.Open", err)
		}

		if err = s.endOpen(ctx, now); err != nil {
			return errs.Wrap("s.endOpen", err)
		}

		return errs.Wrap("s.worlds.SaveWorld", s.worlds.SaveWorld(ctx, w))
	})

	return w, errs.Wrap("s.tx.WithinTx", err)
}

func (s *WorldService) endOpen(ctx context.Context, now time.Time) error {
	open, err := s.worlds.OpenWorld(ctx)
	if errors.Is(err, world.ErrNoWorld) {
		return nil
	}
	if err != nil {
		return errs.Wrap("s.worlds.OpenWorld", err)
	}

	if err = open.End(now); err != nil {
		return errs.Wrap("open.End", err)
	}

	return errs.Wrap("s.worlds.SaveWorld", s.worlds.SaveWorld(ctx, open))
}

// End ends the world open now; world.ErrNoWorld.
func (s *WorldService) End(ctx context.Context) error {
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if _, err := s.worlds.OpenWorld(ctx); err != nil {
			return errs.Wrap("s.worlds.OpenWorld", err)
		}

		return s.endOpen(ctx, s.now())
	})

	return errs.Wrap("s.tx.WithinTx", err)
}

// Worlds are the newest worlds, the newest first.
func (s *WorldService) Worlds(ctx context.Context) ([]*world.World, error) {
	ws, err := s.worlds.Worlds(ctx, MaxWorlds)
	return ws, errs.Wrap("s.worlds.Worlds", err)
}

// Current is the world open now; world.ErrNoWorld.
func (s *WorldService) Current(ctx context.Context) (*world.World, error) {
	w, err := s.worlds.OpenWorld(ctx)
	return w, errs.Wrap("s.worlds.OpenWorld", err)
}

// Atlas is the map of the world id.
func (s *WorldService) Atlas(ctx context.Context, id uuid.UUID) (*world.Atlas, error) {
	s.mu.Lock()
	a, ok := s.atlases[id]
	s.mu.Unlock()
	if ok {
		return a, nil
	}

	a, err := s.maps.Atlas(ctx, id)
	if err != nil {
		return nil, errs.Wrap("s.maps.Atlas", err)
	}

	s.mu.Lock()
	s.atlases[id] = a
	s.mu.Unlock()

	return a, nil
}

func (s *WorldService) Raster(ctx context.Context, id uuid.UUID) ([]byte, error) {
	r, err := s.maps.Raster(ctx, id)
	return r, errs.Wrap("s.maps.Raster", err)
}

func (s *WorldService) Tile(ctx context.Context, id uuid.UUID, z, x, y int) ([]byte, error) {
	t, err := s.maps.Tile(ctx, id, z, x, y)
	return t, errs.Wrap("s.maps.Tile", err)
}
