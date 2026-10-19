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

// maxStickerAuthors is how many stickers GetStickerAuthors looks up at
// once.
const maxStickerAuthors = 100

type StickerService interface {
	Catalog(ctx context.Context, userID uuid.UUID) ([]service.StickerPackView, error)
	Add(ctx context.Context, userID uuid.UUID, pack user.StickerPackID) error
	Remove(ctx context.Context, userID uuid.UUID, pack user.StickerPackID) error
	Authored(ctx context.Context, authorID uuid.UUID) ([]service.AuthoredPack, error)
	Create(ctx context.Context, authorID uuid.UUID, d user.StickerPackDraft, photoIDs []uuid.UUID) (*user.CustomStickerPack, error)
	Edit(ctx context.Context, userID, packID uuid.UUID, e service.StickerPackEdit) (*user.CustomStickerPack, []uuid.UUID, error)
	Delete(ctx context.Context, userID, packID uuid.UUID) ([]uuid.UUID, error)
	Authors(ctx context.Context, stickers []user.Sticker) (map[user.Sticker]uuid.UUID, error)
}

func (h *UserHandler) GetStickerCatalog(ctx context.Context, _ *userv1.GetStickerCatalogRequest) (*userv1.GetStickerCatalogResponse, error) {
	views, err := h.stickerService.Catalog(ctx, viewerID(ctx))
	if err != nil {
		return nil, err
	}

	resp := &userv1.GetStickerCatalogResponse{Packs: make([]*userv1.StickerPack, 0, len(views))}
	for _, v := range views {
		p := toProtoStickerPack(v.Pack)
		p.Bought, p.Added, p.Holders = v.Bought, v.Added, int32(v.Holders) //nolint:gosec // bounded by the users

		resp.Packs = append(resp.Packs, p)
	}

	return resp, nil
}

func (h *UserHandler) ListMyStickerPacks(ctx context.Context, _ *userv1.ListMyStickerPacksRequest) (*userv1.ListMyStickerPacksResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	authored, err := h.stickerService.Authored(ctx, userID)
	if err != nil {
		return nil, err
	}

	resp := &userv1.ListMyStickerPacksResponse{Packs: make([]*userv1.StickerPack, 0, len(authored))}
	for _, a := range authored {
		p := toProtoStickerPack(a.Pack.Pack())
		// The author has the pack among their stickers from the start.
		p.Bought, p.Added, p.Holders = true, true, int32(a.Holders) //nolint:gosec // bounded by the users

		resp.Packs = append(resp.Packs, p)
	}

	return resp, nil
}

func (h *UserHandler) CreateStickerPack(ctx context.Context, req *userv1.CreateStickerPackRequest) (*userv1.CreateStickerPackResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	d, err := fromProtoStickerPackDraft(req.GetName(), req.GetAbout(), req.GetPrice(), req.GetPublished())
	if err != nil {
		return nil, err
	}

	photos, err := parsePhotoIDs(req.GetPhotoIds())
	if err != nil {
		return nil, err
	}

	p, err := h.stickerService.Create(ctx, userID, d, photos)
	if err != nil {
		return nil, err
	}

	return &userv1.CreateStickerPackResponse{Pack: ownStickerPack(p)}, nil
}

func (h *UserHandler) EditStickerPack(ctx context.Context, req *userv1.EditStickerPackRequest) (*userv1.EditStickerPackResponse, error) {
	userID, packID, err := callerAndCustomPack(ctx, req.GetPackId())
	if err != nil {
		return nil, err
	}

	d, err := fromProtoStickerPackDraft(req.GetName(), req.GetAbout(), req.GetPrice(), req.GetPublished())
	if err != nil {
		return nil, err
	}

	added, err := parsePhotoIDs(req.GetAddPhotoIds())
	if err != nil {
		return nil, err
	}

	removed, err := parsePhotoIDs(req.GetRemovePhotoIds())
	if err != nil {
		return nil, err
	}

	p, gone, err := h.stickerService.Edit(ctx, userID, packID, service.StickerPackEdit{Draft: d, Added: added, Removed: removed})
	if err != nil {
		return nil, err
	}

	return &userv1.EditStickerPackResponse{Pack: ownStickerPack(p), RemovedPhotoIds: uuidStrings(gone)}, nil
}

