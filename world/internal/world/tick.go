package world

import (
	"slices"
	"time"

	"github.com/google/uuid"
)

// Tick counts the hours from the last one counted to now, a day at most:
// the income of the provinces, then the colonies settled by then.
func (g *Game) Tick(a *Atlas, r Rules, now time.Time) int {
	end := now.UTC().Truncate(time.Hour)
	if late := end.Add(-maxCatchUp * time.Hour); g.hour.Before(late) {
		g.hour = late
	}

	hours := 0
	for g.hour.Before(end) {
		g.hour = g.hour.Add(time.Hour)
		g.earn(a, r, g.hour)
		g.settle(g.hour)
		hours++
	}

	return hours
}

// earn pays every state the income of the hour that ended at hour.
func (g *Game) earn(a *Atlas, r Rules, hour time.Time) {
	for _, st := range g.states {
		income := g.income(a, r, st)
		st.Treasury += income
		st.book(hour.Add(-time.Hour), func(h *BudgetHour) { h.Income += income })
	}
}

// Income is what the provinces of the state bring in an hour, in
// hundredths.
func (g *Game) Income(a *Atlas, r Rules, stateID uuid.UUID) int64 {
	st := g.state(stateID)
	if st == nil {
		return 0
	}

	return g.income(a, r, st)
}

func (g *Game) income(a *Atlas, r Rules, st *State) int64 {
	people := 0
	for p, id := range g.owners {
		if id == st.ID {
			if pr, ok := a.Province(p); ok {
				people += pr.Population
			}
		}
	}

	return int64(people) * r.Income / 10
}

// settle makes the colonies due by hour provinces of their states.
func (g *Game) settle(hour time.Time) {
	var settled []Colony
	g.colonies = slices.DeleteFunc(g.colonies, func(c Colony) bool {
		if c.Until.After(hour) {
			return false
		}
		settled = append(settled, c)
		return true
	})

	for _, c := range settled {
		st := g.state(c.StateID)
		if st == nil {
			continue
		}
		g.owners[c.Province] = st.ID
		g.record(Event{Kind: EventColonized, StateID: st.ID, Province: c.Province, Name: st.Name.String(), At: hour})
	}
}
