package rest_test

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

type fakeAds struct {
	token, edited, deleted string
	recorded               []string
	all                    bool
	draft                  model.AdDraft
	change                 model.AdChange
	err                    error
}

func (f *fakeAds) List(_ context.Context, token string, all bool) ([]model.Ad, error) {
	f.token, f.all = token, all
	return []model.Ad{
		{ID: "ad2", URL: "https://www.shop.example/sale", Title: "Распродажа", Text: "Всё по 100", PhotoOwnerID: "u1", PhotoID: "ph5", Active: true,
			Stats: &model.AdStats{Views: 10, Viewers: 4, Clicks: 2, Clickers: 1}},
		{ID: "ad1", URL: "https://b.example", Title: "Б"},
	}, f.err
}

func (f *fakeAds) Create(_ context.Context, token string, d model.AdDraft) (model.Ad, error) {
	f.token, f.draft = token, d
	ad := model.Ad{ID: "ad9", URL: d.URL, Title: d.Title, Text: d.Text, PhotoID: d.PhotoID, Active: d.Active}
	if d.PhotoID != "" {
		ad.PhotoOwnerID = "u1"
	}
	return ad, f.err
}

func (f *fakeAds) Edit(_ context.Context, token, adID string, c model.AdChange) (model.Ad, model.AdPhoto, error) {
	f.token, f.edited, f.change = token, adID, c
	ad := model.Ad{ID: adID, URL: "https://a.example", Title: "А", PhotoOwnerID: "u1", PhotoID: "ph1"}
	return ad, model.AdPhoto{OwnerID: "u7", PhotoID: "ph0"}, f.err
}

func (f *fakeAds) Record(_ context.Context, token, adID string, event model.AdEvent) error {
	f.token = token
	f.recorded = append(f.recorded, adID+"/"+string(event))
	return f.err
}

func (f *fakeAds) Delete(_ context.Context, token, adID string) (model.AdPhoto, error) {
	f.token, f.deleted = token, adID
	return model.AdPhoto{OwnerID: "u7", PhotoID: "ph0"}, f.err
}

type adBody struct {
	ID    string `json:"id"`
	URL   string `json:"url"`
	Title string `json:"title"`
	Text  string `json:"text"`
	Site  string `json:"site"`
	Photo *struct {
		Large string `json:"large"`
	} `json:"photo"`
	Active bool `json:"active"`
	Stats  *struct {
		Views    int64 `json:"views"`
		Clickers int64 `json:"clickers"`
	} `json:"stats"`
}

func TestListAds(t *testing.T) {
	t.Parallel()

	// The feed reads the shown ads; a guest too.
	h := newHarness(rest.Config{})
	w := h.do("GET", "/api/v1/ads", "")
	got := decode[struct {
		Ads []adBody `json:"ads"`
	}](t, w)
	if w.Code != http.StatusOK || len(got.Ads) != 2 || h.ads.all || h.ads.token != "" {
		t.Fatalf("%d %+v, asked %+v", w.Code, got, h.ads)
	}
	if ad := got.Ads[0]; ad.Site != "shop.example" || ad.Photo == nil || ad.Photo.Large != "/media/photos/u1/ph5/large.jpg" || !ad.Active {
		t.Fatalf("the first ad = %+v", ad)
	}
	if s := got.Ads[0].Stats; s == nil || s.Views != 10 || s.Clickers != 1 || got.Ads[1].Stats != nil {
		t.Fatalf("stats: %+v, %+v", got.Ads[0].Stats, got.Ads[1].Stats)
	}
	if got.Ads[1].Photo != nil {
		t.Fatalf("an ad without a cover: %+v", got.Ads[1])
	}

	if w = h.do("GET", "/api/v1/ads?all=true", "", "Authorization", "Bearer tok"); w.Code != http.StatusOK || !h.ads.all || h.ads.token != "tok" {
		t.Fatalf("all: %d, asked %+v", w.Code, h.ads)
	}
}

