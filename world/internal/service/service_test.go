package service_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
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

// fakeWallets keeps the votes of pages; refused is what Charge answers.
type fakeWallets struct {
	mu      sync.Mutex
	votes   map[uuid.UUID]int64
	refused error
}

func (w *fakeWallets) Charge(_ context.Context, op service.VoteOp) (int64, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.refused != nil {
		return 0, w.refused
	}
	if w.votes[op.UserID] < op.Votes {
		return 0, service.ErrNotEnoughVotes
	}
	w.votes[op.UserID] -= op.Votes

	return w.votes[op.UserID], nil
}

func (w *fakeWallets) Pay(_ context.Context, op service.VoteOp) (int64, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.votes[op.UserID] += op.Votes

	return w.votes[op.UserID], nil
}

func (w *fakeWallets) of(id uuid.UUID) int64 {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.votes[id]
}

type fakeClubs struct {
	mu   sync.Mutex
	made []uuid.UUID
	err  error
}

func (c *fakeClubs) CreateStateClub(_ context.Context, stateID, _ uuid.UUID, _, _ string) (uuid.UUID, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.err != nil {
		return uuid.Nil, c.err
	}
	c.made = append(c.made, stateID)

	return stateID, nil
}

type env struct {
	worlds  *service.WorldService
	games   *service.GameService
	wallets *fakeWallets
	clubs   *fakeClubs
	store   *inmem.Store
}

