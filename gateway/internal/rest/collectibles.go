package rest

import (
	"net/http"
	"time"

	"github.com/vexner67/freenet/gateway/internal/model"
)

// collectibleJSON is a collectible gift as the viewer sees it: owner is
// null when the site holds it (site) or the page of the owner is gone;
// history comes only with one gift, newest first.
type collectibleJSON struct {
	Gift    giftJSON          `json:"gift"`
	Owner   *authorJSON       `json:"owner"`
	Site    bool              `json:"site"`
	Mine    bool              `json:"mine"`
	Auction *giftAuctionJSON  `json:"auction"`
	History []giftHistoryJSON `json:"history"`
}

type giftAuctionJSON struct {
	Start  int64       `json:"start"`
	TopBid int64       `json:"topBid"`
	MinBid int64       `json:"minBid"`
	Bids   int         `json:"bids"`
	EndsAt string      `json:"endsAt"`
	Site   bool        `json:"site"`
	Seller *authorJSON `json:"seller"`
	Bidder *authorJSON `json:"bidder"`
	Mine   bool        `json:"mine"`
}

type giftHistoryJSON struct {
	Kind  string      `json:"kind"`
	At    string      `json:"at"`
	Price int64       `json:"price"`
	From  *authorJSON `json:"from"`
	To    *authorJSON `json:"to"`
}

type collectiblePageResponse struct {
	Collectibles []collectibleJSON `json:"collectibles"`
	NextCursor   string            `json:"nextCursor"`
}

type transferGiftRequest struct {
	Nickname string `json:"nickname"`
}

type giftSaleRequest struct {
	Price int64 `json:"price"`
}

type startAuctionRequest struct {
	Start int64 `json:"start"`
	Hours int   `json:"hours"`
}

type bidRequest struct {
	Amount int64 `json:"amount"`
}

type mintAuctionRequest struct {
	Gift     string `json:"gift"`
	Model    string `json:"model"`
	Backdrop string `json:"backdrop"`
	Symbol   string `json:"symbol"`
	Start    int64  `json:"start"`
	Hours    int    `json:"hours"`
}

type giftSupplyRequest struct {
	Supply int64 `json:"supply"`
}

type balanceResponse struct {
	Balance int64 `json:"balance"`
}

func personJSON(p *model.PublicProfile) *authorJSON {
	if p == nil {
		return nil
	}

	a := toAuthorJSON(authorOfProfile(*p))

	return &a
}

func toCollectibleJSON(c model.Collectible) collectibleJSON {
	out := collectibleJSON{
		Gift:    toGiftJSON(c.Gift),
		Owner:   personJSON(c.Owner),
		Site:    c.Site,
		Mine:    c.Mine,
		History: make([]giftHistoryJSON, 0, len(c.History)),
	}

	if a := c.Auction; a != nil {
		out.Auction = &giftAuctionJSON{
			Start:  a.Start,
			TopBid: a.TopBid,
			MinBid: a.MinBid,
			Bids:   a.Bids,
			EndsAt: a.EndsAt.UTC().Format(time.RFC3339),
			Site:   a.Site,
			Seller: personJSON(a.Seller),
			Bidder: personJSON(a.Bidder),
			Mine:   a.Mine,
		}
	}

	for _, l := range c.History {
		out.History = append(out.History, giftHistoryJSON{
			Kind:  l.Kind,
			At:    l.At.UTC().Format(time.RFC3339),
			Price: l.Price,
			From:  personJSON(l.From),
			To:    personJSON(l.To),
		})
	}

	return out
}

func toCollectiblePage(page model.CollectiblePage) collectiblePageResponse {
	resp := collectiblePageResponse{Collectibles: make([]collectibleJSON, 0, len(page.Collectibles)), NextCursor: page.NextCursor}
	for _, c := range page.Collectibles {
		resp.Collectibles = append(resp.Collectibles, toCollectibleJSON(c))
	}

	return resp
}

