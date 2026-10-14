package grpc

import (
	"context"

	"github.com/google/uuid"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type StickerService interface {
	Catalog(ctx context.Context, userID uuid.UUID) ([]service.StickerPackView, error)
	Add(ctx context.Context, userID uuid.UUID, pack user.StickerPackID) error
	Remove(ctx context.Context, userID uuid.UUID, pack user.StickerPackID) error
}

func (h *UserHandler) GetStickerCatalog(ctx context.Context, _ *userv1.GetStickerCatalogRequest) (*userv1.GetStickerCatalogResponse, error) {
	views, err := h.stickerService.Catalog(ctx, viewerID(ctx))
	if err != nil {
		return nil, err
	}

	resp := &userv1.GetStickerCatalogResponse{Packs: make([]*userv1.StickerPack, 0, len(views))}
	for _, v := range views {
		stickers := make([]string, 0, len(v.Pack.Stickers))
		for _, s := range v.Pack.Stickers {
			stickers = append(stickers, s.String())
		}

		resp.Packs = append(resp.Packs, &userv1.StickerPack{
			Id:       v.Pack.ID.String(),
			Price:    v.Pack.Price,
			Stickers: stickers,
			Bought:   v.Bought,
			Added:    v.Added,
		})
	}

	return resp, nil
}

func (h *UserHandler) AddStickerPack(ctx context.Context, req *userv1.AddStickerPackRequest) (*emptypb.Empty, error) {
	userID, pack, err := callerAndPack(ctx, req.GetPackId())
	if err != nil {
		return nil, err
	}

	if err = h.stickerService.Add(ctx, userID, pack); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *UserHandler) RemoveStickerPack(ctx context.Context, req *userv1.RemoveStickerPackRequest) (*emptypb.Empty, error) {
	userID, pack, err := callerAndPack(ctx, req.GetPackId())
	if err != nil {
		return nil, err
	}

	if err = h.stickerService.Remove(ctx, userID, pack); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

// callerAndPack are the caller and the pack of the catalogue they name.
func callerAndPack(ctx context.Context, packID string) (uuid.UUID, user.StickerPackID, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return uuid.Nil, "", err
	}

	pack, err := user.NewStickerPackID(packID)
	if err != nil {
		return uuid.Nil, "", status.Error(codes.InvalidArgument, err.Error())
	}

	return userID, pack, nil
}
