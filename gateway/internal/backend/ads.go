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

// Record counts that the caller saw an ad in the feed or followed it.
func (a *Ads) Record(ctx context.Context, accessToken, adID string, event model.AdEvent) error {
	ctx = withToken(ctx, accessToken)

	if event == model.AdClicked {
		_, err := a.api.ClickAd(ctx, &userv1.ClickAdRequest{Id: adID})
		return errs.Wrap("a.api.ClickAd", fromStatus(err))
	}

	_, err := a.api.ViewAd(ctx, &userv1.ViewAdRequest{Id: adID})

	return errs.Wrap("a.api.ViewAd", fromStatus(err))
}

func toAd(ad *userv1.Ad) model.Ad {
	var stats *model.AdStats
	if s := ad.GetStats(); s != nil {
		stats = &model.AdStats{Views: s.GetViews(), Viewers: s.GetViewers(), Clicks: s.GetClicks(), Clickers: s.GetClickers()}
	}

	return model.Ad{
		Stats:        stats,
		ID:           ad.GetId(),
		URL:          ad.GetUrl(),
		Title:        ad.GetTitle(),
		Text:         ad.GetText(),
		PhotoOwnerID: ad.GetPhotoOwnerId(),
		PhotoID:      ad.GetPhotoId(),
		Active:       ad.GetActive(),
	}
}
