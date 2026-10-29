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

// fakeWorld is an open world; calls records what was asked of it.
type fakeWorld struct {
	calls []string
	seed  uint64
	tile  [3]int
	none  bool
}

var worldAt = time.Date(2027, 1, 25, 10, 0, 0, 0, time.UTC)

func (f *fakeWorld) Current(context.Context) (model.World, error) {
	if f.none {
		return model.World{}, failure.New(failure.NotFound, "no world is open")
	}
	return model.World{ID: "w1", Seed: 1 << 63, Grid: 2048, Status: "open", CreatedAt: worldAt, OpenedAt: worldAt}, nil
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

func (f *fakeWorld) did(call string) error {
	f.calls = append(f.calls, call)
	return nil
}

func (f *fakeWorld) Worlds(context.Context) ([]model.World, error) {
	return []model.World{{ID: "w1", Status: "ready", CreatedAt: worldAt}}, nil
}

func (f *fakeWorld) Create(_ context.Context, seed uint64, grid int) (model.World, error) {
	f.seed = seed
	return model.World{ID: "w2", Seed: seed, Grid: grid, Status: "generating", CreatedAt: worldAt}, nil
}

func (f *fakeWorld) Open(_ context.Context, worldID string) error { return f.did("open " + worldID) }
func (f *fakeWorld) End(context.Context) error                    { return f.did("end") }

func TestWorldMap(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("GET", "/api/v1/world", "")
	got := decode[map[string]any](t, w)
	if w.Code != http.StatusOK {
		t.Fatalf("%d %v", w.Code, got)
	}

	// The seed is a string: a JSON number would lose its digits.
	world := got["world"].(map[string]any)
	if world["seed"] != "9223372036854775808" || world["endedAt"] != nil || world["status"] != "open" {
		t.Fatalf("map: %v", got)
	}

	h.world.none = true
	if w = h.do("GET", "/api/v1/world", ""); w.Code != http.StatusNotFound {
		t.Fatalf("no world: %d", w.Code)
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
	} {
		if w := h.do(c.method, c.path, c.body, auth...); w.Code != http.StatusForbidden {
			t.Fatalf("%s %s: %d", c.method, c.path, w.Code)
		}
	}
	if len(h.world.calls) != 0 {
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

	got := decode[map[string]any](t, h.do("GET", "/api/v1/world/worlds", "", auth...))
	if len(got["worlds"].([]any)) != 1 {
		t.Fatalf("worlds: %v", got)
	}

	h.do("POST", "/api/v1/world/worlds/w1/open", "", auth...)
	h.do("POST", "/api/v1/world/end", "", auth...)
	if len(h.world.calls) != 2 || h.world.calls[0] != "open w1" || h.world.calls[1] != "end" {
		t.Fatalf("calls %v", h.world.calls)
	}
}
