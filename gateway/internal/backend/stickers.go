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

	return stickerPacks(resp.GetPacks()), nil
}

// Mine are the packs the caller made, for their "Студия стикеров".
func (s *Stickers) Mine(ctx context.Context, accessToken string) ([]model.StickerPack, error) {
	resp, err := s.api.ListMyStickerPacks(withToken(ctx, accessToken), &userv1.ListMyStickerPacksRequest{})
	if err != nil {
		return nil, errs.Wrap("s.api.ListMyStickerPacks", fromStatus(err))
	}

	return stickerPacks(resp.GetPacks()), nil
}

func (s *Stickers) Create(ctx context.Context, accessToken string, d model.StickerPackDraft) (model.StickerPack, error) {
	resp, err := s.api.CreateStickerPack(withToken(ctx, accessToken), &userv1.CreateStickerPackRequest{
		Name:      d.Name,
		About:     d.About,
		Price:     d.Price,
		Published: d.Published,
		PhotoIds:  d.Added,
	})
	if err != nil {
		return model.StickerPack{}, errs.Wrap("s.api.CreateStickerPack", fromStatus(err))
	}

	return stickerPack(resp.GetPack()), nil
}

// Edit returns the pack and the photos it no longer has.
func (s *Stickers) Edit(ctx context.Context, accessToken, packID string, d model.StickerPackDraft) (model.StickerPack, []string, error) {
	resp, err := s.api.EditStickerPack(withToken(ctx, accessToken), &userv1.EditStickerPackRequest{
		PackId:         packID,
		Name:           d.Name,
		About:          d.About,
		Price:          d.Price,
		Published:      d.Published,
		AddPhotoIds:    d.Added,
		RemovePhotoIds: d.Removed,
	})
	if err != nil {
		return model.StickerPack{}, nil, errs.Wrap("s.api.EditStickerPack", fromStatus(err))
	}

	return stickerPack(resp.GetPack()), resp.GetRemovedPhotoIds(), nil
}

// Delete returns the photos the pack had.
func (s *Stickers) Delete(ctx context.Context, accessToken, packID string) ([]string, error) {
	resp, err := s.api.DeleteStickerPack(withToken(ctx, accessToken), &userv1.DeleteStickerPackRequest{PackId: packID})
	if err != nil {
		return nil, errs.Wrap("s.api.DeleteStickerPack", fromStatus(err))
	}

	return resp.GetRemovedPhotoIds(), nil
}

// Author is whose photo the sticker is, "" for a sticker of the site or
// none.
func (s *Stickers) Author(ctx context.Context, sticker string) (string, error) {
	resp, err := s.api.GetStickerAuthors(ctx, &userv1.GetStickerAuthorsRequest{Stickers: []string{sticker}})
	if err != nil {
		return "", errs.Wrap("s.api.GetStickerAuthors", fromStatus(err))
	}

	return resp.GetAuthors()[sticker], nil
}

func stickerPacks(in []*userv1.StickerPack) []model.StickerPack {
	packs := make([]model.StickerPack, 0, len(in))
	for _, p := range in {
		packs = append(packs, stickerPack(p))
	}

	return packs
}

func stickerPack(p *userv1.StickerPack) model.StickerPack {
	return model.StickerPack{
		ID:        p.GetId(),
		Price:     p.GetPrice(),
		Stickers:  p.GetStickers(),
		Bought:    p.GetBought(),
		Added:     p.GetAdded(),
		AuthorID:  p.GetAuthorId(),
		Name:      p.GetName(),
		About:     p.GetAbout(),
		Published: p.GetPublished(),
		Holders:   int(p.GetHolders()),
	}
}

func (s *Stickers) Add(ctx context.Context, accessToken, packID string) error {
	_, err := s.api.AddStickerPack(withToken(ctx, accessToken), &userv1.AddStickerPackRequest{PackId: packID})

	return errs.Wrap("s.api.AddStickerPack", fromStatus(err))
}

func (s *Stickers) Remove(ctx context.Context, accessToken, packID string) error {
	_, err := s.api.RemoveStickerPack(withToken(ctx, accessToken), &userv1.RemoveStickerPackRequest{PackId: packID})

	return errs.Wrap("s.api.RemoveStickerPack", fromStatus(err))
}
