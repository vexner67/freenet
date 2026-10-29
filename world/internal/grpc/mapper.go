package grpc

import (
	"time"

	worldv1 "github.com/vexner67/freenet/world/api/world/v1"
	"github.com/vexner67/freenet/world/internal/world"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func toProtoTime(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}

	return timestamppb.New(t)
}

var toProtoStatus = map[world.Status]worldv1.WorldStatus{
	world.StatusGenerating: worldv1.WorldStatus_WORLD_STATUS_GENERATING,
	world.StatusReady:      worldv1.WorldStatus_WORLD_STATUS_READY,
	world.StatusOpen:       worldv1.WorldStatus_WORLD_STATUS_OPEN,
	world.StatusEnded:      worldv1.WorldStatus_WORLD_STATUS_ENDED,
	world.StatusFailed:     worldv1.WorldStatus_WORLD_STATUS_FAILED,
}

func toProtoWorld(w *world.World) *worldv1.World {
	return &worldv1.World{
		Id: w.ID().String(), Seed: w.Seed(), Grid: int32(w.Grid()), Status: toProtoStatus[w.Status()],
		CreatedAt: toProtoTime(w.CreatedAt()), OpenedAt: toProtoTime(w.OpenedAt()), EndedAt: toProtoTime(w.EndedAt()),
	}
}

func toProtoAtlas(a *world.Atlas) *worldv1.GetAtlasResponse {
	resp := &worldv1.GetAtlasResponse{Width: int32(a.Width), Height: int32(a.Height), Zooms: int32(a.Zooms), Provinces: make([]*worldv1.Province, len(a.Provinces))}
	for i, p := range a.Provinces {
		pp := &worldv1.Province{
			Id: int32(p.ID), Sea: p.Sea, Name: p.Name, Terrain: string(p.Terrain), Biome: p.Biome,
			CenterX: int32(p.CenterX), CenterY: int32(p.CenterY), Area: int32(p.Area), Coastal: p.Coastal,
			Culture: int32(p.Culture), Population: int32(p.Population), FreeCity: p.FreeCity,
		}
		for _, r := range p.Resources {
			pp.Resources = append(pp.Resources, string(r))
		}
		for _, l := range p.Links {
			pp.Links = append(pp.Links, &worldv1.Link{To: int32(l.To), River: l.River, Sea: l.Sea})
		}
		resp.Provinces[i] = pp
	}

	return resp
}
