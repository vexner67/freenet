package world_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/world/internal/world"
)

var t0 = time.Date(2026, 10, 28, 10, 30, 0, 0, time.UTC)

// line is an atlas of a strip of land 10 provinces long, 0 to 9, each of
// 100 thousand people, with the sea 10 along it and the free city 11 off
// province 9.
func line() *world.Atlas {
	a := &world.Atlas{Width: 64, Height: 32}
	for i := range 10 {
		p := world.Province{ID: i, Name: "P", Terrain: world.TerrainPlains, Population: 100, Links: []world.Link{{To: 10}}}
		if i > 0 {
			p.Links = append(p.Links, world.Link{To: i - 1})
		}
		if i < 9 {
			p.Links = append(p.Links, world.Link{To: i + 1})
		}
		a.Provinces = append(a.Provinces, p)
	}
	a.Provinces = append(a.Provinces,
		world.Province{ID: 10, Sea: true, Terrain: world.TerrainSea},
		world.Province{ID: 11, FreeCity: true, Terrain: world.TerrainPlains, Population: 50, Links: []world.Link{{To: 9}}},
	)
	a.Provinces[9].Links = append(a.Provinces[9].Links, world.Link{To: 11})

	return a
}

func founding(t *testing.T, name string, capital int) world.Founding {
	t.Helper()

	n, err := world.NewStateName(name)
	if err != nil {
		t.Fatal(err)
	}
	flag, err := world.NewFlag(world.FlagParts{Pattern: world.PatternTricolorH, Colors: []int{1, 2, 3}})
	if err != nil {
		t.Fatal(err)
	}

	return world.Founding{
		ID: uuid.Must(uuid.NewV7()), RulerID: uuid.Must(uuid.NewV7()), Name: n, Flag: flag,
		Government: world.GovernmentMonarchy, Capital: capital,
	}
}

func found(t *testing.T, g *world.Game, a *world.Atlas, name string, capital int) world.Founding {
	t.Helper()

	f := founding(t, name, capital)
	if err := g.Found(a, world.DefaultRules, f, t0); err != nil {
		t.Fatalf("Found(%s): %v", name, err)
	}

	return f
}

func TestWorldLife(t *testing.T) {
	t.Parallel()

	if _, err := world.New(1, 1000, t0); !errors.Is(err, world.ErrGrid) {
		t.Fatalf("New on a bad grid: %v", err)
	}

	w, err := world.New(1, 2048, t0)
	if err != nil || w.Status() != world.StatusGenerating {
		t.Fatalf("New = %v, %v", w, err)
	}
	if err = w.Open(t0); !errors.Is(err, world.ErrNotReady) {
		t.Fatalf("Open before the map: %v", err)
	}
	if err = w.End(t0); !errors.Is(err, world.ErrNotOpen) {
		t.Fatalf("End before it opened: %v", err)
	}

	w.Made()
	if err = w.Open(t0); err != nil || w.Status() != world.StatusOpen || !w.OpenedAt().Equal(t0) {
		t.Fatalf("Open = %v, %v", w.Status(), err)
	}
	if err = w.End(t0); err != nil || w.Status() != world.StatusEnded {
		t.Fatalf("End = %v, %v", w.Status(), err)
	}

	f, _ := world.New(2, 1024, t0)
	f.Failed()
	f.Made()
	if f.Status() != world.StatusFailed {
		t.Fatalf("a failed world became %s", f.Status())
	}
}

func TestRulesValidate(t *testing.T) {
	t.Parallel()

	if err := world.DefaultRules.Validate(); err != nil {
		t.Fatalf("DefaultRules: %v", err)
	}

	tests := []func(*world.Rules){
		func(r *world.Rules) { r.FoundCost = -1 },
		func(r *world.Rules) { r.FoundTreasury = 20_000_000 },
		func(r *world.Rules) { r.FoundLands = 11 },
		func(r *world.Rules) { r.CapitalDistance = 0 },
		func(r *world.Rules) { r.ProtectionDays = 61 },
		func(r *world.Rules) { r.ColonizeCost = -5 },
		func(r *world.Rules) { r.ColonizeHours = 0 },
		func(r *world.Rules) { r.Income = 1001 },
	}
	for i, edit := range tests {
		r := world.DefaultRules
		edit(&r)
		if err := r.Validate(); !errors.Is(err, world.ErrRules) {
			t.Errorf("case %d: %v", i, err)
		}
	}
}

