package rest

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
)

// World is "Мир", which the world service keeps: the worlds and their
// maps, made by the administrators of the site, whom the gateway checks
// itself.
type World interface {
	Current(ctx context.Context) (model.World, error)
	Atlas(ctx context.Context, worldID string) (model.Atlas, error)
	Raster(ctx context.Context, worldID string) ([]byte, error)
	Tile(ctx context.Context, worldID string, z, x, y int) ([]byte, error)

	Worlds(ctx context.Context) ([]model.World, error)
	Create(ctx context.Context, seed uint64, grid int) (model.World, error)
	Open(ctx context.Context, worldID string) error
	End(ctx context.Context) error
}

type worldJSON struct {
	ID        string     `json:"id"`
	Seed      string     `json:"seed"`
	Grid      int        `json:"grid"`
	Status    string     `json:"status"`
	CreatedAt time.Time  `json:"createdAt"`
	OpenedAt  *time.Time `json:"openedAt"`
	EndedAt   *time.Time `json:"endedAt"`
}

type provinceLinkJSON struct {
	To    int  `json:"to"`
	River bool `json:"river,omitempty"`
	Sea   bool `json:"sea,omitempty"`
}

type provinceJSON struct {
	ID         int                `json:"id"`
	Sea        bool               `json:"sea,omitempty"`
	Name       string             `json:"name,omitempty"`
	Terrain    string             `json:"terrain,omitempty"`
	Biome      string             `json:"biome,omitempty"`
	Resources  []string           `json:"resources,omitempty"`
	CenterX    int                `json:"x"`
	CenterY    int                `json:"y"`
	Area       int                `json:"area"`
	Coastal    bool               `json:"coastal,omitempty"`
	Culture    int                `json:"culture,omitempty"`
	Population int                `json:"population,omitempty"`
	FreeCity   bool               `json:"freeCity,omitempty"`
	Links      []provinceLinkJSON `json:"links"`
}

type atlasJSON struct {
	Width     int            `json:"width"`
	Height    int            `json:"height"`
	Zooms     int            `json:"zooms"`
	Provinces []provinceJSON `json:"provinces"`
}

// world is the world open now, whose map the site shows.
func (a *api) world(w http.ResponseWriter, r *http.Request) {
	world, err := a.worlds.Current(r.Context())
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, struct {
		World worldJSON `json:"world"`
	}{World: toWorldJSON(world)})
}

// worldAtlas, worldRaster and worldTile are the map of a world: they never
// change, so they are cached for good under the world's ID.
func (a *api) worldAtlas(w http.ResponseWriter, r *http.Request) {
	atlas, err := a.worlds.Atlas(r.Context(), r.PathValue("world"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	resp := atlasJSON{Width: atlas.Width, Height: atlas.Height, Zooms: atlas.Zooms, Provinces: make([]provinceJSON, 0, len(atlas.Provinces))}
	for _, p := range atlas.Provinces {
		links := make([]provinceLinkJSON, 0, len(p.Links))
		for _, l := range p.Links {
			links = append(links, provinceLinkJSON(l))
		}

		resp.Provinces = append(resp.Provinces, provinceJSON{
			ID:         p.ID,
			Sea:        p.Sea,
			Name:       p.Name,
			Terrain:    p.Terrain,
			Biome:      p.Biome,
			Resources:  p.Resources,
			CenterX:    p.CenterX,
			CenterY:    p.CenterY,
			Area:       p.Area,
			Coastal:    p.Coastal,
			Culture:    p.Culture,
			Population: p.Population,
			FreeCity:   p.FreeCity,
			Links:      links,
		})
	}

	w.Header().Set("Cache-Control", cacheForever)
	writeJSON(w, http.StatusOK, resp)
}

// worldRaster is the province of every pixel of the map, little-endian
// uint16, gzipped as the world service keeps it: the browser unpacks it
// by Content-Encoding.
func (a *api) worldRaster(w http.ResponseWriter, r *http.Request) {
	raster, err := a.worlds.Raster(r.Context(), r.PathValue("world"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Encoding", "gzip")
	w.Header().Set("Cache-Control", cacheForever)
	_, _ = w.Write(raster)
}

func (a *api) worldTile(w http.ResponseWriter, r *http.Request) {
	var place [3]int
	for i, name := range []string{"z", "x", "y"} {
		n, err := strconv.Atoi(r.PathValue(name))
		if err != nil || n < 0 || n > maxTileIndex {
			a.writeError(w, r, failure.New(failure.NotFound, "tile not found"))
			return
		}
		place[i] = n
	}

	png, err := a.worlds.Tile(r.Context(), r.PathValue("world"), place[0], place[1], place[2])
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", cacheForever)
	_, _ = w.Write(png)
}

// maxTileIndex bounds the zoom and the place of a tile, far above any map.
const maxTileIndex = 1 << 12

// worldDone answers an action: 204, or its error.
func (a *api) worldDone(w http.ResponseWriter, r *http.Request, err error) {
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// worldAdmin lets the administrators of the site alone through: the world
// service takes their calls from the gateway, not from them.
func (a *api) worldAdmin(w http.ResponseWriter, r *http.Request) bool {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return false
	}

	if _, err = a.adminID(r, token); err != nil {
		a.writeError(w, r, err)
		return false
	}

	return true
}

// listWorlds is "Мир" of the admin panel: the maps made.
func (a *api) listWorlds(w http.ResponseWriter, r *http.Request) {
	if !a.worldAdmin(w, r) {
		return
	}

	worlds, err := a.worlds.Worlds(r.Context())
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	resp := struct {
		Worlds []worldJSON `json:"worlds"`
	}{Worlds: make([]worldJSON, 0, len(worlds))}

	for _, world := range worlds {
		resp.Worlds = append(resp.Worlds, toWorldJSON(world))
	}

	writeJSON(w, http.StatusOK, resp)
}

// createWorld starts making a map; seed "" takes a random one. The seed is
// a string: JSON numbers lose the digits of a large one.
func (a *api) createWorld(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Seed string `json:"seed"`
		Grid int    `json:"grid"`
	}
	if err := decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	var seed uint64
	if req.Seed != "" {
		var err error
		if seed, err = strconv.ParseUint(req.Seed, 10, 64); err != nil {
			a.writeError(w, r, failure.New(failure.InvalidArgument, "seed must be a number"))
			return
		}
	}

	if !a.worldAdmin(w, r) {
		return
	}

	world, err := a.worlds.Create(r.Context(), seed, req.Grid)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, toWorldJSON(world))
}

// openWorld shows a ready map on the site, ending the one open now.
func (a *api) openWorld(w http.ResponseWriter, r *http.Request) {
	if !a.worldAdmin(w, r) {
		return
	}

	a.worldDone(w, r, a.worlds.Open(r.Context(), r.PathValue("id")))
}

func (a *api) endWorld(w http.ResponseWriter, r *http.Request) {
	if !a.worldAdmin(w, r) {
		return
	}

	a.worldDone(w, r, a.worlds.End(r.Context()))
}

func toWorldJSON(w model.World) worldJSON {
	return worldJSON{
		ID:        w.ID,
		Seed:      strconv.FormatUint(w.Seed, 10),
		Grid:      w.Grid,
		Status:    w.Status,
		CreatedAt: w.CreatedAt,
		OpenedAt:  worldTime(w.OpenedAt),
		EndedAt:   worldTime(w.EndedAt),
	}
}

// worldTime is nil for the zero time.
func worldTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}

	return &t
}
