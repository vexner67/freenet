package backend

import (
	"context"

	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/pkg/errs"
	worldv1 "github.com/vexner67/freenet/world/api/world/v1"
	"google.golang.org/protobuf/types/known/emptypb"
)

// World is "Мир", which the world service keeps: its map, its states and
// their news.
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

func (w *World) Map(ctx context.Context, accessToken string) (model.WorldMap, error) {
	resp, err := w.api.GetWorld(withToken(ctx, accessToken), &worldv1.GetWorldRequest{})
	if err != nil {
		return model.WorldMap{}, errs.Wrap("w.api.GetWorld", fromStatus(err))
	}

	m := model.WorldMap{
		World:    fromProtoWorld(resp.GetWorld()),
		Rules:    fromProtoRules(resp.GetRules()),
		Hour:     fromTimestamp(resp.GetHour()),
		Owners:   make([]int, len(resp.GetOwners())),
		Colonies: fromProtoColonies(resp.GetColonies()),
		Viewer:   fromProtoStanding(resp.GetViewer()),
	}
	for _, s := range resp.GetStates() {
		m.States = append(m.States, fromProtoState(s))
	}
	for i, o := range resp.GetOwners() {
		m.Owners[i] = int(o)
	}

	return m, nil
}

func (w *World) Atlas(ctx context.Context, worldID string) (model.Atlas, error) {
	resp, err := w.api.GetAtlas(ctx, &worldv1.GetAtlasRequest{WorldId: worldID})
	if err != nil {
		return model.Atlas{}, errs.Wrap("w.api.GetAtlas", fromStatus(err))
	}

	a := model.Atlas{Width: int(resp.GetWidth()), Height: int(resp.GetHeight())}
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

func (w *World) State(ctx context.Context, accessToken, stateID string) (model.StateView, error) {
	resp, err := w.api.GetState(withToken(ctx, accessToken), &worldv1.GetStateRequest{StateId: stateID})
	if err != nil {
		return model.StateView{}, errs.Wrap("w.api.GetState", fromStatus(err))
	}

	v := model.StateView{
		State:      fromProtoState(resp.GetState()),
		Colonies:   fromProtoColonies(resp.GetColonies()),
		ColonyCost: resp.GetColonyCost(),
		Viewer:     fromProtoStanding(resp.GetViewer()),
	}
	for _, l := range resp.GetLands() {
		v.Lands = append(v.Lands, int(l))
	}
	for _, c := range resp.GetCitizens() {
		v.Citizens = append(v.Citizens, model.Citizen{UserID: c.GetUserId(), Role: c.GetRole(), JoinedAt: fromTimestamp(c.GetJoinedAt())})
	}
	for _, a := range resp.GetApplications() {
		v.Applications = append(v.Applications, model.CitizenshipApplication{UserID: a.GetUserId(), At: fromTimestamp(a.GetAt())})
	}
	for _, b := range resp.GetBudget() {
		v.Budget = append(v.Budget, model.BudgetHour{
			Hour:         fromTimestamp(b.GetHour()),
			Income:       b.GetIncome(),
			Donations:    b.GetDonations(),
			Founding:     b.GetFounding(),
			Colonization: b.GetColonization(),
		})
	}

	return v, nil
}

func (w *World) Events(ctx context.Context, stateID, before string, limit int) ([]model.WorldEvent, error) {
	resp, err := w.api.ListWorldEvents(ctx, &worldv1.ListWorldEventsRequest{
		StateId: stateID,
		Before:  before,
		Limit:   int32(limit), //nolint:gosec // the page size is checked
	})
	if err != nil {
		return nil, errs.Wrap("w.api.ListWorldEvents", fromStatus(err))
	}

	events := make([]model.WorldEvent, 0, len(resp.GetEvents()))
	for _, e := range resp.GetEvents() {
		events = append(events, model.WorldEvent{
			ID:       e.GetId(),
			Kind:     e.GetKind(),
			StateID:  e.GetStateId(),
			UserID:   e.GetUserId(),
			Province: int(e.GetProvince()),
			Name:     e.GetName(),
			Role:     e.GetRole(),
			At:       fromTimestamp(e.GetAt()),
		})
	}

	return events, nil
}

// Citizenship is the state of a user and their office there; no state
// is a zero State.
func (w *World) Citizenship(ctx context.Context, userID string) (model.State, string, error) {
	resp, err := w.api.GetCitizenship(ctx, &worldv1.GetCitizenshipRequest{UserId: userID})
	if err != nil {
		return model.State{}, "", errs.Wrap("w.api.GetCitizenship", fromStatus(err))
	}

	if resp.GetState() == nil {
		return model.State{}, "", nil
	}

	return fromProtoState(resp.GetState()), resp.GetRole(), nil
}

func (w *World) Found(ctx context.Context, accessToken string, f model.Founding) (string, error) {
	resp, err := w.api.FoundState(withToken(ctx, accessToken), &worldv1.FoundStateRequest{
		Name:       f.Name,
		Adjective:  f.Adjective,
		Motto:      f.Motto,
		Color:      int32(f.Color), //nolint:gosec // the world service checks the color
		Flag:       toProtoFlag(f.Flag),
		Government: f.Government,
		Capital:    int32(f.Capital), //nolint:gosec // the world service checks the province
	})
	if err != nil {
		return "", errs.Wrap("w.api.FoundState", fromStatus(err))
	}

	return resp.GetStateId(), nil
}

func (w *World) Edit(ctx context.Context, accessToken string, l model.StateLook) error {
	_, err := w.api.EditState(withToken(ctx, accessToken), &worldv1.EditStateRequest{
		Adjective: l.Adjective,
		Motto:     l.Motto,
		Color:     int32(l.Color), //nolint:gosec // the world service checks the color
		Flag:      toProtoFlag(l.Flag),
	})

	return errs.Wrap("w.api.EditState", fromStatus(err))
}

func (w *World) Apply(ctx context.Context, accessToken, stateID string) error {
	_, err := w.api.ApplyCitizenship(withToken(ctx, accessToken), &worldv1.ApplyCitizenshipRequest{StateId: stateID})
	return errs.Wrap("w.api.ApplyCitizenship", fromStatus(err))
}

func (w *World) Withdraw(ctx context.Context, accessToken string) error {
	_, err := w.api.WithdrawApplication(withToken(ctx, accessToken), &emptypb.Empty{})
	return errs.Wrap("w.api.WithdrawApplication", fromStatus(err))
}

func (w *World) Answer(ctx context.Context, accessToken, userID string, accept bool) error {
	_, err := w.api.AnswerApplication(withToken(ctx, accessToken), &worldv1.AnswerApplicationRequest{UserId: userID, Accept: accept})
	return errs.Wrap("w.api.AnswerApplication", fromStatus(err))
}

func (w *World) Leave(ctx context.Context, accessToken string) error {
	_, err := w.api.LeaveState(withToken(ctx, accessToken), &emptypb.Empty{})
	return errs.Wrap("w.api.LeaveState", fromStatus(err))
}

func (w *World) Expel(ctx context.Context, accessToken, userID string) error {
	_, err := w.api.ExpelCitizen(withToken(ctx, accessToken), &worldv1.ExpelCitizenRequest{UserId: userID})
	return errs.Wrap("w.api.ExpelCitizen", fromStatus(err))
}

func (w *World) Appoint(ctx context.Context, accessToken, userID, role string) error {
	_, err := w.api.AppointCitizen(withToken(ctx, accessToken), &worldv1.AppointCitizenRequest{UserId: userID, Role: role})
	return errs.Wrap("w.api.AppointCitizen", fromStatus(err))
}

func (w *World) Abdicate(ctx context.Context, accessToken, heirID string) error {
	_, err := w.api.Abdicate(withToken(ctx, accessToken), &worldv1.AbdicateRequest{HeirId: heirID})
	return errs.Wrap("w.api.Abdicate", fromStatus(err))
}

func (w *World) Colonize(ctx context.Context, accessToken string, province int) error {
	_, err := w.api.Colonize(withToken(ctx, accessToken), &worldv1.ColonizeRequest{
		Province: int32(province), //nolint:gosec // the world service checks the province
	})

	return errs.Wrap("w.api.Colonize", fromStatus(err))
}

func (w *World) Donate(ctx context.Context, accessToken string, votes int64) error {
	_, err := w.api.Donate(withToken(ctx, accessToken), &worldv1.DonateRequest{Votes: votes})
	return errs.Wrap("w.api.Donate", fromStatus(err))
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

func (w *World) Rules(ctx context.Context) (model.WorldRules, error) {
	resp, err := w.api.GetWorldRules(ctx, &emptypb.Empty{})
	if err != nil {
		return model.WorldRules{}, errs.Wrap("w.api.GetWorldRules", fromStatus(err))
	}

	return fromProtoRules(resp), nil
}

func (w *World) SetRules(ctx context.Context, r model.WorldRules) error {
	_, err := w.api.SetWorldRules(ctx, &worldv1.WorldRules{
		FoundCost:       r.FoundCost,
		FoundTreasury:   r.FoundTreasury,
		FoundLands:      int32(r.FoundLands),      //nolint:gosec // the world service checks the rules
		CapitalDistance: int32(r.CapitalDistance), //nolint:gosec // the world service checks the rules
		ProtectionDays:  int32(r.ProtectionDays),  //nolint:gosec // the world service checks the rules
		ColonizeCost:    r.ColonizeCost,
		ColonizeHours:   int32(r.ColonizeHours), //nolint:gosec // the world service checks the rules
		Income:          r.Income,
	})

	return errs.Wrap("w.api.SetWorldRules", fromStatus(err))
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

func fromProtoRules(r *worldv1.WorldRules) model.WorldRules {
	return model.WorldRules{
		FoundCost:       r.GetFoundCost(),
		FoundTreasury:   r.GetFoundTreasury(),
		FoundLands:      int(r.GetFoundLands()),
		CapitalDistance: int(r.GetCapitalDistance()),
		ProtectionDays:  int(r.GetProtectionDays()),
		ColonizeCost:    r.GetColonizeCost(),
		ColonizeHours:   int(r.GetColonizeHours()),
		Income:          r.GetIncome(),
	}
}

func fromProtoState(s *worldv1.State) model.State {
	return model.State{
		ID:             s.GetId(),
		Name:           s.GetName(),
		Adjective:      s.GetAdjective(),
		Motto:          s.GetMotto(),
		Color:          int(s.GetColor()),
		Flag:           fromProtoFlag(s.GetFlag()),
		Government:     s.GetGovernment(),
		RulerID:        s.GetRulerId(),
		Capital:        int(s.GetCapital()),
		Treasury:       s.GetTreasury(),
		FoundedAt:      fromTimestamp(s.GetFoundedAt()),
		ProtectedUntil: fromTimestamp(s.GetProtectedUntil()),
		ClubID:         s.GetClubId(),
		Lands:          int(s.GetLands()),
		Population:     int(s.GetPopulation()),
		Citizens:       int(s.GetCitizens()),
		Income:         s.GetIncome(),
	}
}

func fromProtoFlag(f *worldv1.Flag) model.Flag {
	flag := model.Flag{
		Pattern:     f.GetPattern(),
		Colors:      make([]int, 0, len(f.GetColors())),
		Emblem:      f.GetEmblem(),
		EmblemColor: int(f.GetEmblemColor()),
		EmblemAt:    f.GetEmblemAt(),
	}
	for _, c := range f.GetColors() {
		flag.Colors = append(flag.Colors, int(c))
	}

	return flag
}

func toProtoFlag(f model.Flag) *worldv1.Flag {
	flag := &worldv1.Flag{
		Pattern:     f.Pattern,
		Emblem:      f.Emblem,
		EmblemColor: int32(f.EmblemColor), //nolint:gosec // the world service checks the colors
		EmblemAt:    f.EmblemAt,
	}
	for _, c := range f.Colors {
		flag.Colors = append(flag.Colors, int32(c)) //nolint:gosec // the world service checks the colors
	}

	return flag
}

func fromProtoColonies(colonies []*worldv1.Colony) []model.Colony {
	out := make([]model.Colony, 0, len(colonies))
	for _, c := range colonies {
		out = append(out, model.Colony{Province: int(c.GetProvince()), StateID: c.GetStateId(), Until: fromTimestamp(c.GetUntil())})
	}

	return out
}

func fromProtoStanding(s *worldv1.Standing) model.Standing {
	return model.Standing{StateID: s.GetStateId(), Role: s.GetRole(), AppliedTo: s.GetAppliedTo()}
}
