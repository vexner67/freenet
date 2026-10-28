// Package grpc is the gRPC transport of the world service.
package grpc

import (
	"context"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/jwtauth"
	worldv1 "github.com/vexner67/freenet/world/api/world/v1"
	"github.com/vexner67/freenet/world/internal/service"
	"github.com/vexner67/freenet/world/internal/world"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// WorldService makes and opens the worlds and reads their maps.
type WorldService interface {
	Create(ctx context.Context, seed uint64, grid int) (*world.World, error)
	Open(ctx context.Context, id uuid.UUID) (*world.World, error)
	End(ctx context.Context) error
	Worlds(ctx context.Context) ([]*world.World, error)
	Rules(ctx context.Context) (world.Rules, error)
	SetRules(ctx context.Context, r world.Rules) error
	Atlas(ctx context.Context, id uuid.UUID) (*world.Atlas, error)
	Raster(ctx context.Context, id uuid.UUID) ([]byte, error)
	Tile(ctx context.Context, id uuid.UUID, z, x, y int) ([]byte, error)
}

// GameService is what people do in the open world.
type GameService interface {
	Overview(ctx context.Context, viewerID uuid.UUID) (service.Overview, error)
	State(ctx context.Context, viewerID, id uuid.UUID) (service.StateView, error)
	Citizenship(ctx context.Context, userID uuid.UUID) (world.Citizen, world.State, bool, error)
	Events(ctx context.Context, stateID, before uuid.UUID, limit int) ([]world.Event, error)
	Found(ctx context.Context, f world.Founding) (uuid.UUID, error)
	Edit(ctx context.Context, userID uuid.UUID, e world.Edit) error
	Apply(ctx context.Context, userID, stateID uuid.UUID) error
	Withdraw(ctx context.Context, userID uuid.UUID) error
	Answer(ctx context.Context, actorID, applicantID uuid.UUID, accept bool) error
	Leave(ctx context.Context, userID uuid.UUID) error
	Expel(ctx context.Context, actorID, targetID uuid.UUID) error
	Appoint(ctx context.Context, actorID, targetID uuid.UUID, role world.Role) error
	Abdicate(ctx context.Context, actorID, heirID uuid.UUID) error
	Colonize(ctx context.Context, actorID uuid.UUID, province int) error
	Donate(ctx context.Context, userID uuid.UUID, votes int64) error
	PurgeUser(ctx context.Context, userID uuid.UUID) error
}

type WorldHandler struct {
	worldv1.UnimplementedWorldServiceServer

	worlds WorldService
	games  GameService
}

func NewWorldHandler(worlds WorldService, games GameService) *WorldHandler {
	return &WorldHandler{worlds: worlds, games: games}
}

// defaultEvents is how many news a page has when the client does not say.
const defaultEvents = 20

func (h *WorldHandler) GetWorld(ctx context.Context, _ *worldv1.GetWorldRequest) (*worldv1.GetWorldResponse, error) {
	o, err := h.games.Overview(ctx, viewerID(ctx))
	if err != nil {
		return nil, err
	}

	return toProtoOverview(o), nil
}

func (h *WorldHandler) GetAtlas(ctx context.Context, req *worldv1.GetAtlasRequest) (*worldv1.GetAtlasResponse, error) {
	id, err := parseID(req.GetWorldId(), "invalid world id")
	if err != nil {
		return nil, err
	}

	a, err := h.worlds.Atlas(ctx, id)
	if err != nil {
		return nil, err
	}

	return toProtoAtlas(a), nil
}

func (h *WorldHandler) GetRaster(ctx context.Context, req *worldv1.GetRasterRequest) (*worldv1.GetRasterResponse, error) {
	id, err := parseID(req.GetWorldId(), "invalid world id")
	if err != nil {
		return nil, err
	}

	r, err := h.worlds.Raster(ctx, id)
	if err != nil {
		return nil, err
	}

	return &worldv1.GetRasterResponse{Raster: r}, nil
}

func (h *WorldHandler) GetTile(ctx context.Context, req *worldv1.GetTileRequest) (*worldv1.GetTileResponse, error) {
	id, err := parseID(req.GetWorldId(), "invalid world id")
	if err != nil {
		return nil, err
	}

	png, err := h.worlds.Tile(ctx, id, int(req.GetZ()), int(req.GetX()), int(req.GetY()))
	if err != nil {
		return nil, err
	}

	return &worldv1.GetTileResponse{Png: png}, nil
}

func (h *WorldHandler) GetState(ctx context.Context, req *worldv1.GetStateRequest) (*worldv1.GetStateResponse, error) {
	id, err := parseID(req.GetStateId(), "invalid state id")
	if err != nil {
		return nil, err
	}

	v, err := h.games.State(ctx, viewerID(ctx), id)
	if err != nil {
		return nil, err
	}

	return toProtoStateView(v), nil
}

func (h *WorldHandler) ListWorldEvents(ctx context.Context, req *worldv1.ListWorldEventsRequest) (*worldv1.ListWorldEventsResponse, error) {
	stateID, err := parseOptionalID(req.GetStateId(), "invalid state id")
	if err != nil {
		return nil, err
	}

	before, err := parseOptionalID(req.GetBefore(), "invalid cursor")
	if err != nil {
		return nil, err
	}

	limit := int(req.GetLimit())
	if limit <= 0 {
		limit = defaultEvents
	}

	events, err := h.games.Events(ctx, stateID, before, limit)
	if err != nil {
		return nil, err
	}

	resp := &worldv1.ListWorldEventsResponse{}
	for _, e := range events {
		resp.Events = append(resp.Events, toProtoEvent(e))
	}

	return resp, nil
}

func (h *WorldHandler) GetCitizenship(ctx context.Context, req *worldv1.GetCitizenshipRequest) (*worldv1.GetCitizenshipResponse, error) {
	id, err := parseID(req.GetUserId(), "invalid user id")
	if err != nil {
		return nil, err
	}

	c, st, ok, err := h.games.Citizenship(ctx, id)
	if err != nil || !ok {
		return &worldv1.GetCitizenshipResponse{}, err
	}

	return &worldv1.GetCitizenshipResponse{State: toProtoState(world.Standing{State: st}), Role: string(c.Role)}, nil
}

func (h *WorldHandler) FoundState(ctx context.Context, req *worldv1.FoundStateRequest) (*worldv1.FoundStateResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	name, err := world.NewStateName(req.GetName())
	if err != nil {
		return nil, err
	}

	government, err := world.NewGovernment(req.GetGovernment())
	if err != nil {
		return nil, err
	}

	e, err := fromProtoEdit(req.GetAdjective(), req.GetMotto(), req.GetColor(), req.GetFlag())
	if err != nil {
		return nil, err
	}

	id, err := h.games.Found(ctx, world.Founding{
		RulerID: userID, Name: name, Adjective: e.Adjective, Motto: e.Motto, Color: e.Color, Flag: e.Flag,
		Government: government, Capital: int(req.GetCapital()),
	})
	if err != nil {
		return nil, err
	}

	return &worldv1.FoundStateResponse{StateId: id.String()}, nil
}

func fromProtoEdit(adjective, motto string, color int32, flag *worldv1.Flag) (world.Edit, error) {
	a, err := world.NewAdjective(adjective)
	if err != nil {
		return world.Edit{}, err
	}

	m, err := world.NewMotto(motto)
	if err != nil {
		return world.Edit{}, err
	}

	c, err := world.NewColor(int(color))
	if err != nil {
		return world.Edit{}, err
	}

	f, err := fromProtoFlag(flag)
	if err != nil {
		return world.Edit{}, err
	}

	return world.Edit{Adjective: a, Motto: m, Color: c, Flag: f}, nil
}

func (h *WorldHandler) EditState(ctx context.Context, req *worldv1.EditStateRequest) (*emptypb.Empty, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	e, err := fromProtoEdit(req.GetAdjective(), req.GetMotto(), req.GetColor(), req.GetFlag())
	if err != nil {
		return nil, err
	}

	return empty(h.games.Edit(ctx, userID, e))
}

func (h *WorldHandler) ApplyCitizenship(ctx context.Context, req *worldv1.ApplyCitizenshipRequest) (*emptypb.Empty, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	stateID, err := parseID(req.GetStateId(), "invalid state id")
	if err != nil {
		return nil, err
	}

	return empty(h.games.Apply(ctx, userID, stateID))
}

func (h *WorldHandler) WithdrawApplication(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	return empty(h.games.Withdraw(ctx, userID))
}

func (h *WorldHandler) AnswerApplication(ctx context.Context, req *worldv1.AnswerApplicationRequest) (*emptypb.Empty, error) {
	actorID, targetID, err := actorAndTarget(ctx, req.GetUserId())
	if err != nil {
		return nil, err
	}

	return empty(h.games.Answer(ctx, actorID, targetID, req.GetAccept()))
}

func (h *WorldHandler) LeaveState(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	return empty(h.games.Leave(ctx, userID))
}

func (h *WorldHandler) ExpelCitizen(ctx context.Context, req *worldv1.ExpelCitizenRequest) (*emptypb.Empty, error) {
	actorID, targetID, err := actorAndTarget(ctx, req.GetUserId())
	if err != nil {
		return nil, err
	}

	return empty(h.games.Expel(ctx, actorID, targetID))
}

func (h *WorldHandler) AppointCitizen(ctx context.Context, req *worldv1.AppointCitizenRequest) (*emptypb.Empty, error) {
	actorID, targetID, err := actorAndTarget(ctx, req.GetUserId())
	if err != nil {
		return nil, err
	}

	role, err := world.NewRole(req.GetRole())
	if err != nil {
		return nil, err
	}

	return empty(h.games.Appoint(ctx, actorID, targetID, role))
}

func (h *WorldHandler) Abdicate(ctx context.Context, req *worldv1.AbdicateRequest) (*emptypb.Empty, error) {
	actorID, heirID, err := actorAndTarget(ctx, req.GetHeirId())
	if err != nil {
		return nil, err
	}

	return empty(h.games.Abdicate(ctx, actorID, heirID))
}

func (h *WorldHandler) Colonize(ctx context.Context, req *worldv1.ColonizeRequest) (*emptypb.Empty, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	return empty(h.games.Colonize(ctx, userID, int(req.GetProvince())))
}

func (h *WorldHandler) Donate(ctx context.Context, req *worldv1.DonateRequest) (*emptypb.Empty, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	return empty(h.games.Donate(ctx, userID, req.GetVotes()))
}

func (h *WorldHandler) PurgeUser(ctx context.Context, req *worldv1.PurgeUserRequest) (*emptypb.Empty, error) {
	userID, err := parseID(req.GetUserId(), "invalid user id")
	if err != nil {
		return nil, err
	}

	return empty(h.games.PurgeUser(ctx, userID))
}

func (h *WorldHandler) ListWorlds(ctx context.Context, _ *emptypb.Empty) (*worldv1.ListWorldsResponse, error) {
	ws, err := h.worlds.Worlds(ctx)
	if err != nil {
		return nil, err
	}

	resp := &worldv1.ListWorldsResponse{}
	for _, w := range ws {
		resp.Worlds = append(resp.Worlds, toProtoWorld(w))
	}

	return resp, nil
}

func (h *WorldHandler) CreateWorld(ctx context.Context, req *worldv1.CreateWorldRequest) (*worldv1.CreateWorldResponse, error) {
	w, err := h.worlds.Create(ctx, req.GetSeed(), int(req.GetGrid()))
	if err != nil {
		return nil, err
	}

	return &worldv1.CreateWorldResponse{World: toProtoWorld(w)}, nil
}

func (h *WorldHandler) OpenWorld(ctx context.Context, req *worldv1.OpenWorldRequest) (*emptypb.Empty, error) {
	id, err := parseID(req.GetWorldId(), "invalid world id")
	if err != nil {
		return nil, err
	}

	if _, err = h.worlds.Open(ctx, id); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *WorldHandler) EndWorld(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	return empty(h.worlds.End(ctx))
}

func (h *WorldHandler) GetWorldRules(ctx context.Context, _ *emptypb.Empty) (*worldv1.WorldRules, error) {
	r, err := h.worlds.Rules(ctx)
	if err != nil {
		return nil, err
	}

	return toProtoRules(r), nil
}

func (h *WorldHandler) SetWorldRules(ctx context.Context, req *worldv1.WorldRules) (*emptypb.Empty, error) {
	return empty(h.worlds.SetRules(ctx, fromProtoRules(req)))
}

// empty answers nothing, or the error.
func empty(err error) (*emptypb.Empty, error) {
	if err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

// actorAndTarget are the caller and the person they act on.
func actorAndTarget(ctx context.Context, target string) (uuid.UUID, uuid.UUID, error) {
	actorID, err := currentUserID(ctx)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	targetID, err := parseID(target, "invalid user id")
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	return actorID, targetID, nil
}

func parseID(s, invalid string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, status.Error(codes.InvalidArgument, invalid)
	}

	return id, nil
}

// parseOptionalID reads an ID that may be left out; "" is uuid.Nil.
func parseOptionalID(s, invalid string) (uuid.UUID, error) {
	if s == "" {
		return uuid.Nil, nil
	}

	return parseID(s, invalid)
}

// viewerID is the caller, uuid.Nil for a guest.
func viewerID(ctx context.Context) uuid.UUID {
	identity, ok := jwtauth.IdentityFromContext(ctx)
	if !ok {
		return uuid.Nil
	}

	return identity.UserID
}

// currentUserID is the caller's ID from the access token verified by
// jwtauth.UnaryServerInterceptor; never trust an ID from the request body.
func currentUserID(ctx context.Context) (uuid.UUID, error) {
	identity, ok := jwtauth.IdentityFromContext(ctx)
	if !ok {
		return uuid.Nil, status.Error(codes.Unauthenticated, "missing access token")
	}

	return identity.UserID, nil
}