func newEnv(t *testing.T, maker service.MapMaker) env {
	t.Helper()

	store := inmem.NewStore()
	worlds := service.NewWorldService(service.WorldDeps{
		Worlds: store, Maps: store, Games: store, Rules: store, Maker: maker, Tx: inmem.TxManager{},
	})
	wallets := &fakeWallets{votes: map[uuid.UUID]int64{}}
	clubs := &fakeClubs{}
	games := service.NewGameService(service.GameDeps{
		Worlds: worlds, Games: store, Events: store, Wallets: wallets, Clubs: clubs, Tx: inmem.TxManager{},
	})

	return env{worlds: worlds, games: games, wallets: wallets, clubs: clubs, store: store}
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

func founding(t *testing.T, ruler uuid.UUID, name string, capital int) world.Founding {
	t.Helper()

	n, err := world.NewStateName(name)
	if err != nil {
		t.Fatal(err)
	}
	flag, err := world.NewFlag(world.FlagParts{Pattern: world.PatternPlain, Colors: []int{3}})
	if err != nil {
		t.Fatal(err)
	}

	return world.Founding{RulerID: ruler, Name: n, Flag: flag, Government: world.GovernmentRepublic, Capital: capital}
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
	if _, err := e.games.Overview(ctx, uuid.Nil); !errors.Is(err, world.ErrNoWorld) {
		t.Fatalf("Overview without a world: %v", err)
	}

	first := e.open(t)
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

	bad := world.DefaultRules
	bad.ColonizeHours = 0
	if err = e.worlds.SetRules(ctx, bad); !errors.Is(err, world.ErrRules) {
		t.Fatalf("SetRules: %v", err)
	}
	good := world.DefaultRules
	good.FoundCost = 10
	if err = e.worlds.SetRules(ctx, good); err != nil {
		t.Fatal(err)
	}
	if r, err := e.worlds.Rules(ctx); err != nil || r.FoundCost != 10 {
		t.Fatalf("Rules = %v, %v", r, err)
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

func TestFoundPays(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	e := newEnv(t, fakeMaker{})
	e.open(t)
	ann, bob := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())

	if _, err := e.games.Found(ctx, founding(t, ann, "Бореалия", 1)); !errors.Is(err, service.ErrNotEnoughVotes) {
		t.Fatalf("Found without votes: %v", err)
	}

	e.wallets.votes[ann] = 6000
	id, err := e.games.Found(ctx, founding(t, ann, "Бореалия", 1))
	if err != nil || e.wallets.of(ann) != 1000 {
		t.Fatalf("Found = %v, %v; ann has %d", id, err, e.wallets.of(ann))
	}

	// Refused before the votes are paid.
	e.wallets.votes[bob] = 6000
	if _, err = e.games.Found(ctx, founding(t, bob, "бореалия", 8)); !errors.Is(err, world.ErrNameTaken) || e.wallets.of(bob) != 6000 {
		t.Fatalf("Found of a taken name: %v; bob has %d", err, e.wallets.of(bob))
	}

	e.wallets.refused = service.ErrNotSettled
	if _, err = e.games.Found(ctx, founding(t, bob, "Нордия", 8)); !errors.Is(err, service.ErrNotSettled) {
		t.Fatalf("Found by a new page: %v", err)
	}
	e.wallets.refused = nil

	o, err := e.games.Overview(ctx, ann)
	if err != nil || len(o.Standings) != 1 || o.Standings[0].Lands != 3 || o.Owners[0] != id || !o.Viewer.IsCitizen ||
		o.Viewer.Citizen.Role != world.RoleRuler {
		t.Fatalf("Overview = %+v, %v", o, err)
	}

	events, err := e.games.Events(ctx, uuid.Nil, uuid.Nil, 10)
	if err != nil || len(events) != 1 || events[0].Kind != world.EventFounded {
		t.Fatalf("Events = %v, %v", events, err)
	}
	if _, err = e.games.Events(ctx, uuid.Nil, uuid.Nil, service.MaxEvents+1); !errors.Is(err, service.ErrTooManyEvents) {
		t.Fatalf("Events of too many: %v", err)
	}
}

// racingGames found the same name between the check of Found and its
// change: the votes come back.
type racingGames struct {
	*inmem.Store
	once  sync.Once
	other world.Founding
}

func (g *racingGames) Game(ctx context.Context, worldID uuid.UUID, lock bool) (*world.Game, error) {
	game, err := g.Store.Game(ctx, worldID, lock)
	if err != nil || !lock {
		return game, err
	}

	g.once.Do(func() {
		g.other.ID = uuid.Must(uuid.NewV7())
		err = game.Found(line(), world.DefaultRules, g.other, time.Now())
	})

	return game, err
}

func TestFoundRefundsWhenRefused(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := inmem.NewStore()
	worlds := service.NewWorldService(service.WorldDeps{Worlds: store, Maps: store, Games: store, Rules: store, Maker: fakeMaker{}, Tx: inmem.TxManager{}})
	wallets := &fakeWallets{votes: map[uuid.UUID]int64{}}
	race := &racingGames{Store: store}
	games := service.NewGameService(service.GameDeps{Worlds: worlds, Games: race, Events: store, Wallets: wallets, Clubs: &fakeClubs{}, Tx: inmem.TxManager{}})
	(env{worlds: worlds, store: store}).open(t)

	ann := uuid.Must(uuid.NewV7())
	race.other = founding(t, uuid.Must(uuid.NewV7()), "Бореалия", 8)
	wallets.votes[ann] = 5000
	if _, err := games.Found(ctx, founding(t, ann, "Бореалия", 1)); !errors.Is(err, world.ErrNameTaken) {
		t.Fatalf("Found = %v", err)
	}
	if got := wallets.of(ann); got != 5000 {
		t.Fatalf("ann has %d after the refund", got)
	}
}

func TestStateAndCitizens(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	e := newEnv(t, fakeMaker{})
	e.open(t)
	ann, bob := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	e.wallets.votes[ann] = 5000
	e.wallets.votes[bob] = 30

	id, err := e.games.Found(ctx, founding(t, ann, "Бореалия", 1))
	if err != nil {
		t.Fatal(err)
	}

	if err = e.games.Apply(ctx, bob, id); err != nil {
		t.Fatal(err)
	}
	if v, err := e.games.State(ctx, bob, id); err != nil || len(v.Applications) != 0 || !v.Viewer.HasApplication {
		t.Fatalf("State by the applicant = %+v, %v", v, err)
	}
	v, err := e.games.State(ctx, ann, id)
	if err != nil || len(v.Applications) != 1 || len(v.Lands) != 3 || v.ColonyCost != 26000 {
		t.Fatalf("State by the ruler = %+v, %v", v, err)
	}
	if _, err = e.games.State(ctx, ann, uuid.Must(uuid.NewV7())); !errors.Is(err, world.ErrStateNotFound) {
		t.Fatalf("State of nothing: %v", err)
	}

	if err = e.games.Answer(ctx, ann, bob, true); err != nil {
		t.Fatal(err)
	}
	if err = e.games.Donate(ctx, bob, 50); !errors.Is(err, service.ErrNotEnoughVotes) {
		t.Fatalf("Donate of too many: %v", err)
	}
	if err = e.games.Donate(ctx, bob, 20); err != nil || e.wallets.of(bob) != 10 {
		t.Fatalf("Donate = %v; bob has %d", err, e.wallets.of(bob))
	}
	if err = e.games.Donate(ctx, uuid.Must(uuid.NewV7()), 20); !errors.Is(err, world.ErrNotCitizen) {
		t.Fatalf("Donate by a stranger: %v", err)
	}

	if err = e.games.Appoint(ctx, ann, bob, world.RoleMinisterEconomy); err != nil {
		t.Fatal(err)
	}
	if err = e.games.Colonize(ctx, bob, 3); err != nil {
		t.Fatal(err)
	}
	if err = e.games.Edit(ctx, bob, world.Edit{}); !errors.Is(err, world.ErrNotAllowed) {
		t.Fatalf("Edit by a minister: %v", err)
	}

	citizen, st, ok, err := e.games.Citizenship(ctx, bob)
	if err != nil || !ok || st.ID != id || citizen.Role != world.RoleMinisterEconomy || st.Treasury != 100000+2000-26000 {
		t.Fatalf("Citizenship = %+v, %+v, %v, %v", citizen, st, ok, err)
	}

	if err = e.games.Abdicate(ctx, ann, bob); err != nil {
		t.Fatal(err)
	}
	if err = e.games.Leave(ctx, ann); err != nil {
		t.Fatal(err)
	}
	if err = e.games.Apply(ctx, ann, id); err != nil {
		t.Fatal(err)
	}
	if err = e.games.Withdraw(ctx, ann); err != nil {
		t.Fatal(err)
	}

	// Bob's page is gone: nobody is left, the state dissolves.
	if err = e.games.PurgeUser(ctx, bob); err != nil {
		t.Fatal(err)
	}
	if _, _, ok, err = e.games.Citizenship(ctx, bob); err != nil || ok {
		t.Fatalf("Citizenship after the purge = %v, %v", ok, err)
	}
	if err = e.games.Expel(ctx, bob, ann); !errors.Is(err, world.ErrNotCitizen) {
		t.Fatalf("Expel by the purged: %v", err)
	}
}

func TestTickAndClubs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	e := newEnv(t, fakeMaker{})

	if hours, err := e.games.Tick(ctx); err != nil || hours != 0 {
		t.Fatalf("Tick without a world = %d, %v", hours, err)
	}
	if made, err := e.games.MakeClubs(ctx); err != nil || made != 0 {
		t.Fatalf("MakeClubs without a world = %d, %v", made, err)
	}
	if err := e.games.PurgeUser(ctx, uuid.Must(uuid.NewV7())); err != nil {
		t.Fatalf("PurgeUser without a world: %v", err)
	}
	if _, _, ok, err := e.games.Citizenship(ctx, uuid.Must(uuid.NewV7())); err != nil || ok {
		t.Fatalf("Citizenship without a world = %v, %v", ok, err)
	}

	e.open(t)
	ann := uuid.Must(uuid.NewV7())
	e.wallets.votes[ann] = 5000
	id, err := e.games.Found(ctx, founding(t, ann, "Бореалия", 1))
	if err != nil {
		t.Fatal(err)
	}

	e.clubs.err = errors.New("club is down")
	if _, err = e.games.MakeClubs(ctx); err == nil {
		t.Fatal("MakeClubs with club down succeeded")
	}
	e.clubs.err = nil
	if made, err := e.games.MakeClubs(ctx); err != nil || made != 1 {
		t.Fatalf("MakeClubs = %d, %v", made, err)
	}
	if made, err := e.games.MakeClubs(ctx); err != nil || made != 0 {
		t.Fatalf("MakeClubs again = %d, %v", made, err)
	}
	if v, err := e.games.State(ctx, uuid.Nil, id); err != nil || v.Standing.State.ClubID != id {
		t.Fatalf("State = %+v, %v", v.Standing.State, err)
	}

	if _, err = e.games.Tick(ctx); err != nil {
		t.Fatal(err)
	}
}
