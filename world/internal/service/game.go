package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/world/internal/world"
)

// MaxEvents is the most news read at once.
const MaxEvents = 50

var ErrTooManyEvents = errors.New("too many events")

// GameService is what people do in the open world: found states, join
// them, settle the land around; and the hour of the world that pays the
// treasuries.
type GameService struct {
	worlds  *WorldService
	games   GameRepository
	events  EventRepository
	wallets Wallets
	clubs   Clubs
	tx      TxManager
	now     func() time.Time
}

// GameDeps are what GameService works with.
type GameDeps struct {
	Worlds  *WorldService
	Games   GameRepository
	Events  EventRepository
	Wallets Wallets
	Clubs   Clubs
	Tx      TxManager
}

func NewGameService(d GameDeps) *GameService {
	return &GameService{
		worlds: d.Worlds, games: d.Games, events: d.Events, wallets: d.Wallets, clubs: d.Clubs, tx: d.Tx, now: time.Now,
	}
}

// current is the open world with its map and rules.
type current struct {
	world *world.World
	atlas *world.Atlas
	rules world.Rules
}

func (s *GameService) current(ctx context.Context) (current, error) {
	w, err := s.worlds.worlds.OpenWorld(ctx)
	if err != nil {
		return current{}, errs.Wrap("s.worlds.worlds.OpenWorld", err)
	}

	a, err := s.worlds.Atlas(ctx, w.ID())
	if err != nil {
		return current{}, errs.Wrap("s.worlds.Atlas", err)
	}

	r, err := s.worlds.Rules(ctx)
	if err != nil {
		return current{}, errs.Wrap("s.worlds.Rules", err)
	}

	return current{world: w, atlas: a, rules: r}, nil
}

// change runs fn on the game of the open world locked and keeps what it
// did.
func (s *GameService) change(ctx context.Context, c current, fn func(g *world.Game) error) error {
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		g, err := s.games.Game(ctx, c.world.ID(), true)
		if err != nil {
			return errs.Wrap("s.games.Game", err)
		}

		if err = fn(g); err != nil {
			return err
		}

		if err = s.games.SaveGame(ctx, c.world.ID(), g); err != nil {
			return errs.Wrap("s.games.SaveGame", err)
		}

		return errs.Wrap("s.events.AddEvents", s.events.AddEvents(ctx, c.world.ID(), g.TakeEvents()))
	})

	return errs.Wrap("s.tx.WithinTx", err)
}

// act runs fn on the game of the open world.
func (s *GameService) act(ctx context.Context, fn func(c current, g *world.Game, now time.Time) error) error {
	c, err := s.current(ctx)
	if err != nil {
		return errs.Wrap("s.current", err)
	}

	return s.change(ctx, c, func(g *world.Game) error { return fn(c, g, s.now()) })
}

// Viewer is who looks at the world: their citizenship and application.
type Viewer struct {
	Citizen        world.Citizen
	IsCitizen      bool
	Application    world.Application
	HasApplication bool
}

func viewerOf(g *world.Game, viewerID uuid.UUID) Viewer {
	var v Viewer
	v.Citizen, v.IsCitizen = g.Citizen(viewerID)
	v.Application, v.HasApplication = g.Application(viewerID)

	return v
}

// Overview is the open world as the map shows it.
type Overview struct {
	World *world.World
	Rules world.Rules
	// Hour is the last hour counted.
	Hour      time.Time
	Standings []world.Standing
	// Owners are the states holding every province, uuid.Nil for none.
	Owners   []uuid.UUID
	Colonies []world.Colony
	Viewer   Viewer
}

// Overview is the open world as viewerID (uuid.Nil: a guest) sees it;
// world.ErrNoWorld.
func (s *GameService) Overview(ctx context.Context, viewerID uuid.UUID) (Overview, error) {
	c, err := s.current(ctx)
	if err != nil {
		return Overview{}, errs.Wrap("s.current", err)
	}

	g, err := s.games.Game(ctx, c.world.ID(), false)
	if err != nil {
		return Overview{}, errs.Wrap("s.games.Game", err)
	}

	owners := make([]uuid.UUID, len(c.atlas.Provinces))
	for i := range owners {
		owners[i] = g.Owner(i)
	}

	return Overview{
		World: c.world, Rules: c.rules, Hour: g.Hour(), Standings: g.Standings(c.atlas, c.rules),
		Owners: owners, Colonies: g.Colonies(), Viewer: viewerOf(g, viewerID),
	}, nil
}

// StateView is a state with its people and land.
type StateView struct {
	Standing world.Standing
	Lands    []int
	Citizens []world.Citizen
	Colonies []world.Colony
	// Applications are shown to those who answer them only.
	Applications []world.Application
	// ColonyCost is what the next colony costs, in hundredths.
	ColonyCost int64
	Viewer     Viewer
}

// State is the state id of the open world as viewerID sees it;
// world.ErrStateNotFound.
func (s *GameService) State(ctx context.Context, viewerID, id uuid.UUID) (StateView, error) {
	c, err := s.current(ctx)
	if err != nil {
		return StateView{}, errs.Wrap("s.current", err)
	}

	g, err := s.games.Game(ctx, c.world.ID(), false)
	if err != nil {
		return StateView{}, errs.Wrap("s.games.Game", err)
	}

	var standing world.Standing
	found := false
	for _, st := range g.Standings(c.atlas, c.rules) {
		if st.State.ID == id {
			standing, found = st, true
		}
	}
	if !found {
		return StateView{}, world.ErrStateNotFound
	}

	v := StateView{
		Standing: standing, Lands: g.Lands(id), Citizens: g.Citizens(id), Colonies: g.ColoniesOf(id),
		ColonyCost: g.ColonyCost(c.rules, id), Viewer: viewerOf(g, viewerID),
	}
	if g.MayAnswer(viewerID, id) {
		v.Applications = g.Applications(id)
	}

	return v, nil
}

// Citizenship is the state userID is a citizen of in the open world;
// false for none, or when no world is open.
func (s *GameService) Citizenship(ctx context.Context, userID uuid.UUID) (world.Citizen, world.State, bool, error) {
	c, err := s.current(ctx)
	if errors.Is(err, world.ErrNoWorld) {
		return world.Citizen{}, world.State{}, false, nil
	}
	if err != nil {
		return world.Citizen{}, world.State{}, false, errs.Wrap("s.current", err)
	}

	g, err := s.games.Game(ctx, c.world.ID(), false)
	if err != nil {
		return world.Citizen{}, world.State{}, false, errs.Wrap("s.games.Game", err)
	}

	citizen, ok := g.Citizen(userID)
	if !ok {
		return world.Citizen{}, world.State{}, false, nil
	}

	st, ok := g.State(citizen.StateID)

	return citizen, st, ok, nil
}

// Events are the news of the open world, of the state unless stateID is
// uuid.Nil, older than before, the newest first; at most MaxEvents
// (ErrTooManyEvents).
func (s *GameService) Events(ctx context.Context, stateID, before uuid.UUID, limit int) ([]world.Event, error) {
	if limit > MaxEvents {
		return nil, ErrTooManyEvents
	}

	w, err := s.worlds.worlds.OpenWorld(ctx)
	if err != nil {
		return nil, errs.Wrap("s.worlds.worlds.OpenWorld", err)
	}

	events, err := s.events.Events(ctx, w.ID(), stateID, before, max(limit, 1))

	return events, errs.Wrap("s.events.Events", err)
}