func (h *UserHandler) DeleteStickerPack(ctx context.Context, req *userv1.DeleteStickerPackRequest) (*userv1.DeleteStickerPackResponse, error) {
	userID, packID, err := callerAndCustomPack(ctx, req.GetPackId())
	if err != nil {
		return nil, err
	}

	gone, err := h.stickerService.Delete(ctx, userID, packID)
	if err != nil {
		return nil, err
	}

	return &userv1.DeleteStickerPackResponse{RemovedPhotoIds: uuidStrings(gone)}, nil
}

func (h *UserHandler) GetStickerAuthors(ctx context.Context, req *userv1.GetStickerAuthorsRequest) (*userv1.GetStickerAuthorsResponse, error) {
	if len(req.GetStickers()) > maxStickerAuthors {
		return nil, status.Error(codes.InvalidArgument, "too many stickers")
	}

	stickers := make([]user.Sticker, len(req.GetStickers()))
	for i, s := range req.GetStickers() {
		stickers[i] = user.Sticker(s)
	}

	authors, err := h.stickerService.Authors(ctx, stickers)
	if err != nil {
		return nil, err
	}

	resp := &userv1.GetStickerAuthorsResponse{Authors: make(map[string]string, len(authors))}
	for s, author := range authors {
		resp.Authors[s.String()] = author.String()
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

// callerAndCustomPack are the caller and the pack of a person they name.
func callerAndCustomPack(ctx context.Context, packID string) (uuid.UUID, uuid.UUID, error) {
	userID, pack, err := callerAndPack(ctx, packID)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	id, ok := pack.Custom()
	if !ok {
		return uuid.Nil, uuid.Nil, status.Error(codes.InvalidArgument, user.ErrUnknownStickerPack.Error())
	}

	return userID, id, nil
}

func fromProtoStickerPackDraft(name, about string, price int64, published bool) (user.StickerPackDraft, error) {
	n, err := user.NewStickerPackName(name)
	if err != nil {
		return user.StickerPackDraft{}, status.Error(codes.InvalidArgument, err.Error())
	}

	a, err := user.NewStickerPackAbout(about)
	if err != nil {
		return user.StickerPackDraft{}, status.Error(codes.InvalidArgument, err.Error())
	}

	return user.StickerPackDraft{Name: n, About: a, Price: price, Published: published}, nil
}

func parsePhotoIDs(ids []string) ([]uuid.UUID, error) {
	if len(ids) > user.MaxPackStickers {
		return nil, status.Error(codes.InvalidArgument, user.ErrTooManyPackStickers.Error())
	}

	photos := make([]uuid.UUID, 0, len(ids))
	for _, s := range ids {
		id, err := parseID(s, "photo id")
		if err != nil {
			return nil, err
		}

		photos = append(photos, id)
	}

	return photos, nil
}

func uuidStrings(ids []uuid.UUID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id.String()
	}

	return out
}

func toProtoStickerPack(p user.StickerPack) *userv1.StickerPack {
	stickers := make([]string, 0, len(p.Stickers))
	for _, s := range p.Stickers {
		stickers = append(stickers, s.String())
	}

	out := &userv1.StickerPack{
		Id:        p.ID.String(),
		Price:     p.Price,
		Stickers:  stickers,
		Name:      p.Name,
		About:     p.About,
		Published: p.Published,
	}
	if p.Author != uuid.Nil {
		out.AuthorId = p.Author.String()
	}

	return out
}

// ownStickerPack is a pack as its author sees it, among their stickers.
func ownStickerPack(p *user.CustomStickerPack) *userv1.StickerPack {
	out := toProtoStickerPack(p.Pack())
	out.Bought, out.Added = true, true

	return out
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
