package rest

import (
	"net/http"
	"time"

	"github.com/vexner67/freenet/gateway/internal/model"
)

type giftCollectionJSON struct {
	ID    string   `json:"id"`
	Gifts []string `json:"gifts"`
	// Price is what a gift of the collection costs, in votes, unless
	// priced otherwise; 0 is free.
	Price int64 `json:"price"`
	// Prices are what each of gifts costs now, in their order.
	Prices []int64 `json:"prices"`
	// Prime: only those with Freenet Prime send its gifts.
	Prime bool `json:"prime"`
}

type giftPriceRequest struct {
	Price int64 `json:"price"`
}

type giftCatalogResponse struct {
	Collections []giftCollectionJSON `json:"collections"`
}

type sendGiftRequest struct {
	Gift      string `json:"gift"`
	Caption   string `json:"caption"`
	Anonymous bool   `json:"anonymous"`
}

// giftJSON is a gift sent to a page as the viewer sees it: sender is null
// and caption empty when the viewer is not told them (an anonymous gift),
// sender alone when the page of the sender is deleted.
type giftJSON struct {
	ID        string      `json:"id"`
	Gift      string      `json:"gift"`
	Caption   string      `json:"caption"`
	Anonymous bool        `json:"anonymous"`
	SentAt    string      `json:"sentAt"`
	Sender    *authorJSON `json:"sender"`
}

type giftPageResponse struct {
	Gifts      []giftJSON `json:"gifts"`
	NextCursor string     `json:"nextCursor"`
	Total      int        `json:"total"`
}

func toGiftJSON(g model.Gift) giftJSON {
	gift := giftJSON{
		ID:        g.ID,
		Gift:      g.Gift,
		Caption:   g.Caption,
		Anonymous: g.Anonymous,
		SentAt:    g.SentAt.UTC().Format(time.RFC3339),
	}

	if g.Sender != nil {
		sender := toAuthorJSON(authorOfProfile(*g.Sender))
		gift.Sender = &sender
	}

	return gift
}

// giftCatalog is what there is to send ("Выбрать подарок"), public.
func (a *api) giftCatalog(w http.ResponseWriter, r *http.Request) {
	catalog, err := a.gifts.Catalog(r.Context())
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	resp := giftCatalogResponse{Collections: make([]giftCollectionJSON, 0, len(catalog))}
	for _, c := range catalog {
		collection := giftCollectionJSON(c)
		if collection.Prices == nil {
			collection.Prices = []int64{}
		}

		resp.Collections = append(resp.Collections, collection)
	}

	writeJSON(w, http.StatusOK, resp)
}

// listGifts is "Подарки" of a person, public, as "Кому видны мои подарки"
// allows; those of a deleted page are gone, as its wall.
func (a *api) listGifts(w http.ResponseWriter, r *http.Request) {
	token, err := optionalBearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	limit, err := pageLimit(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	owner, err := a.visiblePage(r, token, giftsPart)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	page, err := a.gifts.List(r.Context(), token, owner.ID, r.URL.Query().Get("cursor"), limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	resp := giftPageResponse{Gifts: make([]giftJSON, 0, len(page.Gifts)), NextCursor: page.NextCursor, Total: page.Total}
	for _, g := range page.Gifts {
		resp.Gifts = append(resp.Gifts, toGiftJSON(g))
	}

	writeJSON(w, http.StatusOK, resp)
}

// sendGift is "Отправить подарок". A filled-in profile is needed, as to
// post: a gift is signed with the name of its sender. Whether the page
// takes gifts from the caller is the user service's to say.
func (a *api) sendGift(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req sendGiftRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	if _, err = a.completedMe(r, token, "fill in your profile before sending gifts"); err != nil {
		a.writeError(w, r, err)
		return
	}

	recipient, err := a.users.GetProfile(r.Context(), token, r.PathValue("nickname"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	gift, err := a.gifts.Send(r.Context(), token, recipient.ID, model.GiftDraft{
		Gift:      req.Gift,
		Caption:   req.Caption,
		Anonymous: req.Anonymous,
	})
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, toGiftJSON(gift))
}

// deleteGift removes a gift from the caller's page.
func (a *api) deleteGift(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.gifts.Delete(r.Context(), token, r.PathValue("id")); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// setGiftPrice is "Подарки" of the admin panel: an administrator of the
// site makes a gift cost so many votes from now on.
func (a *api) setGiftPrice(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req giftPriceRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.gifts.SetPrice(r.Context(), token, r.PathValue("gift"), req.Price); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
