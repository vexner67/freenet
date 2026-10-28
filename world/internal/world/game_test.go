package world_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/world/internal/world"
)

func TestFound(t *testing.T) {
	t.Parallel()

	a := line()
	g := world.NewGame(t0)
	f := found(t, g, a, "Бореалия", 3)

	st, ok := g.State(f.ID)
	if !ok || st.RulerID != f.RulerID || st.Treasury != world.DefaultRules.FoundTreasury*100 ||
		!st.ProtectedUntil.Equal(t0.AddDate(0, 0, 7)) || st.Budget[0].Founding != st.Treasury {
		t.Fatalf("State = %+v", st)
	}
	// The capital and the first two free neighbours: 2 and 4.
	if lands := g.Lands(f.ID); len(lands) != 3 || lands[0] != 2 || lands[1] != 3 || lands[2] != 4 {
		t.Fatalf("Lands = %v", lands)
	}
	if c, ok := g.Citizen(f.RulerID); !ok || c.Role != world.RoleRuler {
		t.Fatalf("the founder is %+v", c)
	}
	if e := g.TakeEvents(); len(e) != 1 || e[0].Kind != world.EventFounded || e[0].Name != "Бореалия" {
		t.Fatalf("events = %+v", e)
	}

	tests := []struct {
		name string
		edit func(*world.Founding)
		want error
	}{
		{"the founder rules already", func(n *world.Founding) { n.RulerID = f.RulerID; n.Capital = 8 }, world.ErrCitizen},
		{"the name is taken", func(n *world.Founding) { n.Name = "бОРЕАЛИЯ"; n.Capital = 8 }, world.ErrNameTaken},
		{"the province is held", func(n *world.Founding) { n.Capital = 4 }, world.ErrProvinceTaken},
		// 6 is 3 steps from the capital 3: near enough; 5 is 2.
		{"too close to a capital", func(n *world.Founding) { n.Capital = 5 }, world.ErrTooClose},
		{"at sea", func(n *world.Founding) { n.Capital = 10 }, world.ErrNotProvince},
		{"off the map", func(n *world.Founding) { n.Capital = 99 }, world.ErrNotProvince},
		{"a free city", func(n *world.Founding) { n.Capital = 11 }, world.ErrFreeCity},
	}
	for _, tt := range tests {
		n := founding(t, "Нордия", 0)
		tt.edit(&n)
		if err := g.Found(a, world.DefaultRules, n, t0); !errors.Is(err, tt.want) {
			t.Errorf("%s: Found = %v, want %v", tt.name, err, tt.want)
		}
	}

	far := world.DefaultRules
	far.CapitalDistance = 4
	if err := g.CheckFound(a, far, founding(t, "Нордия", 6)); !errors.Is(err, world.ErrTooClose) {
		t.Errorf("3 steps from a capital with 4 asked: %v", err)
	}
	if err := g.CheckFound(a, world.DefaultRules, founding(t, "Нордия", 6)); err != nil {
		t.Errorf("3 steps from a capital: %v", err)
	}
}

func TestFoundWithdrawsTheApplication(t *testing.T) {
	t.Parallel()

	a := line()
	g := world.NewGame(t0)
	first := found(t, g, a, "Бореалия", 0)
	f := founding(t, "Нордия", 7)
	if err := g.Apply(f.RulerID, first.ID, t0); err != nil {
		t.Fatal(err)
	}
	if err := g.Found(a, world.DefaultRules, f, t0); err != nil {
		t.Fatal(err)
	}
	if _, ok := g.Application(f.RulerID); ok {
		t.Fatal("the application stayed")
	}
}

func TestEditState(t *testing.T) {
	t.Parallel()

	a := line()
	g := world.NewGame(t0)
	f := found(t, g, a, "Бореалия", 0)
	citizen := join(t, g, f)

	e := world.Edit{Motto: "Вперёд", Color: 7, Flag: f.Flag}
	if err := g.EditState(citizen, e); !errors.Is(err, world.ErrNotAllowed) {
		t.Fatalf("EditState by a citizen: %v", err)
	}
	if err := g.EditState(uuid.Must(uuid.NewV7()), e); !errors.Is(err, world.ErrNotCitizen) {
		t.Fatalf("EditState by a stranger: %v", err)
	}
	if err := g.EditState(f.RulerID, e); err != nil {
		t.Fatal(err)
	}
	if st, _ := g.State(f.ID); st.Motto != "Вперёд" || st.Color != 7 {
		t.Fatalf("State = %+v", st)
	}
}

