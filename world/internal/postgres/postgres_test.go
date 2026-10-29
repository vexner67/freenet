package postgres_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/world/internal/postgres"
	"github.com/vexner67/freenet/world/internal/service"
	"github.com/vexner67/freenet/world/internal/world"
)

// Integration test: needs a database with migrations applied, e.g.
// TEST_DATABASE_URL=postgres://... go test ./internal/postgres
func newPool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)

	pool, err := pgpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	// The tests share one database: start without an open world.
	if _, err = pool.Exec(ctx, `UPDATE worlds SET status = 'ended' WHERE status = 'open'`); err != nil {
		t.Fatal(err)
	}

	return pool, ctx
}

func line() *world.Atlas {
	a := &world.Atlas{Width: 64, Height: 32}
	for i := range 5 {
		p := world.Province{
			ID: i, Name: "Серые Холмы", Terrain: world.TerrainHills, Biome: "steppe", Population: 40,
			Resources: []world.Resource{world.ResourceOre}, CenterX: i, CenterY: 3, Area: 9, Culture: 2,
		}
		if i > 0 {
			p.Links = append(p.Links, world.Link{To: i - 1, River: true})
		}
		if i < 4 {
			p.Links = append(p.Links, world.Link{To: i + 1})
		}
		a.Provinces = append(a.Provinces, p)
	}
	a.Provinces[4].FreeCity = true

	return a
}

