// Package grpc is the gRPC transport of the world service.
package grpc

import (
	"context"

	"github.com/google/uuid"
	worldv1 "github.com/vexner67/freenet/world/api/world/v1"
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
	Current(ctx context.Context) (*world.World, error)
	Worlds(ctx context.Context) ([]*world.World, error)
	Atlas(ctx context.Context, id uuid.UUID) (*world.Atlas, error)
	Raster(ctx context.Context, id uuid.UUID) ([]byte, error)
	Tile(ctx context.Context, id uuid.UUID, z, x, y int) ([]byte, error)
}

type WorldHandler struct {
	worldv1.UnimplementedWorldServiceServer

	worlds WorldService
}

func NewWorldHandler(worlds WorldService) *WorldHandler {
	return &WorldHandler{worlds: worlds}
}

func (h *WorldHandler) GetWorld(ctx context.Context, _ *worldv1.GetWorldRequest) (*worldv1.GetWorldResponse, error) {
	w, err := h.worlds.Current(ctx)
	if err != nil {
		return nil, err
	}

	return &worldv1.GetWorldResponse{World: toProtoWorld(w)}, nil
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
	if err := h.worlds.End(ctx); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func parseID(s, invalid string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, status.Error(codes.InvalidArgument, invalid)
	}

	return id, nil
}
