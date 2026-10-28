package world

import (
	"slices"
	"time"

	"github.com/google/uuid"
)

// Apply asks the state to take userID in, instead of the application they
// had.
func (g *Game) Apply(userID, stateID uuid.UUID, now time.Time) error {
	if _, ok := g.citizens[userID]; ok {
		return ErrCitizen
	}

	if g.state(stateID) == nil {
		return ErrStateNotFound
	}

	if a, ok := g.Application(userID); ok && a.StateID == stateID {
		return ErrApplied
	}

	g.withdraw(userID)
	g.applications = append(g.applications, Application{UserID: userID, StateID: stateID, At: trim(now)})

	return nil
}

// Withdraw takes back the application of userID; ErrNoApplication.
func (g *Game) Withdraw(userID uuid.UUID) error {
	if _, ok := g.Application(userID); !ok {
		return ErrNoApplication
	}

	g.withdraw(userID)

	return nil
}

func (g *Game) withdraw(userID uuid.UUID) {
	g.applications = slices.DeleteFunc(g.applications, func(a Application) bool { return a.UserID == userID })
}

// Applications are those to the state, the oldest first.
func (g *Game) Applications(stateID uuid.UUID) []Application {
	var out []Application
	for _, a := range g.applications {
		if a.StateID == stateID {
			out = append(out, a)
		}
	}

	return out
}

// Answer takes the applicant in or turns them down; the ruler and the
// minister of the interior do it.
func (g *Game) Answer(actorID, applicantID uuid.UUID, accept bool, now time.Time) error {
	c, st, err := g.office(actorID)
	if err != nil {
		return err
	}

	if !c.Role.mayAnswer() {
		return ErrNotAllowed
	}

	a, ok := g.Application(applicantID)
	if !ok || a.StateID != st.ID {
		return ErrNoApplication
	}

	g.withdraw(applicantID)
	if !accept {
		return nil
	}

	now = trim(now)
	g.citizens[applicantID] = Citizen{UserID: applicantID, StateID: st.ID, Role: RoleCitizen, JoinedAt: now}
	g.record(Event{Kind: EventJoined, StateID: st.ID, UserID: applicantID, Name: st.Name.String(), At: now})

	return nil
}

// Leave: userID is a citizen no more. The ruler first gives the state to
// another (Abdicate).
func (g *Game) Leave(userID uuid.UUID, now time.Time) error {
	c, st, err := g.office(userID)
	if err != nil {
		return err
	}

	if c.Role == RoleRuler {
		return ErrRulerLeaves
	}

	delete(g.citizens, userID)
	g.record(Event{Kind: EventLeft, StateID: st.ID, UserID: userID, Name: st.Name.String(), At: trim(now)})

	return nil
}

// Expel sends a citizen away: the ruler anyone, the minister of the
// interior those without an office of a minister.
func (g *Game) Expel(actorID, targetID uuid.UUID, now time.Time) error {
	c, st, err := g.office(actorID)
	if err != nil {
		return err
	}

	t, err := g.fellow(st, targetID)
	if err != nil {
		return err
	}

	switch {
	case actorID == targetID:
		return ErrSelf
	case !c.Role.mayAnswer(), t.Role == RoleRuler, c.Role != RoleRuler && t.Role.isMinister():
		return ErrNotAllowed
	}

	delete(g.citizens, targetID)
	g.record(Event{Kind: EventExpelled, StateID: st.ID, UserID: targetID, Name: st.Name.String(), At: trim(now)})

	return nil
}

// Appoint gives a citizen an office; only the ruler does.
func (g *Game) Appoint(actorID, targetID uuid.UUID, role Role, now time.Time) error {
	c, st, err := g.office(actorID)
	if err != nil {
		return err
	}

	t, err := g.fellow(st, targetID)
	if err != nil {
		return err
	}

	switch {
	case c.Role != RoleRuler:
		return ErrNotAllowed
	case actorID == targetID:
		return ErrSelf
	case role == RoleRuler:
		return ErrRole
	}

	t.Role = role
	g.citizens[targetID] = t
	g.record(Event{Kind: EventAppointed, StateID: st.ID, UserID: targetID, Name: st.Name.String(), Role: role, At: trim(now)})

	return nil
}

