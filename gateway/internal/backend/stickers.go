package backend

import (
	"context"

	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/pkg/errs"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
)

// Stickers is rest.Stickers over the user service, which keeps the packs
// people take.
type Stickers struct {
	api userv1.UserServiceClient
}

func NewStickers(api userv1.UserServiceClient) *Stickers {
	return &Stickers{
		api: api,
	}
}

func (s *Stickers) Catalog(ctx context.Context, accessToken string) ([]model.StickerPack, error) {
	resp, err := s.api.GetStickerCatalog(withToken(ctx, accessToken), &userv1.GetStickerCatalogRequest{})
	if err != nil {
		return nil, errs.Wrap("s.api.GetStickerCatalog", fromStatus(err))
	}

	packs := make([]model.StickerPack, 0, len(resp.GetPacks()))
	for _, p := range resp.GetPacks() {
		packs = append(packs, model.StickerPack{
			ID:       p.GetId(),
			Price:    p.GetPrice(),
			Stickers: p.GetStickers(),
			Bought:   p.GetBought(),
			Added:    p.GetAdded(),
		})
	}

	return packs, nil
}

func (s *Stickers) Add(ctx context.Context, accessToken, packID string) error {
	_, err := s.api.AddStickerPack(withToken(ctx, accessToken), &userv1.AddStickerPackRequest{PackId: packID})

	return errs.Wrap("s.api.AddStickerPack", fromStatus(err))
}

func (s *Stickers) Remove(ctx context.Context, accessToken, packID string) error {
	_, err := s.api.RemoveStickerPack(withToken(ctx, accessToken), &userv1.RemoveStickerPackRequest{PackId: packID})

	return errs.Wrap("s.api.RemoveStickerPack", fromStatus(err))
}