func TestCreateAd(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	h.users.admin = true
	w := h.do("POST", "/api/v1/ads", `{"url":"a.example","title":"А","text":"Т","photo":"ph1","active":true}`, "Authorization", "Bearer tok")
	got := decode[adBody](t, w)
	if w.Code != http.StatusCreated || got.ID != "ad9" || got.Photo == nil || h.ads.draft != (model.AdDraft{URL: "a.example", Title: "А", Text: "Т", PhotoID: "ph1", Active: true}) {
		t.Fatalf("%d %+v, asked %+v", w.Code, got, h.ads)
	}
	// The cover is the administrator's upload, kept for good.
	if !slices.Equal(h.media.claimed, []string{"ph1"}) || !slices.Equal(h.media.published, []string{"u1/ph1"}) {
		t.Fatalf("claimed %v, published %v", h.media.claimed, h.media.published)
	}

	// Refused, the cover goes.
	h = newHarness(rest.Config{})
	h.users.admin = true
	h.ads.err = failure.New(failure.InvalidArgument, "ad title is empty")
	if w = h.do("POST", "/api/v1/ads", `{"url":"a.example","photo":"ph1"}`, "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest ||
		!slices.Equal(h.media.unpublished, []string{"u1/ph1"}) {
		t.Fatalf("refused: %d, unpublished %v", w.Code, h.media.unpublished)
	}

	// A stranger's photo is not taken.
	h = newHarness(rest.Config{})
	h.users.admin = true
	if w = h.do("POST", "/api/v1/ads", `{"url":"a.example","title":"А","photo":"ph7"}`, "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest || h.ads.token != "" {
		t.Fatalf("a stranger's photo: %d", w.Code)
	}

	// Nobody else touches a photo for an ad.
	h = newHarness(rest.Config{})
	if w = h.do("POST", "/api/v1/ads", `{"url":"a.example","title":"А","photo":"ph1"}`, "Authorization", "Bearer tok"); w.Code != http.StatusForbidden ||
		len(h.media.claimed) != 0 || h.ads.token != "" {
		t.Fatalf("not an administrator: %d", w.Code)
	}
	if w = newHarness(rest.Config{}).do("POST", "/api/v1/ads", `{"url":"a.example"}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("without token: %d", w.Code)
	}
}

func TestEditAndDeleteAd(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	h.users.admin = true
	w := h.do("PATCH", "/api/v1/ads/ad1", `{"active":false,"photo":"ph2"}`, "Authorization", "Bearer tok")
	if w.Code != http.StatusOK || h.ads.edited != "ad1" || h.ads.change.Active == nil || *h.ads.change.Active ||
		h.ads.change.PhotoID == nil || *h.ads.change.PhotoID != "ph2" || h.ads.change.Title != nil {
		t.Fatalf("%d %s, asked %+v", w.Code, w.Body, h.ads.change)
	}
	// The new cover is kept, the old one goes.
	if !slices.Equal(h.media.published, []string{"u1/ph2"}) || !slices.Equal(h.media.unpublished, []string{"u7/ph0"}) {
		t.Fatalf("published %v, unpublished %v", h.media.published, h.media.unpublished)
	}

	// Without a new cover nothing is claimed.
	h = newHarness(rest.Config{})
	h.users.admin = true
	if w = h.do("PATCH", "/api/v1/ads/ad1", `{"title":"Б"}`, "Authorization", "Bearer tok"); w.Code != http.StatusOK || len(h.media.claimed) != 0 || *h.ads.change.Title != "Б" {
		t.Fatalf("a title: %d, claimed %v", w.Code, h.media.claimed)
	}

	h = newHarness(rest.Config{})
	if w = h.do("DELETE", "/api/v1/ads/ad1", "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent || h.ads.deleted != "ad1" ||
		!slices.Equal(h.media.unpublished, []string{"u7/ph0"}) {
		t.Fatalf("delete: %d, unpublished %v", w.Code, h.media.unpublished)
	}

	h = newHarness(rest.Config{})
	h.ads.err = failure.New(failure.PermissionDenied, "permission denied")
	if w = h.do("DELETE", "/api/v1/ads/ad1", "", "Authorization", "Bearer tok"); w.Code != http.StatusForbidden || len(h.media.unpublished) != 0 {
		t.Fatalf("delete by a stranger: %d", w.Code)
	}
	if w = newHarness(rest.Config{}).do("PATCH", "/api/v1/ads/ad1", `{"title":"Б"}`, "Authorization", "Bearer tok"); w.Code != http.StatusForbidden {
		t.Fatalf("edit by a stranger: %d", w.Code)
	}
}

func TestRecordAd(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	for _, path := range []string{"/api/v1/ads/ad2/views", "/api/v1/ads/ad2/clicks"} {
		if w := h.do("POST", path, "", "Authorization", "Bearer tok"); w.Code != http.StatusNoContent || h.ads.token != "tok" {
			t.Fatalf("%s: %d", path, w.Code)
		}
	}
	if !slices.Equal(h.ads.recorded, []string{"ad2/view", "ad2/click"}) {
		t.Fatalf("recorded %v", h.ads.recorded)
	}

	// A guest is not counted.
	h = newHarness(rest.Config{})
	if w := h.do("POST", "/api/v1/ads/ad2/views", ""); w.Code != http.StatusUnauthorized || len(h.ads.recorded) != 0 {
		t.Fatalf("a guest: %d", w.Code)
	}
}
