package rest_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

// fakeWorld is an open world with one state of ann (u1); calls records
// what was asked of it.
type fakeWorld struct {
	token string
	calls []string
	found model.Founding
	seed  uint64
	tile  [3]int
	rules model.WorldRules
	none  bool
}

var worldAt = time.Date(2027, 1, 25, 10, 0, 0, 0, time.UTC)

func (f *fakeWorld) state() model.State {
	return model.State{ID: "s1", Name: "Ruritania", RulerID: "u1", Capital: 3, Treasury: 100000,
		Flag: model.Flag{Pattern: "bicolor_h", Colors: []int{1, 2}}, FoundedAt: worldAt, Lands: 3}
}

func (f *fakeWorld) Map(_ context.Context, token string) (model.WorldMap, error) {
	f.token = token
	return model.WorldMap{
		World:  model.World{ID: "w1", Seed: 1 << 63, Grid: 2048, Status: "open", CreatedAt: worldAt, OpenedAt: worldAt},
		Rules:  model.WorldRules{FoundCost: 5000},
		Hour:   worldAt,
		States: []model.State{f.state()},
		Owners: []int{0, 1, 1, 1},
		Viewer: model.Standing{StateID: "s1", Role: "ruler"},
	}, nil
}

func (f *fakeWorld) Atlas(_ context.Context, worldID string) (model.Atlas, error) {
	if worldID != "w1" {
		return model.Atlas{}, failure.New(failure.NotFound, "world not found")
	}
	return model.Atlas{Width: 4, Height: 2, Zooms: 3, Provinces: []model.Province{{ID: 0, Sea: true}, {ID: 1, Name: "Vale", Links: []model.ProvinceLink{{To: 0, Sea: true}}}}}, nil
}

func (f *fakeWorld) Raster(context.Context, string) ([]byte, error) { return []byte{0x1f, 0x8b}, nil }

func (f *fakeWorld) Tile(_ context.Context, _ string, z, x, y int) ([]byte, error) {
	f.tile = [3]int{z, x, y}
	return []byte("png"), nil
}

func (f *fakeWorld) State(_ context.Context, token, _ string) (model.StateView, error) {
	f.token = token
	return model.StateView{
		State:        f.state(),
		Lands:        []int{1, 2, 3},
		Citizens:     []model.Citizen{{UserID: "u1", Role: "ruler"}, {UserID: "gone", Role: "citizen"}},
		Applications: []model.CitizenshipApplication{{UserID: "u2", At: worldAt}},
		Budget:       []model.BudgetHour{{Hour: worldAt, Income: 50}},
	}, nil
}

func (f *fakeWorld) Events(_ context.Context, stateID, before string, limit int) ([]model.WorldEvent, error) {
	f.calls = append(f.calls, "events "+stateID+" "+before)
	return []model.WorldEvent{{ID: "e1", Kind: "founded", StateID: "s1", UserID: "u1", Name: "Ruritania", At: worldAt}, {ID: "e2", Kind: "colonized", StateID: "s1", Province: 4}}, nil
}

func (f *fakeWorld) Citizenship(_ context.Context, userID string) (model.State, string, error) {
	if f.none {
		return model.State{}, "", failure.New(failure.NotFound, "no world is open")
	}
	if userID != "u1" {
		return model.State{}, "", nil
	}
	return f.state(), "ruler", nil
}

func (f *fakeWorld) Found(_ context.Context, token string, fd model.Founding) (string, error) {
	f.token, f.found = token, fd
	return "s2", nil
}

func (f *fakeWorld) did(token, call string) error {
	f.token = token
	f.calls = append(f.calls, call)
	return nil
}

func (f *fakeWorld) Edit(_ context.Context, token string, l model.StateLook) error {
	return f.did(token, "edit "+l.Motto)
}
func (f *fakeWorld) Apply(_ context.Context, token, stateID string) error {
	return f.did(token, "apply "+stateID)
}
func (f *fakeWorld) Withdraw(_ context.Context, token string) error { return f.did(token, "withdraw") }
func (f *fakeWorld) Answer(_ context.Context, token, userID string, accept bool) error {
	if accept {
		return f.did(token, "accept "+userID)
	}
	return f.did(token, "reject "+userID)
}
func (f *fakeWorld) Leave(_ context.Context, token string) error { return f.did(token, "leave") }
func (f *fakeWorld) Expel(_ context.Context, token, userID string) error {
	return f.did(token, "expel "+userID)
}
func (f *fakeWorld) Appoint(_ context.Context, token, userID, role string) error {
	return f.did(token, "appoint "+userID+" "+role)
}
func (f *fakeWorld) Abdicate(_ context.Context, token, heirID string) error {
	return f.did(token, "abdicate "+heirID)
}
func (f *fakeWorld) Colonize(_ context.Context, token string, province int) error {
	if province == 0 {
		return failure.New(failure.Conflict, "the province is not free")
	}
	return f.did(token, "colonize")
}
func (f *fakeWorld) Donate(_ context.Context, token string, votes int64) error {
	if votes != 10 {
		return failure.New(failure.InvalidArgument, "invalid donation")
	}
	return f.did(token, "donate")
}

