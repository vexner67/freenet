package rest

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
)

// Ads are the advertising of the global feed, which the user service
// keeps: everyone reads the shown ads, the administrators of the site
// make and change them.
type Ads interface {
	// List is the shown ads, newest first; all of them with all, for the
	// administrators.
	List(ctx context.Context, accessToken string, all bool) ([]model.Ad, error)
	Create(ctx context.Context, accessToken string, d model.AdDraft) (model.Ad, error)
	// Edit returns the cover the ad no longer shows.
	Edit(ctx context.Context, accessToken, adID string, c model.AdChange) (model.Ad, model.AdPhoto, error)
	// Delete returns the cover the ad had.
	Delete(ctx context.Context, accessToken, adID string) (model.AdPhoto, error)
}

type createAdRequest struct {
	URL   string `json:"url"`
	Title string `json:"title"`
	Text  string `json:"text"`
	// Photo is an upload of the caller (POST /me/photos), empty for no
	// cover.
	Photo  string `json:"photo"`
	Active bool   `json:"active"`
}

// editAdRequest changes what it names; photo "" takes the cover away.
type editAdRequest struct {
	URL    *string `json:"url"`
	Title  *string `json:"title"`
	Text   *string `json:"text"`
	Photo  *string `json:"photo"`
	Active *bool   `json:"active"`
}

type adJSON struct {
	ID    string `json:"id"`
	URL   string `json:"url"`
	Title string `json:"title"`
	Text  string `json:"text"`
	// Site is the host of the link, without "www.".
	Site   string     `json:"site"`
	Photo  *photoJSON `json:"photo"`
	Active bool       `json:"active"`
}

type adsResponse struct {
	Ads []adJSON `json:"ads"`
}

func toAdJSON(ad model.Ad) adJSON {
	j := adJSON{ID: ad.ID, URL: ad.URL, Title: ad.Title, Text: ad.Text, Site: adSite(ad.URL), Active: ad.Active}
	if ad.PhotoID != "" {
		photo := toPhotoJSON(ad.PhotoOwnerID, ad.PhotoID)
		j.Photo = &photo
	}

	return j
}

// adSite is the host an ad leads to, as a card names its site.
func adSite(link string) string {
	u, err := url.Parse(link)
	if err != nil {
		return ""
	}

	return strings.TrimPrefix(u.Hostname(), "www.")
}

// listAds is the shown ads, which the global feed slips between its
// posts; ?all=true is every ad, for the administrators.
func (a *api) listAds(w http.ResponseWriter, r *http.Request) {
	token, err := optionalBearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	ads, err := a.ads.List(r.Context(), token, r.URL.Query().Get("all") == "true")
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	resp := adsResponse{Ads: make([]adJSON, 0, len(ads))}
	for _, ad := range ads {
		resp.Ads = append(resp.Ads, toAdJSON(ad))
	}

	writeJSON(w, http.StatusOK, resp)
}

// createAd makes an ad; its cover is kept for good before the ad shows
// it, and goes if the ad is refused.
func (a *api) createAd(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req createAdRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	adminID, err := a.adminID(r, token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.keepAdPhoto(r, token, adminID, req.Photo); err != nil {
		a.writeError(w, r, err)
		return
	}

	ad, err := a.ads.Create(r.Context(), token, model.AdDraft{
		URL:     req.URL,
		Title:   req.Title,
		Text:    req.Text,
		PhotoID: req.Photo,
		Active:  req.Active,
	})
	if err != nil {
		a.dropAdPhoto(r, model.AdPhoto{OwnerID: adminID, PhotoID: req.Photo})
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, toAdJSON(ad))
}

// editAd changes an ad; a new cover replaces the old one, whose file goes.
func (a *api) editAd(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req editAdRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	adminID, err := a.adminID(r, token)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	added := ""
	if req.Photo != nil {
		added = *req.Photo
	}

	if err = a.keepAdPhoto(r, token, adminID, added); err != nil {
		a.writeError(w, r, err)
		return
	}

	ad, dropped, err := a.ads.Edit(r.Context(), token, r.PathValue("id"), model.AdChange{
		URL:     req.URL,
		Title:   req.Title,
		Text:    req.Text,
		PhotoID: req.Photo,
		Active:  req.Active,
	})
	if err != nil {
		a.dropAdPhoto(r, model.AdPhoto{OwnerID: adminID, PhotoID: added})
		a.writeError(w, r, err)
		return
	}

	a.dropAdPhoto(r, dropped)
	writeJSON(w, http.StatusOK, toAdJSON(ad))
}

// deleteAd takes an ad away with its cover.
func (a *api) deleteAd(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	dropped, err := a.ads.Delete(r.Context(), token, r.PathValue("id"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	a.dropAdPhoto(r, dropped)
	w.WriteHeader(http.StatusNoContent)
}

// adminID is the caller, who has to run the site: the cover is not taken
// for anyone else.
func (a *api) adminID(r *http.Request, token string) (string, error) {
	me, err := a.users.GetMe(r.Context(), token)
	if err != nil {
		return "", err
	}

	if !me.Admin {
		return "", failure.New(failure.PermissionDenied, "permission denied")
	}

	return me.ID, nil
}

// keepAdPhoto takes the caller's upload for a cover and keeps it for
// good, as a sticker: an ad shows longer than media keeps what nothing
// shows. Refused, it is given back.
func (a *api) keepAdPhoto(r *http.Request, token, ownerID, photoID string) error {
	if photoID == "" {
		return nil
	}

	if err := a.claimPhotos(r, token, []string{photoID}); err != nil {
		return err
	}

	if err := a.media.PublishPhotos(r.Context(), ownerID, []string{photoID}); err != nil {
		a.releasePhotos(r, token, []string{photoID}, err)
		return err
	}

	return nil
}

// dropAdPhoto deletes the file of a cover no ad shows. A failure only
// leaves an unused file behind, so it is logged.
func (a *api) dropAdPhoto(r *http.Request, p model.AdPhoto) {
	if p.PhotoID == "" {
		return
	}

	if err := a.media.DeletePublishedPhotos(r.Context(), p.OwnerID, []string{p.PhotoID}); err != nil {
		a.logger.WarnContext(r.Context(), "the cover of an ad left behind", "error", err)
	}
}
