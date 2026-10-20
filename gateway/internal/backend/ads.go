package backend

import (
	"context"

	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/pkg/errs"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
)

// Ads is rest.Ads over the user service, which keeps the ads of the
// global feed.
type Ads struct {
	api userv1.UserServiceClient
}

func NewAds(api userv1.UserServiceClient) *Ads {
	return &Ads{
		api: api,
	}
}

func (a *Ads) List(ctx context.Context, accessToken string, all bool) ([]model.Ad, error) {
	resp, err := a.api.ListAds(withToken(ctx, accessToken), &userv1.ListAdsRequest{All: all})
	if err != nil {
		return nil, errs.Wrap("a.api.ListAds", fromStatus(err))
	}

	ads := make([]model.Ad, 0, len(resp.GetAds()))
	for _, ad := range resp.GetAds() {
		ads = append(ads, toAd(ad))
	}

	return ads, nil
}

func (a *Ads) Create(ctx context.Context, accessToken string, d model.AdDraft) (model.Ad, error) {
	resp, err := a.api.CreateAd(withToken(ctx, accessToken), &userv1.CreateAdRequest{
		Url:     d.URL,
		Title:   d.Title,
		Text:    d.Text,
		PhotoId: d.PhotoID,
		Active:  d.Active,
	})
	if err != nil {
		return model.Ad{}, errs.Wrap("a.api.CreateAd", fromStatus(err))
	}

	return toAd(resp.GetAd()), nil
}

func (a *Ads) Edit(ctx context.Context, accessToken, adID string, c model.AdChange) (model.Ad, model.AdPhoto, error) {
	resp, err := a.api.EditAd(withToken(ctx, accessToken), &userv1.EditAdRequest{
		Id:      adID,
		Url:     c.URL,
		Title:   c.Title,
		Text:    c.Text,
		PhotoId: c.PhotoID,
		Active:  c.Active,
	})
	if err != nil {
		return model.Ad{}, model.AdPhoto{}, errs.Wrap("a.api.EditAd", fromStatus(err))
	}

	return toAd(resp.GetAd()), model.AdPhoto{OwnerID: resp.GetDroppedPhotoOwnerId(), PhotoID: resp.GetDroppedPhotoId()}, nil
}

func (a *Ads) Delete(ctx context.Context, accessToken, adID string) (model.AdPhoto, error) {
	resp, err := a.api.DeleteAd(withToken(ctx, accessToken), &userv1.DeleteAdRequest{Id: adID})
	if err != nil {
		return model.AdPhoto{}, errs.Wrap("a.api.DeleteAd", fromStatus(err))
	}

	return model.AdPhoto{OwnerID: resp.GetPhotoOwnerId(), PhotoID: resp.GetPhotoId()}, nil
}

func toAd(ad *userv1.Ad) model.Ad {
	return model.Ad{
		ID:           ad.GetId(),
		URL:          ad.GetUrl(),
		Title:        ad.GetTitle(),
		Text:         ad.GetText(),
		PhotoOwnerID: ad.GetPhotoOwnerId(),
		PhotoID:      ad.GetPhotoId(),
		Active:       ad.GetActive(),
	}
}