func (f *fakeWorld) Worlds(context.Context) ([]model.World, error) {
	return []model.World{{ID: "w1", Status: "ready", CreatedAt: worldAt}}, nil
}

func (f *fakeWorld) Create(_ context.Context, seed uint64, grid int) (model.World, error) {
	f.seed = seed
	return model.World{ID: "w2", Seed: seed, Grid: grid, Status: "generating", CreatedAt: worldAt}, nil
}

func (f *fakeWorld) Open(_ context.Context, worldID string) error { return f.did("", "open "+worldID) }
func (f *fakeWorld) End(context.Context) error                    { return f.did("", "end") }
func (f *fakeWorld) Rules(context.Context) (model.WorldRules, error) {
	return f.rules, nil
}
func (f *fakeWorld) SetRules(_ context.Context, r model.WorldRules) error {
	f.rules = r
	return nil
}

func TestWorldMap(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("GET", "/api/v1/world", "")
	got := decode[map[string]any](t, w)
	if w.Code != http.StatusOK || h.world.token != "" {
		t.Fatalf("%d %v", w.Code, got)
	}

	// The seed is a string: a JSON number would lose its digits.
	world := got["world"].(map[string]any)
	state := got["states"].([]any)[0].(map[string]any)
	if world["seed"] != "9223372036854775808" || world["endedAt"] != nil || state["ruler"].(map[string]any)["nickname"] != "ann" ||
		len(got["owners"].([]any)) != 4 || got["viewer"].(map[string]any)["role"] != "ruler" {
		t.Fatalf("map: %v", got)
	}

	if h.do("GET", "/api/v1/world", "", "Authorization", "Bearer tok"); h.world.token != "tok" {
		t.Fatalf("the viewer is not told: %q", h.world.token)
	}
}

func TestWorldMapFiles(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})

	w := h.do("GET", "/media/world/w1/atlas", "")
	atlas := decode[map[string]any](t, w)
	if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" || len(atlas["provinces"].([]any)) != 2 || atlas["zooms"] != 3.0 {
		t.Fatalf("atlas: %d %v %v", w.Code, w.Header(), atlas)
	}
	if w = h.do("GET", "/media/world/w9/atlas", ""); w.Code != http.StatusNotFound {
		t.Fatalf("atlas of no world: %d", w.Code)
	}

	if w = h.do("GET", "/media/world/w1/raster", ""); w.Code != http.StatusOK || w.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("raster: %d %v", w.Code, w.Header())
	}

	if w = h.do("GET", "/media/world/w1/tiles/2/3/1", ""); w.Code != http.StatusOK || w.Header().Get("Content-Type") != "image/png" || h.world.tile != [3]int{2, 3, 1} {
		t.Fatalf("tile: %d %v %v", w.Code, w.Header(), h.world.tile)
	}
	for _, path := range []string{"/media/world/w1/tiles/a/0/0", "/media/world/w1/tiles/0/-1/0", "/media/world/w1/tiles/0/0/99999"} {
		if w = h.do("GET", path, ""); w.Code != http.StatusNotFound {
			t.Fatalf("%s: %d", path, w.Code)
		}
	}
}

func TestWorldState(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("GET", "/api/v1/world/states/s1", "", "Authorization", "Bearer tok")
	got := decode[map[string]any](t, w)

	// Who is not found among the users is left out.
	if w.Code != http.StatusOK || h.world.token != "tok" || len(got["citizens"].([]any)) != 1 ||
		got["applications"].([]any)[0].(map[string]any)["user"].(map[string]any)["nickname"] != "bob" || len(got["budget"].([]any)) != 1 {
		t.Fatalf("%d %v", w.Code, got)
	}

	w = h.do("GET", "/api/v1/world/events?state=s1&before=e9", "")
	events := decode[map[string]any](t, w)["events"].([]any)
	if w.Code != http.StatusOK || len(events) != 2 || events[1].(map[string]any)["user"] != nil || h.world.calls[0] != "events s1 e9" {
		t.Fatalf("events: %d %v %v", w.Code, events, h.world.calls)
	}

	got = decode[map[string]any](t, h.do("GET", "/api/v1/users/ann/world", ""))
	if got["state"].(map[string]any)["name"] != "Ruritania" || got["role"] != "ruler" {
		t.Fatalf("citizenship: %v", got)
	}
	if got = decode[map[string]any](t, h.do("GET", "/api/v1/users/bob/world", "")); got["state"] != nil {
		t.Fatalf("no citizenship: %v", got)
	}
	h.world.none = true
	if w = h.do("GET", "/api/v1/users/ann/world", ""); w.Code != http.StatusOK || decode[map[string]any](t, w)["state"] != nil {
		t.Fatalf("no world: %d", w.Code)
	}
}