// join makes a new person a citizen of the state f.
func join(t *testing.T, g *world.Game, f world.Founding) uuid.UUID {
	t.Helper()

	id := uuid.Must(uuid.NewV7())
	if err := g.Apply(id, f.ID, t0); err != nil {
		t.Fatal(err)
	}
	if err := g.Answer(f.RulerID, id, true, t0); err != nil {
		t.Fatal(err)
	}

	return id
}

func TestCitizens(t *testing.T) {
	t.Parallel()

	a := line()
	g := world.NewGame(t0)
	f := found(t, g, a, "Бореалия", 0)
	other := found(t, g, a, "Нордия", 7)
	ann, bob := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())

	if err := g.Apply(ann, uuid.Must(uuid.NewV7()), t0); !errors.Is(err, world.ErrStateNotFound) {
		t.Fatalf("Apply to nothing: %v", err)
	}
	if err := g.Apply(f.RulerID, other.ID, t0); !errors.Is(err, world.ErrCitizen) {
		t.Fatalf("Apply by a ruler: %v", err)
	}
	if err := g.Apply(ann, other.ID, t0); err != nil {
		t.Fatal(err)
	}
	// A new application replaces the old one.
	if err := g.Apply(ann, f.ID, t0); err != nil {
		t.Fatal(err)
	}
	if err := g.Apply(ann, f.ID, t0); !errors.Is(err, world.ErrApplied) {
		t.Fatalf("Apply twice: %v", err)
	}
	if got := g.Applications(other.ID); len(got) != 0 {
		t.Fatalf("Applications of the first = %v", got)
	}
	if err := g.Answer(other.RulerID, ann, true, t0); !errors.Is(err, world.ErrNoApplication) {
		t.Fatalf("Answer of another state: %v", err)
	}
	if err := g.Answer(f.RulerID, ann, true, t0); err != nil {
		t.Fatal(err)
	}

	// Bob applies; ann, a plain citizen, may not answer.
	if err := g.Apply(bob, f.ID, t0); err != nil {
		t.Fatal(err)
	}
	if err := g.Answer(ann, bob, true, t0); !errors.Is(err, world.ErrNotAllowed) {
		t.Fatalf("Answer by a citizen: %v", err)
	}
	if err := g.Appoint(f.RulerID, ann, world.RoleMinisterInterior, t0); err != nil {
		t.Fatal(err)
	}
	if err := g.Answer(ann, bob, false, t0); err != nil {
		t.Fatal(err)
	}
	if _, ok := g.Citizen(bob); ok {
		t.Fatal("bob turned down is a citizen")
	}
	if err := g.Withdraw(bob); !errors.Is(err, world.ErrNoApplication) {
		t.Fatalf("Withdraw of nothing: %v", err)
	}
	bob = join(t, g, f)

	// The minister of the interior expels citizens, not ministers.
	if err := g.Appoint(f.RulerID, bob, world.RoleMinisterEconomy, t0); err != nil {
		t.Fatal(err)
	}
	if err := g.Expel(ann, bob, t0); !errors.Is(err, world.ErrNotAllowed) {
		t.Fatalf("Expel of a minister: %v", err)
	}
	if err := g.Expel(ann, f.RulerID, t0); !errors.Is(err, world.ErrNotAllowed) {
		t.Fatalf("Expel of the ruler: %v", err)
	}
	if err := g.Expel(f.RulerID, f.RulerID, t0); !errors.Is(err, world.ErrSelf) {
		t.Fatalf("Expel of oneself: %v", err)
	}
	if err := g.Expel(f.RulerID, other.RulerID, t0); !errors.Is(err, world.ErrNotCitizen) {
		t.Fatalf("Expel of a stranger: %v", err)
	}
	if err := g.Expel(f.RulerID, bob, t0); err != nil {
		t.Fatal(err)
	}

	if err := g.Appoint(ann, f.RulerID, world.RoleGeneral, t0); !errors.Is(err, world.ErrNotAllowed) {
		t.Fatalf("Appoint by a minister: %v", err)
	}
	if err := g.Leave(f.RulerID, t0); !errors.Is(err, world.ErrRulerLeaves) {
		t.Fatalf("Leave by the ruler: %v", err)
	}
	if err := g.Leave(ann, t0); err != nil {
		t.Fatal(err)
	}
	if err := g.Leave(ann, t0); !errors.Is(err, world.ErrNotCitizen) {
		t.Fatalf("Leave twice: %v", err)
	}

	kinds := map[world.EventKind]int{}
	for _, e := range g.TakeEvents() {
		kinds[e.Kind]++
	}
	if kinds[world.EventJoined] != 2 || kinds[world.EventAppointed] != 2 || kinds[world.EventExpelled] != 1 || kinds[world.EventLeft] != 1 {
		t.Fatalf("events = %v", kinds)
	}
}