func TestRepositories(t *testing.T) {
	pool, ctx := newPool(t)
	worlds := postgres.NewWorldRepository(pool)
	maps := postgres.NewMapRepository(pool)
	games := postgres.NewGameRepository(pool)
	events := postgres.NewEventRepository(pool)
	tx := pgpool.NewTxManager(pool)
	now := time.Now().UTC()

	w, err := world.New(1<<63+5, 2048, now)
	if err != nil {
		t.Fatal(err)
	}
	if err = worlds.SaveWorld(ctx, w); err != nil {
		t.Fatal(err)
	}

	got, err := worlds.World(ctx, w.ID())
	if err != nil || got.Seed() != w.Seed() || got.Grid() != 2048 || got.Status() != world.StatusGenerating || !got.CreatedAt().Equal(w.CreatedAt()) {
		t.Fatalf("World = %+v, %v", got, err)
	}
	if _, err = worlds.World(ctx, uuid.Must(uuid.NewV7())); !errors.Is(err, world.ErrNotFound) {
		t.Fatalf("World of nothing: %v", err)
	}

	t.Run("one replica claims a world at a time", func(t *testing.T) {
		claimed, err := worlds.ClaimGeneration(ctx, now, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		for {
			// Worlds of other runs may wait too: claim them all.
			if _, err = worlds.ClaimGeneration(ctx, now, time.Minute); errors.Is(err, world.ErrNotFound) {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
		}
		if claimed.ID() != w.ID() {
			if again, err := worlds.ClaimGeneration(ctx, now.Add(2*time.Minute), time.Minute); err != nil || again == nil {
				t.Fatalf("ClaimGeneration after the lease = %v, %v", again, err)
			}
		}
	})

	tile := service.Tile{Z: 1, X: 2, Y: 1, PNG: []byte{9, 9}}
	err = tx.WithinTx(ctx, func(ctx context.Context) error {
		return maps.SaveMap(ctx, w.ID(), service.MadeMap{Atlas: line(), Raster: []byte{1, 2, 3}, Tiles: []service.Tile{tile}})
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("the map reads back", func(t *testing.T) {
		a, err := maps.Atlas(ctx, w.ID())
		if err != nil || len(a.Provinces) != 5 || a.Provinces[1].Links[0] != (world.Link{To: 0, River: true}) ||
			a.Provinces[2].Resources[0] != world.ResourceOre || !a.Provinces[4].FreeCity || a.Width != 64 || a.Provinces[3].Name != "Серые Холмы" || a.Zooms != 2 {
			t.Fatalf("Atlas = %+v, %v", a, err)
		}
		if r, err := maps.Raster(ctx, w.ID()); err != nil || len(r) != 3 {
			t.Fatalf("Raster = %v, %v", r, err)
		}
		if png, err := maps.Tile(ctx, w.ID(), 1, 2, 1); err != nil || len(png) != 2 {
			t.Fatalf("Tile = %v, %v", png, err)
		}
		if _, err := maps.Tile(ctx, w.ID(), 0, 0, 0); !errors.Is(err, world.ErrTileNotFound) {
			t.Fatalf("Tile of nothing: %v", err)
		}
		if _, err := maps.Atlas(ctx, uuid.Must(uuid.NewV7())); !errors.Is(err, world.ErrNotFound) {
			t.Fatalf("Atlas of nothing: %v", err)
		}
	})

	w.Made()
	if err = w.Open(now); err != nil {
		t.Fatal(err)
	}
	if err = worlds.SaveWorld(ctx, w); err != nil {
		t.Fatal(err)
	}
	if open, err := worlds.OpenWorld(ctx); err != nil || open.ID() != w.ID() || !open.OpenedAt().Equal(w.OpenedAt()) {
		t.Fatalf("OpenWorld = %v, %v", open, err)
	}
	if list, err := worlds.Worlds(ctx, 1); err != nil || len(list) != 1 {
		t.Fatalf("Worlds = %v, %v", list, err)
	}

	t.Run("a game reads back whole", func(t *testing.T) {
		g := world.NewGame(now)
		if err := games.CreateGame(ctx, w.ID(), g); err != nil {
			t.Fatal(err)
		}

		name, _ := world.NewStateName("Бореалия")
		flag, _ := world.NewFlag(world.FlagParts{Pattern: world.PatternNordic, Colors: []int{1, 2}, Emblem: "star", EmblemColor: 3, EmblemAt: world.EmblemCanton})
		ruler, other := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
		f := world.Founding{ID: uuid.Must(uuid.NewV7()), RulerID: ruler, Name: name, Motto: "Вперёд", Color: 5, Flag: flag, Government: world.GovernmentFederation, Capital: 1}

		err := tx.WithinTx(ctx, func(ctx context.Context) error {
			g, err := games.Game(ctx, w.ID(), true)
			if err != nil {
				return err
			}
			if err = g.Found(line(), world.DefaultRules, f, now); err != nil {
				return err
			}
			if err = g.Apply(other, f.ID, now); err != nil {
				return err
			}
			if err = g.Colonize(line(), world.DefaultRules, ruler, 3, now); err != nil {
				return err
			}
			if err = games.SaveGame(ctx, w.ID(), g); err != nil {
				return err
			}
			return events.AddEvents(ctx, w.ID(), g.TakeEvents())
		})
		if err != nil {
			t.Fatal(err)
		}

		back, err := games.Game(ctx, w.ID(), false)
		if err != nil {
			t.Fatal(err)
		}
		s := back.Snapshot()
		st := s.States[0]
		if len(s.States) != 1 || st.Name != "Бореалия" || st.Motto != "Вперёд" || st.Color != 5 || st.Government != world.GovernmentFederation ||
			st.Flag.Parts().EmblemAt != world.EmblemCanton || st.Flag.Parts().Colors[1] != 2 || st.Treasury != 100000-26000 ||
			len(st.Budget) != 1 || st.Budget[0].Colonization != 26000 || !st.FoundedAt.Equal(now.Truncate(time.Microsecond)) {
			t.Fatalf("state = %+v", st)
		}
		if len(s.Owners) != 3 || s.Owners[1] != f.ID || len(s.Citizens) != 1 || s.Citizens[0].Role != world.RoleRuler ||
			len(s.Applications) != 1 || len(s.Colonies) != 1 || s.Colonies[0].Province != 3 || !s.Hour.Equal(g.Hour()) {
			t.Fatalf("game = %+v", s)
		}

		news, err := events.Events(ctx, w.ID(), f.ID, uuid.Nil, 10)
		if err != nil || len(news) != 1 || news[0].Kind != world.EventFounded || news[0].UserID != ruler || news[0].Province != 1 {
			t.Fatalf("Events = %+v, %v", news, err)
		}
		if news, err = events.Events(ctx, w.ID(), uuid.Nil, news[0].ID, 10); err != nil || len(news) != 0 {
			t.Fatalf("Events before the first = %+v, %v", news, err)
		}
	})

	if err = w.End(now); err != nil {
		t.Fatal(err)
	}
	if err = worlds.SaveWorld(ctx, w); err != nil {
		t.Fatal(err)
	}
	if _, err = games.Game(ctx, uuid.Must(uuid.NewV7()), false); !errors.Is(err, world.ErrNotFound) {
		t.Fatalf("Game of nothing: %v", err)
	}
	if err = games.SaveGame(ctx, uuid.Must(uuid.NewV7()), world.NewGame(now)); !errors.Is(err, world.ErrNotFound) {
		t.Fatalf("SaveGame of nothing: %v", err)
	}
}

func TestRulesRepository(t *testing.T) {
	pool, ctx := newPool(t)
	rules := postgres.NewRulesRepository(pool)

	r := world.DefaultRules
	r.ColonizeHours = 6
	if err := rules.SetRules(ctx, r); err != nil {
		t.Fatal(err)
	}
	if got, err := rules.Rules(ctx); err != nil || got != r {
		t.Fatalf("Rules = %+v, %v", got, err)
	}
	if err := rules.SetRules(ctx, world.DefaultRules); err != nil {
		t.Fatal(err)
	}
}
