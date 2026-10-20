package grpc

import (
	"context"

	"github.com/google/uuid"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"github.com/vexner67/freenet/user/internal/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type AdService interface {
	Shown(ctx context.Context) ([]*user.Ad, error)
	All(ctx context.Context, adminID uuid.UUID) ([]*user.Ad, map[uuid.UUID]user.AdStats, error)
	Create(ctx context.Context, adminID uuid.UUID, d user.AdDraft) (*user.Ad, error)
	Edit(ctx context.Context, adminID, id uuid.UUID, c user.AdChange) (*user.Ad, user.AdPhoto, error)
	Delete(ctx context.Context, adminID, id uuid.UUID) (user.AdPhoto, error)
	Record(ctx context.Context, readerID, adID uuid.UUID, event user.AdEvent) error
}

func (h *UserHandler) ListAds(ctx context.Context, req *userv1.ListAdsRequest) (*userv1.ListAdsResponse, error) {
	var (
		ads   []*user.Ad
		stats map[uuid.UUID]user.AdStats
		err   error
	)

	if req.GetAll() {
		adminID, idErr := currentUserID(ctx)
		if idErr != nil {
			return nil, idErr
		}

		ads, stats, err = h.ads.All(ctx, adminID)
	} else {
		ads, err = h.ads.Shown(ctx)
	}

	if err != nil {
		return nil, err
	}

	resp := &userv1.ListAdsResponse{Ads: make([]*userv1.Ad, 0, len(ads))}
	for _, ad := range ads {
		a := toProtoAd(ad)
		if req.GetAll() {
			a.Stats = toProtoAdStats(stats[ad.ID()])
		}
		resp.Ads = append(resp.Ads, a)
	}

	return resp, nil
}

func (h *UserHandler) CreateAd(ctx context.Context, req *userv1.CreateAdRequest) (*userv1.CreateAdResponse, error) {
	adminID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	d := user.AdDraft{Active: req.GetActive()}

	if d.URL, err = user.NewAdURL(req.GetUrl()); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if d.Title, err = user.NewAdTitle(req.GetTitle()); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if d.Text, err = user.NewAdText(req.GetText()); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if d.Photo, err = fromProtoAdPhoto(req.GetPhotoId()); err != nil {
		return nil, err
	}

	ad, err := h.ads.Create(ctx, adminID, d)
	if err != nil {
		return nil, err
	}

	return &userv1.CreateAdResponse{Ad: toProtoAd(ad)}, nil
}

func (h *UserHandler) EditAd(ctx context.Context, req *userv1.EditAdRequest) (*userv1.EditAdResponse, error) {
	adminID, id, err := callerAndAd(ctx, req.GetId())
	if err != nil {
		return nil, err
	}

	c, err := fromProtoAdChange(req)
	if err != nil {
		return nil, err
	}

	ad, dropped, err := h.ads.Edit(ctx, adminID, id, c)
	if err != nil {
		return nil, err
	}

	owner, photo := toProtoAdPhoto(dropped)

	return &userv1.EditAdResponse{Ad: toProtoAd(ad), DroppedPhotoOwnerId: owner, DroppedPhotoId: photo}, nil
}

func (h *UserHandler) DeleteAd(ctx context.Context, req *userv1.DeleteAdRequest) (*userv1.DeleteAdResponse, error) {
	adminID, id, err := callerAndAd(ctx, req.GetId())
	if err != nil {
		return nil, err
	}

	dropped, err := h.ads.Delete(ctx, adminID, id)
	if err != nil {
		return nil, err
	}

	owner, photo := toProtoAdPhoto(dropped)

	return &userv1.DeleteAdResponse{PhotoOwnerId: owner, PhotoId: photo}, nil
}

func (h *UserHandler) ViewAd(ctx context.Context, req *userv1.ViewAdRequest) (*userv1.ViewAdResponse, error) {
	if err := h.recordAd(ctx, req.GetId(), user.AdViewed); err != nil {
		return nil, err
	}

	return &userv1.ViewAdResponse{}, nil
}

func (h *UserHandler) ClickAd(ctx context.Context, req *userv1.ClickAdRequest) (*userv1.ClickAdResponse, error) {
	if err := h.recordAd(ctx, req.GetId(), user.AdClicked); err != nil {
		return nil, err
	}

	return &userv1.ClickAdResponse{}, nil
}

func (h *UserHandler) recordAd(ctx context.Context, adID string, event user.AdEvent) error {
	readerID, id, err := callerAndAd(ctx, adID)
	if err != nil {
		return err
	}

	return h.ads.Record(ctx, readerID, id, event)
}

func callerAndAd(ctx context.Context, adID string) (uuid.UUID, uuid.UUID, error) {
	adminID, err := currentUserID(ctx)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	id, err := parseID(adID, "ad id")
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	return adminID, id, nil
}