// getCollectible is the page of a collectible gift, public: who has it,
// its sale or auction and its history.
func (a *api) getCollectible(w http.ResponseWriter, r *http.Request) {
	token, err := optionalBearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	c, err := a.gifts.Collectible(r.Context(), token, r.PathValue("id"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, toCollectibleJSON(c))
}

// upgradeGift is "Улучшить": the caller's gift becomes a collectible.
func (a *api) upgradeGift(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	g, err := a.gifts.Upgrade(r.Context(), token, r.PathValue("id"))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, toGiftJSON(g))
}

// transferGift is "Передать": the caller's collectible goes to the page
// named, signed with the caller's name — a filled-in profile is needed, as
// to send a gift.
func (a *api) transferGift(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req transferGiftRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	if _, err = a.completedMe(r, token, "fill in your profile before sending gifts"); err != nil {
		a.writeError(w, r, err)
		return
	}

	recipient, err := a.users.GetProfile(r.Context(), token, req.Nickname)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.gifts.Transfer(r.Context(), token, r.PathValue("id"), recipient.ID); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *api) putGiftOnSale(w http.ResponseWriter, r *http.Request) {
	var req giftSaleRequest
	if err := decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	a.setGiftSale(w, r, req.Price)
}

func (a *api) takeGiftOffSale(w http.ResponseWriter, r *http.Request) {
	a.setGiftSale(w, r, 0)
}

func (a *api) setGiftSale(w http.ResponseWriter, r *http.Request, price int64) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	g, err := a.gifts.SetSale(r.Context(), token, r.PathValue("id"), price)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, toGiftJSON(g))
}

// buyGift buys a collectible for the price the buyer was shown.
func (a *api) buyGift(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req giftSaleRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	balance, err := a.gifts.Buy(r.Context(), token, r.PathValue("id"), req.Price)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, balanceResponse{balance})
}

func (a *api) startGiftAuction(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req startAuctionRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.gifts.StartAuction(r.Context(), token, r.PathValue("id"), req.Start, req.Hours); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *api) cancelGiftAuction(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.gifts.CancelAuction(r.Context(), token, r.PathValue("id")); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *api) bidGift(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req bidRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	balance, err := a.gifts.Bid(r.Context(), token, r.PathValue("id"), req.Amount)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, balanceResponse{balance})
}

// giftMarket is the market of collectibles, public.
func (a *api) giftMarket(w http.ResponseWriter, r *http.Request) {
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

	q := r.URL.Query()

	page, err := a.gifts.Market(r.Context(), token, model.MarketFilter{
		Gift:     q.Get("gift"),
		Model:    q.Get("model"),
		Backdrop: q.Get("backdrop"),
		Symbol:   q.Get("symbol"),
		Sort:     q.Get("sort"),
	}, q.Get("cursor"), limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, toCollectiblePage(page))
}

// giftAuctions are the running auctions, ending soonest first, public.
func (a *api) giftAuctions(w http.ResponseWriter, r *http.Request) {
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

	page, err := a.gifts.Auctions(r.Context(), token, r.URL.Query().Get("cursor"), limit)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, toCollectiblePage(page))
}

// mintGiftAuction is for the administrators of the site: an edition the
// site auctions.
func (a *api) mintGiftAuction(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req mintAuctionRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	id, err := a.gifts.MintAuction(r.Context(), token, model.GiftEdition(req))
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, struct {
		ID string `json:"id"`
	}{id})
}

// setGiftSupply is for the administrators of the site: how many of a gift
// may ever be sent, 0 for no end.
func (a *api) setGiftSupply(w http.ResponseWriter, r *http.Request) {
	token, err := bearerToken(r)
	if err != nil {
		a.writeError(w, r, err)
		return
	}

	var req giftSupplyRequest
	if err = decode(w, r, &req); err != nil {
		a.writeError(w, r, err)
		return
	}

	if err = a.gifts.SetSupply(r.Context(), token, r.PathValue("gift"), req.Supply); err != nil {
		a.writeError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
