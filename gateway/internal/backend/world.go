package backend

import (
	"context"

	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/pkg/errs"
	worldv1 "github.com/vexner67/freenet/world/api/world/v1"
	"google.golang.org/protobuf/types/known/emptypb"
)

// World is "Мир", which the world service keeps: the worlds and their
// maps.
type World struct {
	api worldv1.WorldServiceClient
}

func NewWorld(api worldv1.WorldServiceClient) *World {
	return &World{
		api: api,
	}
}

var worldStatuses = map[worldv1.WorldStatus]string{
	worldv1.WorldStatus_WORLD_STATUS_GENERATING: "generating",
	worldv1.WorldStatus_WORLD_STATUS_READY:      "ready",
	worldv1.WorldStatus_WORLD_STATUS_OPEN:       "open",
	worldv1.WorldStatus_WORLD_STATUS_ENDED:      "ended",
	worldv1.WorldStatus_WORLD_STATUS_FAILED:     "failed",
}

// Current is the world open now.
func (w *World) Current(ctx context.Context) (model.World, error) {
	resp, err := w.api.GetWorld(ctx, &worldv1.GetWorldRequest{})
	if err != nil {
		return model.World{}, errs.Wrap("w.api.GetWorld", fromStatus(err))
	}

	return fromProtoWorld(resp.GetWorld()), nil
}

func (w *World) Atlas(ctx context.Context, worldID string) (model.Atlas, error) {
	resp, err := w.api.GetAtlas(ctx, &worldv1.GetAtlasRequest{WorldId: worldID})
	if err != nil {
		return model.Atlas{}, errs.Wrap("w.api.GetAtlas", fromStatus(err))
	}

	a := model.Atlas{Width: int(resp.GetWidth()), Height: int(resp.GetHeight()), Zooms: int(resp.GetZooms())}
	for _, p := range resp.GetProvinces() {
		province := model.Province{
			ID:         int(p.GetId()),
			Sea:        p.GetSea(),
			Name:       p.GetName(),
			Terrain:    p.GetTerrain(),
			Biome:      p.GetBiome(),
			Resources:  p.GetResources(),
			CenterX:    int(p.GetCenterX()),
			CenterY:    int(p.GetCenterY()),
			Area:       int(p.GetArea()),
			Coastal:    p.GetCoastal(),
			Culture:    int(p.GetCulture()),
			Population: int(p.GetPopulation()),
			FreeCity:   p.GetFreeCity(),
		}
		for _, l := range p.GetLinks() {
			province.Links = append(province.Links, model.ProvinceLink{To: int(l.GetTo()), River: l.GetRiver(), Sea: l.GetSea()})
		}
		a.Provinces = append(a.Provinces, province)
	}

	return a, nil
}

func (w *World) Raster(ctx context.Context, worldID string) ([]byte, error) {
	resp, err := w.api.GetRaster(ctx, &worldv1.GetRasterRequest{WorldId: worldID})
	if err != nil {
		return nil, errs.Wrap("w.api.GetRaster", fromStatus(err))
	}

	return resp.GetRaster(), nil
}

func (w *World) Tile(ctx context.Context, worldID string, z, x, y int) ([]byte, error) {
	resp, err := w.api.GetTile(ctx, &worldv1.GetTileRequest{
		WorldId: worldID,
		Z:       int32(z), //nolint:gosec // the zoom and the place are checked
		X:       int32(x), //nolint:gosec // the zoom and the place are checked
		Y:       int32(y), //nolint:gosec // the zoom and the place are checked
	})
	if err != nil {
		return nil, errs.Wrap("w.api.GetTile", fromStatus(err))
	}

	return resp.GetPng(), nil
}

// The calls below are the administrators' of the site: the world service
// takes them from the gateway alone, which asks the user service whether
// the caller runs the site first.

func (w *World) Worlds(ctx context.Context) ([]model.World, error) {
	resp, err := w.api.ListWorlds(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, errs.Wrap("w.api.ListWorlds", fromStatus(err))
	}

	worlds := make([]model.World, 0, len(resp.GetWorlds()))
	for _, world := range resp.GetWorlds() {
		worlds = append(worlds, fromProtoWorld(world))
	}

	return worlds, nil
}

func (w *World) Create(ctx context.Context, seed uint64, grid int) (model.World, error) {
	resp, err := w.api.CreateWorld(ctx, &worldv1.CreateWorldRequest{
		Seed: seed,
		Grid: int32(grid), //nolint:gosec // the world service checks the grid
	})
	if err != nil {
		return model.World{}, errs.Wrap("w.api.CreateWorld", fromStatus(err))
	}

	return fromProtoWorld(resp.GetWorld()), nil
}

func (w *World) Open(ctx context.Context, worldID string) error {
	_, err := w.api.OpenWorld(ctx, &worldv1.OpenWorldRequest{WorldId: worldID})
	return errs.Wrap("w.api.OpenWorld", fromStatus(err))
}

func (w *World) End(ctx context.Context) error {
	_, err := w.api.EndWorld(ctx, &emptypb.Empty{})
	return errs.Wrap("w.api.EndWorld", fromStatus(err))
}

func fromProtoWorld(w *worldv1.World) model.World {
	return model.World{
		ID:        w.GetId(),
		Seed:      w.GetSeed(),
		Grid:      int(w.GetGrid()),
		Status:    worldStatuses[w.GetStatus()],
		CreatedAt: fromTimestamp(w.GetCreatedAt()),
		OpenedAt:  fromTimestamp(w.GetOpenedAt()),
		EndedAt:   fromTimestamp(w.GetEndedAt()),
	}
}
