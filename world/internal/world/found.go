package world

import (
	"time"

	"github.com/google/uuid"
)

// Founding is what a state is founded with.
type Founding struct {
	ID         uuid.UUID
	RulerID    uuid.UUID
	Name       StateName
	Adjective  Adjective
	Motto      Motto
	Color      Color
	Flag       Flag
	Government Government
	Capital    int
}

// CheckFound tells whether f may be founded now, before its votes are
// paid: Found checks the same again.
func (g *Game) CheckFound(a *Atlas, r Rules, f Founding) error {
	if _, ok := g.citizens[f.RulerID]; ok {
		return ErrCitizen
	}

	for _, st := range g.states {
		if st.Name.same(f.Name) {
			return ErrNameTaken
		}
	}

	if err := g.settleable(a, f.Capital); err != nil {
		return err
	}

	near := a.distances(f.Capital, r.CapitalDistance-1)
	for _, st := range g.states {
		if _, ok := near[st.Capital]; ok {
			return ErrTooClose
		}
	}

	return nil
}

// Found makes the state f: its capital and up to Rules.FoundLands free
// neighbours, the treasury of Rules.FoundTreasury and the protection of
// Rules.ProtectionDays. The founder rules it, and their application, if
// any, is withdrawn.
func (g *Game) Found(a *Atlas, r Rules, f Founding, now time.Time) error {
	if err := g.CheckFound(a, r, f); err != nil {
		return err
	}

	now = trim(now)
	st := &State{
		ID: f.ID, Name: f.Name, Adjective: f.Adjective, Motto: f.Motto, Color: f.Color, Flag: f.Flag,
		Government: f.Government, RulerID: f.RulerID, Capital: f.Capital, Treasury: r.FoundTreasury * 100,
		FoundedAt: now, ProtectedUntil: now.AddDate(0, 0, r.ProtectionDays),
	}
	st.book(now, func(h *BudgetHour) { h.Founding += st.Treasury })
	g.states = append(g.states, st)

	g.owners[f.Capital] = st.ID
	extra := 0
	for _, n := range a.landNeighbours(f.Capital) {
		if extra == r.FoundLands {
			break
		}
		if g.settleable(a, n) == nil {
			g.owners[n] = st.ID
			extra++
		}
	}

	g.withdraw(f.RulerID)
	g.citizens[f.RulerID] = Citizen{UserID: f.RulerID, StateID: st.ID, Role: RoleRuler, JoinedAt: now}
	g.record(Event{Kind: EventFounded, StateID: st.ID, UserID: f.RulerID, Province: f.Capital, Name: st.Name.String(), At: now})

	return nil
}

// Edit is what the ruler may change of a state.
type Edit struct {
	Adjective Adjective
	Motto     Motto
	Color     Color
	Flag      Flag
}

// EditState changes the state of its ruler userID.
func (g *Game) EditState(userID uuid.UUID, e Edit) error {
	c, st, err := g.office(userID)
	if err != nil {
		return err
	}

	if c.Role != RoleRuler {
		return ErrNotAllowed
	}

	st.Adjective, st.Motto, st.Color, st.Flag = e.Adjective, e.Motto, e.Color, e.Flag

	return nil
}

// SetClub remembers the group of the state.
func (g *Game) SetClub(stateID, clubID uuid.UUID) {
	if st := g.state(stateID); st != nil {
		st.ClubID = clubID
	}
}

// WithoutClub are the states whose group is still to be made.
func (g *Game) WithoutClub() []State {
	var out []State
	for _, st := range g.states {
		if st.ClubID == uuid.Nil {
			out = append(out, *st)
		}
	}

	return out
}
