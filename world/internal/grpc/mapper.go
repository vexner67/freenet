package grpc

import (
	"time"

	"github.com/google/uuid"
	worldv1 "github.com/vexner67/freenet/world/api/world/v1"
	"github.com/vexner67/freenet/world/internal/service"
	"github.com/vexner67/freenet/world/internal/world"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func toProtoTime(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}

	return timestamppb.New(t)
}

func toProtoID(id uuid.UUID) string {
	if id == uuid.Nil {
		return ""
	}

	return id.String()
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

func toProtoRules(r world.Rules) *worldv1.WorldRules {
	return &worldv1.WorldRules{
		FoundCost: r.FoundCost, FoundTreasury: r.FoundTreasury, FoundLands: int32(r.FoundLands),
		CapitalDistance: int32(r.CapitalDistance), ProtectionDays: int32(r.ProtectionDays),
		ColonizeCost: r.ColonizeCost, ColonizeHours: int32(r.ColonizeHours), Income: r.Income,
	}
}

func fromProtoRules(r *worldv1.WorldRules) world.Rules {
	return world.Rules{
		FoundCost: r.GetFoundCost(), FoundTreasury: r.GetFoundTreasury(), FoundLands: int(r.GetFoundLands()),
		CapitalDistance: int(r.GetCapitalDistance()), ProtectionDays: int(r.GetProtectionDays()),
		ColonizeCost: r.GetColonizeCost(), ColonizeHours: int(r.GetColonizeHours()), Income: r.GetIncome(),
	}
}

func toProtoFlag(f world.Flag) *worldv1.Flag {
	p := f.Parts()
	colors := make([]int32, len(p.Colors))
	for i, c := range p.Colors {
		colors[i] = int32(c)
	}

	return &worldv1.Flag{
		Pattern: string(p.Pattern), Colors: colors, Emblem: string(p.Emblem), EmblemColor: int32(p.EmblemColor),
		EmblemAt: string(p.EmblemAt),
	}
}

func fromProtoFlag(f *worldv1.Flag) (world.Flag, error) {
	colors := make([]int, len(f.GetColors()))
	for i, c := range f.GetColors() {
		colors[i] = int(c)
	}

	return world.NewFlag(world.FlagParts{
		Pattern: world.Pattern(f.GetPattern()), Colors: colors, Emblem: world.Emblem(f.GetEmblem()),
		EmblemColor: int(f.GetEmblemColor()), EmblemAt: world.EmblemPlace(f.GetEmblemAt()),
	})
}

func toProtoState(s world.Standing) *worldv1.State {
	st := s.State

	return &worldv1.State{
		Id: st.ID.String(), Name: st.Name.String(), Adjective: st.Adjective.String(), Motto: st.Motto.String(),
		Color: int32(st.Color), Flag: toProtoFlag(st.Flag), Government: string(st.Government), RulerId: st.RulerID.String(),
		Capital: int32(st.Capital), Treasury: st.Treasury, FoundedAt: toProtoTime(st.FoundedAt),
		ProtectedUntil: toProtoTime(st.ProtectedUntil), ClubId: toProtoID(st.ClubID), Lands: int32(s.Lands),
		Population: int32(s.Population), Citizens: int32(s.Citizens), Income: s.Income,
	}
}

func toProtoColonies(cs []world.Colony) []*worldv1.Colony {
	out := make([]*worldv1.Colony, len(cs))
	for i, c := range cs {
		out[i] = &worldv1.Colony{Province: int32(c.Province), StateId: c.StateID.String(), Until: toProtoTime(c.Until)}
	}

	return out
}

func toProtoViewer(v service.Viewer) *worldv1.Standing {
	s := &worldv1.Standing{}
	if v.IsCitizen {
		s.StateId, s.Role = v.Citizen.StateID.String(), string(v.Citizen.Role)
	}
	if v.HasApplication {
		s.AppliedTo = v.Application.StateID.String()
	}

	return s
}

func toProtoOverview(o service.Overview) *worldv1.GetWorldResponse {
	resp := &worldv1.GetWorldResponse{
		World: toProtoWorld(o.World), Rules: toProtoRules(o.Rules), Hour: toProtoTime(o.Hour),
		Owners: make([]int32, len(o.Owners)), Colonies: toProtoColonies(o.Colonies), Viewer: toProtoViewer(o.Viewer),
	}

	index := make(map[uuid.UUID]int32, len(o.Standings))
	for i, s := range o.Standings {
		resp.States = append(resp.States, toProtoState(s))
		index[s.State.ID] = int32(i + 1)
	}
	for p, id := range o.Owners {
		resp.Owners[p] = index[id]
	}

	return resp
}

func toProtoAtlas(a *world.Atlas) *worldv1.GetAtlasResponse {
	resp := &worldv1.GetAtlasResponse{Width: int32(a.Width), Height: int32(a.Height), Provinces: make([]*worldv1.Province, len(a.Provinces))}
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

func toProtoStateView(v service.StateView) *worldv1.GetStateResponse {
	resp := &worldv1.GetStateResponse{
		State: toProtoState(v.Standing), Colonies: toProtoColonies(v.Colonies), ColonyCost: v.ColonyCost,
		Viewer: toProtoViewer(v.Viewer),
	}
	for _, p := range v.Lands {
		resp.Lands = append(resp.Lands, int32(p))
	}
	for _, c := range v.Citizens {
		resp.Citizens = append(resp.Citizens, &worldv1.Citizen{UserId: c.UserID.String(), Role: string(c.Role), JoinedAt: toProtoTime(c.JoinedAt)})
	}
	for _, a := range v.Applications {
		resp.Applications = append(resp.Applications, &worldv1.Application{UserId: a.UserID.String(), At: toProtoTime(a.At)})
	}
	for _, h := range v.Standing.State.Budget {
		resp.Budget = append(resp.Budget, &worldv1.BudgetHour{
			Hour: toProtoTime(h.Hour), Income: h.Income, Donations: h.Donations, Founding: h.Founding, Colonization: h.Colonization,
		})
	}

	return resp
}

func toProtoEvent(e world.Event) *worldv1.WorldEvent {
	return &worldv1.WorldEvent{
		Id: e.ID.String(), Kind: string(e.Kind), StateId: e.StateID.String(), UserId: toProtoID(e.UserID),
		Province: int32(e.Province), Name: e.Name, Role: string(e.Role), At: toProtoTime(e.At),
	}
}