func TestAbdicate(t *testing.T) {
	t.Parallel()

	a := line()
	g := world.NewGame(t0)
	f := found(t, g, a, "Бореалия", 0)
	heir := join(t, g, f)

	if err := g.Abdicate(heir, f.RulerID, t0); !errors.Is(err, world.ErrNotAllowed) {
		t.Fatalf("Abdicate by a citizen: %v", err)
	}
	if err := g.Abdicate(f.RulerID, f.RulerID, t0); !errors.Is(err, world.ErrSelf) {
		t.Fatalf("Abdicate to oneself: %v", err)
	}
	if err := g.Abdicate(f.RulerID, heir, t0); err != nil {
		t.Fatal(err)
	}
	if st, _ := g.State(f.ID); st.RulerID != heir {
		t.Fatalf("the ruler is %v", st.RulerID)
	}
	if c, _ := g.Citizen(f.RulerID); c.Role != world.RoleCitizen {
		t.Fatalf("the old ruler is %s", c.Role)
	}
}

func TestColonize(t *testing.T) {
	t.Parallel()

	a := line()
	g := world.NewGame(t0)
	r := world.DefaultRules
	f := found(t, g, a, "Бореалия", 4) // 3, 4, 5
	citizen := join(t, g, f)

	if err := g.Colonize(a, r, citizen, 2, t0); !errors.Is(err, world.ErrNotAllowed) {
		t.Fatalf("Colonize by a citizen: %v", err)
	}
	if err := g.Colonize(a, r, f.RulerID, 0, t0); !errors.Is(err, world.ErrNotNeighbour) {
		t.Fatalf("Colonize afar: %v", err)
	}
	if err := g.Colonize(a, r, f.RulerID, 5, t0); !errors.Is(err, world.ErrProvinceTaken) {
		t.Fatalf("Colonize of its own: %v", err)
	}
	if err := g.Colonize(a, r, f.RulerID, 10, t0); !errors.Is(err, world.ErrNotProvince) {
		t.Fatalf("Colonize at sea: %v", err)
	}

	// 3 provinces: 200 × 1.3 votes.
	if cost := g.ColonyCost(r, f.ID); cost != 26000 {
		t.Fatalf("ColonyCost = %d", cost)
	}
	if err := g.Colonize(a, r, f.RulerID, 2, t0); err != nil {
		t.Fatal(err)
	}
	if st, _ := g.State(f.ID); st.Treasury != 100000-26000 || st.Budget[0].Colonization != 26000 {
		t.Fatalf("State = %+v", st)
	}
	if err := g.Appoint(f.RulerID, citizen, world.RoleMinisterEconomy, t0); err != nil {
		t.Fatal(err)
	}
	if err := g.Colonize(a, r, citizen, 2, t0); !errors.Is(err, world.ErrProvinceTaken) {
		t.Fatalf("Colonize of a colony: %v", err)
	}
	// One colonist for 3 provinces.
	if err := g.Colonize(a, r, citizen, 6, t0); !errors.Is(err, world.ErrNoColonists) {
		t.Fatalf("a second colony: %v", err)
	}

	// Settled after 12 hours, counted by the hour.
	g.Tick(a, r, t0.Add(11*time.Hour))
	if g.Owner(2) != uuid.Nil {
		t.Fatal("settled too early")
	}
	g.Tick(a, r, t0.Add(13*time.Hour))
	if g.Owner(2) != f.ID || len(g.ColoniesOf(f.ID)) != 0 {
		t.Fatalf("Owner = %v, colonies %v", g.Owner(2), g.ColoniesOf(f.ID))
	}

	poor := r
	poor.ColonizeCost = 1_000_000
	if err := g.Colonize(a, poor, f.RulerID, 6, t0); !errors.Is(err, world.ErrTreasury) {
		t.Fatalf("Colonize without the votes: %v", err)
	}
}

