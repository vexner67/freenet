package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/world/internal/service"
	"github.com/vexner67/freenet/world/internal/world"
)

// MapRepository keeps the maps of the worlds (world_maps, world_tiles).
type MapRepository struct {
	pool *pgxpool.Pool
}

func NewMapRepository(pool *pgxpool.Pool) *MapRepository {
	return &MapRepository{
		pool: pool,
	}
}

func (r *MapRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

type storedLink struct {
	To    int  `json:"to"`
	River bool `json:"river,omitempty"`
	Sea   bool `json:"sea,omitempty"`
}

type storedProvince struct {
	ID         int          `json:"id"`
	Sea        bool         `json:"sea,omitempty"`
	Name       string       `json:"name"`
	Terrain    string       `json:"terrain"`
	Biome      string       `json:"biome"`
	Resources  []string     `json:"resources,omitempty"`
	CenterX    int          `json:"x"`
	CenterY    int          `json:"y"`
	Area       int          `json:"area"`
	Coastal    bool         `json:"coastal,omitempty"`
	Culture    int          `json:"culture"`
	Population int          `json:"population"`
	FreeCity   bool         `json:"freeCity,omitempty"`
	Links      []storedLink `json:"links"`
}

type storedAtlas struct {
	Width     int              `json:"width"`
	Height    int              `json:"height"`
	Provinces []storedProvince `json:"provinces"`
}

func storeAtlas(a *world.Atlas) storedAtlas {
	s := storedAtlas{Width: a.Width, Height: a.Height, Provinces: make([]storedProvince, len(a.Provinces))}
	for i, p := range a.Provinces {
		sp := storedProvince{
			ID: p.ID, Sea: p.Sea, Name: p.Name, Terrain: string(p.Terrain), Biome: p.Biome, CenterX: p.CenterX,
			CenterY: p.CenterY, Area: p.Area, Coastal: p.Coastal, Culture: p.Culture, Population: p.Population,
			FreeCity: p.FreeCity, Links: make([]storedLink, len(p.Links)),
		}
		for _, res := range p.Resources {
			sp.Resources = append(sp.Resources, string(res))
		}
		for k, l := range p.Links {
			sp.Links[k] = storedLink(l)
		}
		s.Provinces[i] = sp
	}

	return s
}

func restoreAtlas(s storedAtlas) *world.Atlas {
	a := &world.Atlas{Width: s.Width, Height: s.Height, Provinces: make([]world.Province, len(s.Provinces))}
	for i, sp := range s.Provinces {
		p := world.Province{
			ID: sp.ID, Sea: sp.Sea, Name: sp.Name, Terrain: world.Terrain(sp.Terrain), Biome: sp.Biome, CenterX: sp.CenterX,
			CenterY: sp.CenterY, Area: sp.Area, Coastal: sp.Coastal, Culture: sp.Culture, Population: sp.Population,
			FreeCity: sp.FreeCity, Links: make([]world.Link, len(sp.Links)),
		}
		for _, res := range sp.Resources {
			p.Resources = append(p.Resources, world.Resource(res))
		}
		for k, l := range sp.Links {
			p.Links[k] = world.Link(l)
		}
		a.Provinces[i] = p
	}

	return a
}

const (
	saveMapQuery = `
		INSERT INTO world_maps (
			world_id,
			atlas,
			raster
		)
		VALUES ($1, $2, $3)
		ON CONFLICT (world_id) DO UPDATE SET
			atlas = EXCLUDED.atlas,
			raster = EXCLUDED.raster
	`
	deleteTilesQuery = `DELETE FROM world_tiles WHERE world_id = $1`
	saveTilesQuery   = `
		INSERT INTO world_tiles (
			world_id,
			z,
			x,
			y,
			png
		)
		SELECT $1, z, x, y, png
		FROM unnest($2::smallint[], $3::smallint[], $4::smallint[], $5::bytea[]) AS t (z, x, y, png)
	`
)

// SaveMap has to run in a transaction (TxManager): a map is all of it or
// nothing.
func (r *MapRepository) SaveMap(ctx context.Context, worldID uuid.UUID, m service.MadeMap) error {
	atlas, err := json.Marshal(storeAtlas(m.Atlas))
	if err != nil {
		return errs.Wrap("json.Marshal", err)
	}

	if _, err = r.conn(ctx).Exec(ctx, saveMapQuery, worldID, atlas, m.Raster); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	if _, err = r.conn(ctx).Exec(ctx, deleteTilesQuery, worldID); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	zs, xs, ys := make([]int16, len(m.Tiles)), make([]int16, len(m.Tiles)), make([]int16, len(m.Tiles))
	pngs := make([][]byte, len(m.Tiles))
	for i, t := range m.Tiles {
		zs[i], xs[i], ys[i], pngs[i] = int16(t.Z), int16(t.X), int16(t.Y), t.PNG
	}

	_, err = r.conn(ctx).Exec(ctx, saveTilesQuery, worldID, zs, xs, ys, pngs)

	return errs.Wrap("r.conn.Exec", err)
}

// The zooms are counted by the tiles, so that the maps made before the
// atlas said them have them too.
const atlasQuery = `
SELECT
    m.atlas,
    (SELECT COALESCE(MAX(t.z) + 1, 0) FROM world_tiles t WHERE t.world_id = m.world_id)
FROM world_maps m
WHERE m.world_id = $1`

func (r *MapRepository) Atlas(ctx context.Context, worldID uuid.UUID) (*world.Atlas, error) {
	var (
		s     storedAtlas
		zooms int
	)
	err := r.conn(ctx).QueryRow(ctx, atlasQuery, worldID).Scan(&s, &zooms)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, world.ErrNotFound
	}
	if err != nil {
		return nil, errs.Wrap("row.Scan", err)
	}

	a := restoreAtlas(s)
	a.Zooms = zooms

	return a, nil
}

const rasterQuery = `SELECT raster FROM world_maps WHERE world_id = $1`

func (r *MapRepository) Raster(ctx context.Context, worldID uuid.UUID) ([]byte, error) {
	var raster []byte
	err := r.conn(ctx).QueryRow(ctx, rasterQuery, worldID).Scan(&raster)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, world.ErrNotFound
	}

	return raster, errs.Wrap("row.Scan", err)
}

const tileQuery = `SELECT png FROM world_tiles WHERE world_id = $1 AND z = $2 AND x = $3 AND y = $4`

func (r *MapRepository) Tile(ctx context.Context, worldID uuid.UUID, z, x, y int) ([]byte, error) {
	if z < 0 || x < 0 || y < 0 || z > 100 || x > 30000 || y > 30000 {
		return nil, world.ErrTileNotFound
	}

	var data []byte
	err := r.conn(ctx).QueryRow(ctx, tileQuery, worldID, z, x, y).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, world.ErrTileNotFound
	}

	return data, errs.Wrap("row.Scan", err)
}
