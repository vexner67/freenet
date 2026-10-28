package world

import (
	"slices"
	"time"

	"github.com/google/uuid"
)

// Colonize starts a colony of the state of actorID in province: free land
// next to the state, paid from the treasury, settled in
// Rules.ColonizeHours. The ruler and the minister of the economy do it,
// with one colonist at a time and one more for every ten provinces.
func (g *Game) Colonize(a *Atlas, r Rules, actorID uuid.UUID, province int, now time.Time) error {
	c, st, err := g.office(actorID)
	if err != nil {
		return err
	}

	if !c.Role.mayColonize() {
		return ErrNotAllowed
	}

	if err = g.settleable(a, province); err != nil {
		return err
	}

	if !slices.ContainsFunc(a.landNeighbours(province), func(n int) bool { return g.owners[n] == st.ID }) {
		return ErrNotNeighbour
	}

	lands := len(g.Lands(st.ID))
	if len(g.ColoniesOf(st.ID)) >= colonists(lands) {
		return ErrNoColonists
	}

	cost := r.colonizeCost(lands)
	if st.Treasury < cost {
		return ErrTreasury
	}

	st.Treasury -= cost
	st.book(now, func(h *BudgetHour) { h.Colonization += cost })
	g.colonies = append(g.colonies, Colony{
		Province: province, StateID: st.ID, Until: trim(now).Add(time.Duration(r.ColonizeHours) * time.Hour),
	})

	return nil
}

// ColoniesOf are the colonies of the state under way.
func (g *Game) ColoniesOf(stateID uuid.UUID) []Colony {
	var out []Colony
	for _, c := range g.colonies {
		if c.StateID == stateID {
			out = append(out, c)
		}
	}

	return out
}

// ColonyCost is what the next colony of the state costs, in hundredths.
func (g *Game) ColonyCost(r Rules, stateID uuid.UUID) int64 {
	return r.colonizeCost(len(g.Lands(stateID)))
}