func TestTick(t *testing.T) {
	t.Parallel()

	a := line()
	g := world.NewGame(t0)
	r := world.DefaultRules
	f := found(t, g, a, "Бореалия", 1) // 0, 1, 2: 100 thousand each

	// 300 thousand × 5 hundredths per 10 thousand.
	if got := g.Income(a, r, f.ID); got != 150 {
		t.Fatalf("Income = %d", got)
	}
	if hours := g.Tick(a, r, t0); hours != 0 {
		t.Fatalf("Tick within the hour = %d", hours)
	}
	if hours := g.Tick(a, r, t0.Add(2*time.Hour)); hours != 2 {
		t.Fatalf("Tick = %d", hours)
	}
	st, _ := g.State(f.ID)
	if st.Treasury != 100000+300 || st.Budget[len(st.Budget)-1].Income != 150 {
		t.Fatalf("State = %d, %+v", st.Treasury, st.Budget)
	}

	// A week away counts one day.
	if hours := g.Tick(a, r, t0.Add(7*24*time.Hour)); hours != 24 {
		t.Fatalf("Tick after a week = %d", hours)
	}
	if st, _ = g.State(f.ID); len(st.Budget) != 24 {
		t.Fatalf("the budget keeps %d hours", len(st.Budget))
	}
}

func TestDonate(t *testing.T) {
	t.Parallel()

	a := line()
	g := world.NewGame(t0)
	f := found(t, g, a, "Бореалия", 0)

	if err := g.CheckDonate(uuid.Must(uuid.NewV7()), 5); !errors.Is(err, world.ErrNotCitizen) {
		t.Fatalf("CheckDonate by a stranger: %v", err)
	}
	if err := g.CheckDonate(f.RulerID, 0); !errors.Is(err, world.ErrDonation) {
		t.Fatalf("CheckDonate of nothing: %v", err)
	}
	if err := g.Donate(f.RulerID, 5, t0); err != nil {
		t.Fatal(err)
	}
	if st, _ := g.State(f.ID); st.Treasury != 100500 || st.Budget[0].Donations != 500 {
		t.Fatalf("State = %+v", st)
	}
}

func TestPurge(t *testing.T) {
	t.Parallel()

	a := line()
	g := world.NewGame(t0)
	f := found(t, g, a, "Бореалия", 1)
	citizen, general := join(t, g, f), join(t, g, f)
	if err := g.Appoint(f.RulerID, general, world.RoleGeneral, t0); err != nil {
		t.Fatal(err)
	}

	// The general outranks the older citizen.
	g.Purge(f.RulerID, t0)
	if st, _ := g.State(f.ID); st.RulerID != general {
		t.Fatalf("the ruler after the purge is %v", st.RulerID)
	}
	g.Purge(general, t0)
	if st, _ := g.State(f.ID); st.RulerID != citizen {
		t.Fatalf("the ruler after the second purge is %v", st.RulerID)
	}

	applicant := uuid.Must(uuid.NewV7())
	if err := g.Apply(applicant, f.ID, t0); err != nil {
		t.Fatal(err)
	}
	if err := g.Colonize(a, world.DefaultRules, citizen, 3, t0); err != nil {
		t.Fatal(err)
	}
	g.Purge(citizen, t0)
	if _, ok := g.State(f.ID); ok || len(g.Lands(f.ID)) != 0 || len(g.Colonies()) != 0 {
		t.Fatal("the state without people stayed")
	}
	if _, ok := g.Application(applicant); ok {
		t.Fatal("the application to a dissolved state stayed")
	}

	events := g.TakeEvents()
	if last := events[len(events)-1]; last.Kind != world.EventDissolved || last.Name != "Бореалия" {
		t.Fatalf("the last event is %+v", last)
	}
	// Purging someone unknown changes nothing.
	g.Purge(uuid.Must(uuid.NewV7()), t0)
}

func TestSnapshotRestores(t *testing.T) {
	t.Parallel()

	a := line()
	g := world.NewGame(t0)
	f := found(t, g, a, "Бореалия", 1)
	join(t, g, f)
	if err := g.Apply(uuid.Must(uuid.NewV7()), f.ID, t0); err != nil {
		t.Fatal(err)
	}
	if err := g.Colonize(a, world.DefaultRules, f.RulerID, 3, t0); err != nil {
		t.Fatal(err)
	}

	s := g.Snapshot()
	back := world.RestoreGame(s).Snapshot()
	if len(back.States) != 1 || len(back.Citizens) != 2 || len(back.Applications) != 1 || len(back.Colonies) != 1 ||
		len(back.Owners) != 3 || !back.Hour.Equal(s.Hour) || back.States[0].Flag.Parts().Pattern != world.PatternTricolorH {
		t.Fatalf("restored %+v", back)
	}

	standings := world.RestoreGame(s).Standings(a, world.DefaultRules)
	if len(standings) != 1 || standings[0].Lands != 3 || standings[0].Population != 300 || standings[0].Citizens != 2 || standings[0].Income != 150 {
		t.Fatalf("Standings = %+v", standings)
	}
}