func TestValueObjects(t *testing.T) {
	t.Parallel()

	long := func(n int) string {
		s := ""
		for range n {
			s += "ж"
		}
		return s
	}

	if n, err := world.NewStateName("  Бореалия \n Великая "); err != nil || n != "Бореалия Великая" {
		t.Errorf("NewStateName = %q, %v", n, err)
	}
	for _, bad := range []string{"", "   ", long(41)} {
		if _, err := world.NewStateName(bad); !errors.Is(err, world.ErrStateName) {
			t.Errorf("NewStateName(%q) = %v", bad, err)
		}
	}
	if _, err := world.NewStateName(long(40)); err != nil {
		t.Errorf("a name of 40 letters: %v", err)
	}
	if _, err := world.NewAdjective(long(41)); !errors.Is(err, world.ErrAdjective) {
		t.Errorf("NewAdjective: %v", err)
	}
	if m, err := world.NewMotto(""); err != nil || m != "" {
		t.Errorf("an empty motto: %v", err)
	}
	if _, err := world.NewMotto(long(81)); !errors.Is(err, world.ErrMotto) {
		t.Errorf("NewMotto: %v", err)
	}
	for _, c := range []int{-1, world.Colors} {
		if _, err := world.NewColor(c); !errors.Is(err, world.ErrColor) {
			t.Errorf("NewColor(%d) = %v", c, err)
		}
	}
	if _, err := world.NewGovernment("anarchy"); !errors.Is(err, world.ErrGovernment) {
		t.Errorf("NewGovernment: %v", err)
	}
	if _, err := world.NewRole(string(world.RoleRuler)); !errors.Is(err, world.ErrRole) {
		t.Errorf("NewRole of the ruler: %v", err)
	}
}

func TestNewFlag(t *testing.T) {
	t.Parallel()

	good := []world.FlagParts{
		{Pattern: world.PatternPlain, Colors: []int{0}},
		{Pattern: world.PatternNordic, Colors: []int{4, 23}, Emblem: "star", EmblemColor: 5, EmblemAt: world.EmblemCanton},
	}
	for _, p := range good {
		if _, err := world.NewFlag(p); err != nil {
			t.Errorf("NewFlag(%v) = %v", p, err)
		}
	}

	f, _ := world.NewFlag(world.FlagParts{Pattern: world.PatternPlain, Colors: []int{0}, EmblemAt: "anywhere"})
	if f.Parts().EmblemAt != world.EmblemCenter {
		t.Errorf("a flag without an emblem has it at %q", f.Parts().EmblemAt)
	}

	bad := []world.FlagParts{
		{Pattern: "stars", Colors: []int{0}},
		{Pattern: world.PatternPlain, Colors: []int{0, 1}},
		{Pattern: world.PatternBicolorH, Colors: []int{0, world.Colors}},
		{Pattern: world.PatternPlain, Colors: []int{0}, Emblem: "dragon"},
		{Pattern: world.PatternPlain, Colors: []int{0}, Emblem: "sun", EmblemAt: "corner"},
		{Pattern: world.PatternPlain, Colors: []int{0}, Emblem: "sun", EmblemColor: -1, EmblemAt: world.EmblemCenter},
	}
	for _, p := range bad {
		if _, err := world.NewFlag(p); !errors.Is(err, world.ErrFlag) {
			t.Errorf("NewFlag(%v) = %v", p, err)
		}
	}

	if got := world.RestoreFlag(world.FlagParts{Pattern: "gone"}); got.Parts().Pattern != world.PatternPlain {
		t.Errorf("RestoreFlag of an unknown pattern = %v", got.Parts())
	}
}
