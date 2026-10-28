package world

import (
	"slices"
	"time"

	"github.com/google/uuid"
)

// maxCatchUp is how many hours a tick counts at most: a world stopped for
// longer loses the rest, as the business does.
const maxCatchUp = 24

// Game is what people made of a world: the states, who holds which
// province, the citizens, their applications and the colonies under way.
// Every change takes the whole of it, as one aggregate.
type Game struct {
	hour         time.Time
	states       []*State
	owners       map[int]uuid.UUID
	citizens     map[uuid.UUID]Citizen
	applications []Application
	colonies     []Colony
	// events are what happened since the game was read, to be kept.
	events []Event
}

// NewGame is the game of a world opened at now: nobody holds anything.
func NewGame(now time.Time) *Game {
	return &Game{hour: now.UTC().Truncate(time.Hour), owners: map[int]uuid.UUID{}, citizens: map[uuid.UUID]Citizen{}}
}

// GameState is a game as storage keeps it.
type GameState struct {
	Hour         time.Time
	States       []State
	Owners       map[int]uuid.UUID
	Citizens     []Citizen
	Applications []Application
	Colonies     []Colony
}

func RestoreGame(s GameState) *Game {
	g := &Game{
		hour: s.Hour.UTC(), owners: map[int]uuid.UUID{}, citizens: map[uuid.UUID]Citizen{},
		applications: slices.Clone(s.Applications), colonies: slices.Clone(s.Colonies),
	}
	for _, st := range s.States {
		st.Budget = slices.Clone(st.Budget)
		g.states = append(g.states, &st)
	}
	for p, id := range s.Owners {
		g.owners[p] = id
	}
	for _, c := range s.Citizens {
		g.citizens[c.UserID] = c
	}

	return g
}

// Snapshot is the game as it is now, to keep or to show; the citizens in
// the order they joined.
func (g *Game) Snapshot() GameState {
	s := GameState{
		Hour: g.hour, Owners: make(map[int]uuid.UUID, len(g.owners)),
		Applications: slices.Clone(g.applications), Colonies: slices.Clone(g.colonies),
	}
	for _, st := range g.states {
		c := *st
		c.Budget = slices.Clone(st.Budget)
		s.States = append(s.States, c)
	}
	for p, id := range g.owners {
		s.Owners[p] = id
	}
	s.Citizens = g.sortedCitizens()

	return s
}

// sortedCitizens are the citizens in the order they joined.
func (g *Game) sortedCitizens() []Citizen {
	out := make([]Citizen, 0, len(g.citizens))
	for _, c := range g.citizens {
		out = append(out, c)
	}
	slices.SortFunc(out, func(a, b Citizen) int {
		if c := a.JoinedAt.Compare(b.JoinedAt); c != 0 {
			return c
		}
		return slices.Compare(a.UserID[:], b.UserID[:])
	})

	return out
}

// Hour is the last hour counted.
func (g *Game) Hour() time.Time { return g.hour }

// TakeEvents are what happened since the last call.
func (g *Game) TakeEvents() []Event {
	e := g.events
	g.events = nil

	return e
}

// State is the state id.
func (g *Game) State(id uuid.UUID) (State, bool) {
	st := g.state(id)
	if st == nil {
		return State{}, false
	}

	c := *st
	c.Budget = slices.Clone(st.Budget)

	return c, true
}

func (g *Game) state(id uuid.UUID) *State {
	for _, st := range g.states {
		if st.ID == id {
			return st
		}
	}

	return nil
}

// Citizen is the citizenship of userID; false for none.
func (g *Game) Citizen(userID uuid.UUID) (Citizen, bool) {
	c, ok := g.citizens[userID]
	return c, ok
}

// Application is the application of userID; false for none.
func (g *Game) Application(userID uuid.UUID) (Application, bool) {
	for _, a := range g.applications {
		if a.UserID == userID {
			return a, true
		}
	}

	return Application{}, false
}

// Lands are the provinces of the state, in order.
func (g *Game) Lands(stateID uuid.UUID) []int {
	var out []int
	for p, id := range g.owners {
		if id == stateID {
			out = append(out, p)
		}
	}
	slices.Sort(out)

	return out
}

func (g *Game) record(e Event) {
	e.ID = uuid.Must(uuid.NewV7())
	g.events = append(g.events, e)
}

// office is the role of userID in their state, and the state.
func (g *Game) office(userID uuid.UUID) (Citizen, *State, error) {
	c, ok := g.citizens[userID]
	if !ok {
		return Citizen{}, nil, ErrNotCitizen
	}

	st := g.state(c.StateID)
	if st == nil {
		return Citizen{}, nil, ErrNotCitizen
	}

	return c, st, nil
}

// isFree says whether nobody holds or settles the province.
func (g *Game) isFree(province int) bool {
	if _, held := g.owners[province]; held {
		return false
	}

	return !slices.ContainsFunc(g.colonies, func(c Colony) bool { return c.Province == province })
}

// settleable checks that province is land nobody holds, settles, nor
// keeps as a free city.
func (g *Game) settleable(a *Atlas, province int) error {
	p, ok := a.Province(province)
	switch {
	case !ok || p.Sea:
		return ErrNotProvince
	case p.FreeCity:
		return ErrFreeCity
	case !g.isFree(province):
		return ErrProvinceTaken
	}

	return nil
}

// Standing is a state with what it has: for the map and the ratings.
type Standing struct {
	State      State
	Lands      int
	Population int
	Citizens   int
	// Income is per hour, in hundredths.
	Income int64
}

// Standings are all the states, the oldest first.
func (g *Game) Standings(a *Atlas, r Rules) []Standing {
	out := make([]Standing, 0, len(g.states))
	index := make(map[uuid.UUID]int, len(g.states))
	for i, st := range g.states {
		s, _ := g.State(st.ID)
		out = append(out, Standing{State: s, Income: g.income(a, r, st)})
		index[st.ID] = i
	}

	for p, id := range g.owners {
		out[index[id]].Lands++
		if pr, ok := a.Province(p); ok {
			out[index[id]].Population += pr.Population
		}
	}
	for _, c := range g.citizens {
		if i, ok := index[c.StateID]; ok {
			out[i].Citizens++
		}
	}

	return out
}

// Owner is the state holding the province, uuid.Nil for none.
func (g *Game) Owner(province int) uuid.UUID {
	return g.owners[province]
}

// Colonies are all the colonies under way.
func (g *Game) Colonies() []Colony {
	return slices.Clone(g.colonies)
}