func TestWorldActions(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	auth := []string{"Authorization", "Bearer tok"}

	w := h.do("POST", "/api/v1/world/states", `{"name":"Ruritania","capital":3,"flag":{"pattern":"plain","colors":[4]}}`, auth...)
	if w.Code != http.StatusCreated || decode[map[string]any](t, w)["id"] != "s2" || h.world.found.Capital != 3 || h.world.found.Flag.Colors[0] != 4 {
		t.Fatalf("found: %d %+v", w.Code, h.world.found)
	}

	for _, c := range []struct{ method, path, body, call string }{
		{"PUT", "/api/v1/world/state", `{"motto":"Onwards"}`, "edit Onwards"},
		{"POST", "/api/v1/world/states/s1/applications", "", "apply s1"},
		{"DELETE", "/api/v1/world/application", "", "withdraw"},
		{"PUT", "/api/v1/world/applications/u2", "", "accept u2"},
		{"DELETE", "/api/v1/world/applications/u2", "", "reject u2"},
		{"DELETE", "/api/v1/world/citizenship", "", "leave"},
		{"DELETE", "/api/v1/world/citizens/u2", "", "expel u2"},
		{"PUT", "/api/v1/world/citizens/u2/role", `{"role":"general"}`, "appoint u2 general"},
		{"POST", "/api/v1/world/abdication", `{"heirId":"u2"}`, "abdicate u2"},
		{"POST", "/api/v1/world/colonies", `{"province":5}`, "colonize"},
		{"POST", "/api/v1/world/donations", `{"votes":10}`, "donate"},
	} {
		h.world.calls = nil
		if w = h.do(c.method, c.path, c.body, auth...); w.Code != http.StatusNoContent || len(h.world.calls) != 1 || h.world.calls[0] != c.call || h.world.token != "tok" {
			t.Fatalf("%s %s: %d %v", c.method, c.path, w.Code, h.world.calls)
		}
		if w = h.do(c.method, c.path, c.body); w.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s without token: %d", c.method, c.path, w.Code)
		}
	}

	if w = h.do("POST", "/api/v1/world/colonies", `{"province":0}`, auth...); w.Code != http.StatusConflict {
		t.Fatalf("taken: %d", w.Code)
	}
	if w = h.do("POST", "/api/v1/world/donations", `{"votes":-1}`, auth...); w.Code != http.StatusBadRequest {
		t.Fatalf("bad donation: %d", w.Code)
	}
}

func TestWorldAdmin(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	auth := []string{"Authorization", "Bearer tok"}

	// Only the administrators of the site.
	for _, c := range []struct{ method, path, body string }{
		{"GET", "/api/v1/world/worlds", ""},
		{"POST", "/api/v1/world/worlds", `{"grid":2048}`},
		{"POST", "/api/v1/world/worlds/w1/open", ""},
		{"POST", "/api/v1/world/end", ""},
		{"PUT", "/api/v1/world/rules", `{"foundCost":1}`},
	} {
		if w := h.do(c.method, c.path, c.body, auth...); w.Code != http.StatusForbidden {
			t.Fatalf("%s %s: %d", c.method, c.path, w.Code)
		}
	}
	if len(h.world.calls) != 0 || h.world.rules.FoundCost != 0 {
		t.Fatalf("called %v", h.world.calls)
	}

	h.users.admin = true

	w := h.do("POST", "/api/v1/world/worlds", `{"seed":"18446744073709551615","grid":2048}`, auth...)
	if w.Code != http.StatusCreated || h.world.seed != 1<<64-1 || decode[map[string]any](t, w)["status"] != "generating" {
		t.Fatalf("create: %d %d", w.Code, h.world.seed)
	}
	if w = h.do("POST", "/api/v1/world/worlds", `{"seed":"many"}`, auth...); w.Code != http.StatusBadRequest {
		t.Fatalf("bad seed: %d", w.Code)
	}

	if w = h.do("PUT", "/api/v1/world/rules", `{"foundCost":7,"income":3}`, auth...); w.Code != http.StatusNoContent || h.world.rules.FoundCost != 7 {
		t.Fatalf("rules: %d %+v", w.Code, h.world.rules)
	}

	got := decode[map[string]any](t, h.do("GET", "/api/v1/world/worlds", "", auth...))
	if len(got["worlds"].([]any)) != 1 || got["rules"].(map[string]any)["income"] != float64(3) {
		t.Fatalf("worlds: %v", got)
	}

	h.do("POST", "/api/v1/world/worlds/w1/open", "", auth...)
	h.do("POST", "/api/v1/world/end", "", auth...)
	if len(h.world.calls) != 2 || h.world.calls[0] != "open w1" || h.world.calls[1] != "end" {
		t.Fatalf("calls %v", h.world.calls)
	}
}