// Abdicate gives the state of its ruler to the citizen heirID; the ruler
// stays a citizen.
func (g *Game) Abdicate(actorID, heirID uuid.UUID, now time.Time) error {
	c, st, err := g.office(actorID)
	if err != nil {
		return err
	}

	heir, err := g.fellow(st, heirID)
	switch {
	case err != nil:
		return err
	case c.Role != RoleRuler:
		return ErrNotAllowed
	case actorID == heirID:
		return ErrSelf
	}

	c.Role = RoleCitizen
	g.citizens[actorID] = c
	g.crown(st, heir, now)

	return nil
}

// crown makes heir the ruler of st.
func (g *Game) crown(st *State, heir Citizen, now time.Time) {
	heir.Role = RoleRuler
	g.citizens[heir.UserID] = heir
	st.RulerID = heir.UserID
	g.record(Event{Kind: EventSucceeded, StateID: st.ID, UserID: heir.UserID, Name: st.Name.String(), At: trim(now)})
}

// fellow is the citizen targetID of st; ErrNotCitizen for another.
func (g *Game) fellow(st *State, targetID uuid.UUID) (Citizen, error) {
	t, ok := g.citizens[targetID]
	if !ok || t.StateID != st.ID {
		return Citizen{}, ErrNotCitizen
	}

	return t, nil
}

// Citizens are those of the state, the oldest first.
func (g *Game) Citizens(stateID uuid.UUID) []Citizen {
	var out []Citizen
	for _, c := range g.sortedCitizens() {
		if c.StateID == stateID {
			out = append(out, c)
		}
	}

	return out
}

// Donate puts votes of userID, a citizen, into the treasury of their
// state; the votes are paid already.
func (g *Game) Donate(userID uuid.UUID, votes int64, now time.Time) error {
	if votes <= 0 {
		return ErrDonation
	}

	_, st, err := g.office(userID)
	if err != nil {
		return err
	}

	st.Treasury += votes * 100
	st.book(now, func(h *BudgetHour) { h.Donations += votes * 100 })

	return nil
}

// CheckDonate tells whether userID may donate, before the votes are paid.
func (g *Game) CheckDonate(userID uuid.UUID, votes int64) error {
	if votes <= 0 {
		return ErrDonation
	}

	_, _, err := g.office(userID)

	return err
}

// Purge forgets userID, whose page is gone: the application, the
// citizenship. A state they ruled goes to the oldest minister, else the
// oldest general, else the oldest citizen; without anyone it dissolves
// and its provinces are free.
func (g *Game) Purge(userID uuid.UUID, now time.Time) {
	g.withdraw(userID)

	c, st, err := g.office(userID)
	if err != nil {
		return
	}

	delete(g.citizens, userID)
	if c.Role != RoleRuler {
		return
	}

	if heir, ok := g.heir(st.ID); ok {
		g.crown(st, heir, now)
		return
	}

	g.dissolve(st, now)
}

// heir is who rules after the ruler of the state.
func (g *Game) heir(stateID uuid.UUID) (Citizen, bool) {
	rank := func(r Role) int {
		switch {
		case r.isMinister():
			return 0
		case r == RoleGeneral:
			return 1
		}
		return 2
	}

	var best Citizen
	found := false
	for _, c := range g.Citizens(stateID) {
		if !found || rank(c.Role) < rank(best.Role) {
			best, found = c, true
		}
	}

	return best, found
}

func (g *Game) dissolve(st *State, now time.Time) {
	for p, id := range g.owners {
		if id == st.ID {
			delete(g.owners, p)
		}
	}
	g.colonies = slices.DeleteFunc(g.colonies, func(c Colony) bool { return c.StateID == st.ID })
	g.applications = slices.DeleteFunc(g.applications, func(a Application) bool { return a.StateID == st.ID })
	g.states = slices.DeleteFunc(g.states, func(s *State) bool { return s.ID == st.ID })
	g.record(Event{Kind: EventDissolved, StateID: st.ID, Province: st.Capital, Name: st.Name.String(), At: trim(now)})
}

// MayAnswer says whether userID takes people into the state.
func (g *Game) MayAnswer(userID, stateID uuid.UUID) bool {
	c, ok := g.citizens[userID]
	return ok && c.StateID == stateID && c.Role.mayAnswer()
}
