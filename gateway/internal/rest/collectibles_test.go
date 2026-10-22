package rest_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

// fakeMarket records what the market was asked; its g7 is a collectible of
// ann's at an auction bob leads.
type fakeMarket struct {
	asked              string
	giftID, to         string
	amount             int64
	hours              int
	filter             model.MarketFilter
	edition            model.GiftEdition
	marketErr          error
	marketCursor       string
	marketLimit        int
	supplied           string
	supply             int64
	cancelled, upgrade string
}

func collectibleG7() model.Collectible {
	ann := model.PublicProfile{ID: "u1", Profile: model.Profile{Name: "Ann", Nickname: "ann"}}
	bob := model.PublicProfile{ID: "u2", Profile: model.Profile{Name: "Bob", Nickname: "bob"}}

	return model.Collectible{
		Gift:  model.Gift{ID: "g7", Gift: "freenet_coin", SentAt: giftTime, Number: 7, Model: "onyx", Backdrop: "gold", Symbol: "star", Auctioned: true},
		Owner: &ann,
		Auction: &model.GiftAuction{
			Start: 10, TopBid: 20, MinBid: 21, Bids: 2, EndsAt: giftTime.Add(time.Hour), Seller: &ann, Bidder: &bob,
		},
		History: []model.GiftHistoryLine{{Kind: "upgrade", At: giftTime, To: &ann}},
	}
}

func (f *fakeMarket) Upgrade(_ context.Context, _, giftID string) (model.Gift, error) {
	f.upgrade = giftID
	return model.Gift{ID: giftID, Gift: "freenet_coin", SentAt: giftTime, Number: 1, Model: "classic", Backdrop: "sky", Symbol: "f"}, f.marketErr
}

func (f *fakeMarket) Transfer(_ context.Context, _, giftID, userID string) error {
	f.asked, f.giftID, f.to = "transfer", giftID, userID
	return f.marketErr
}

func (f *fakeMarket) SetSale(_ context.Context, _, giftID string, price int64) (model.Gift, error) {
	f.asked, f.giftID, f.amount = "sale", giftID, price
	return model.Gift{ID: giftID, SentAt: giftTime, Number: 1, SalePrice: price}, f.marketErr
}

func (f *fakeMarket) Buy(_ context.Context, _, giftID string, price int64) (int64, error) {
	f.asked, f.giftID, f.amount = "buy", giftID, price
	return 77, f.marketErr
}

func (f *fakeMarket) Collectible(_ context.Context, _, giftID string) (model.Collectible, error) {
	f.giftID = giftID
	return collectibleG7(), f.marketErr
}

func (f *fakeMarket) Market(_ context.Context, _ string, filter model.MarketFilter, cursor string, limit int) (model.CollectiblePage, error) {
	f.filter, f.marketCursor, f.marketLimit = filter, cursor, limit
	return model.CollectiblePage{Collectibles: []model.Collectible{collectibleG7()}, NextCursor: "next"}, f.marketErr
}

func (f *fakeMarket) StartAuction(_ context.Context, _, giftID string, start int64, hours int) error {
	f.asked, f.giftID, f.amount, f.hours = "auction", giftID, start, hours
	return f.marketErr
}

func (f *fakeMarket) MintAuction(_ context.Context, _ string, e model.GiftEdition) (string, error) {
	f.edition = e
	return "g8", f.marketErr
}

func (f *fakeMarket) CancelAuction(_ context.Context, _, giftID string) error {
	f.cancelled = giftID
	return f.marketErr
}

func (f *fakeMarket) Bid(_ context.Context, _, giftID string, amount int64) (int64, error) {
	f.asked, f.giftID, f.amount = "bid", giftID, amount
	return 55, f.marketErr
}

func (f *fakeMarket) Auctions(_ context.Context, _, cursor string, limit int) (model.CollectiblePage, error) {
	f.marketCursor, f.marketLimit = cursor, limit
	return model.CollectiblePage{Collectibles: []model.Collectible{collectibleG7()}}, f.marketErr
}

func (f *fakeMarket) SetSupply(_ context.Context, _, gift string, supply int64) error {
	f.supplied, f.supply = gift, supply
	return f.marketErr
}

type collectibleBody struct {
	Gift struct {
		ID        string `json:"id"`
		Number    int    `json:"number"`
		Model     string `json:"model"`
		SalePrice int64  `json:"salePrice"`
		Auctioned bool   `json:"auctioned"`
	} `json:"gift"`
	Owner   map[string]any `json:"owner"`
	Site    bool           `json:"site"`
	Auction *struct {
		MinBid int64          `json:"minBid"`
		EndsAt string         `json:"endsAt"`
		Bidder map[string]any `json:"bidder"`
	} `json:"auction"`
	History []struct {
		Kind string         `json:"kind"`
		From map[string]any `json:"from"`
		To   map[string]any `json:"to"`
	} `json:"history"`
}

func TestCollectible(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("GET", "/api/v1/gifts/g7", "")
	got := decode[collectibleBody](t, w)
	if w.Code != http.StatusOK || h.gifts.giftID != "g7" || got.Gift.Number != 7 || got.Gift.Model != "onyx" || !got.Gift.Auctioned ||
		got.Owner["nickname"] != "ann" || got.Auction == nil || got.Auction.MinBid != 21 || got.Auction.Bidder["nickname"] != "bob" ||
		got.Auction.EndsAt != "2026-10-12T10:30:00Z" || len(got.History) != 1 || got.History[0].From != nil || got.History[0].To["nickname"] != "ann" {
		t.Fatalf("%d %+v", w.Code, got)
	}

	h.gifts.marketErr = failure.New(failure.PermissionDenied, "this user's gifts are hidden")
	if w = h.do("GET", "/api/v1/gifts/g7", ""); w.Code != http.StatusForbidden {
		t.Fatalf("hidden: %d", w.Code)
	}
}

func TestGiftMarket(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("GET", "/api/v1/market?gift=freenet_coin&model=onyx&sort=number&cursor=c&limit=5", "")
	got := decode[struct {
		Collectibles []collectibleBody `json:"collectibles"`
		NextCursor   string            `json:"nextCursor"`
	}](t, w)
	if w.Code != http.StatusOK || len(got.Collectibles) != 1 || got.NextCursor != "next" ||
		h.gifts.filter != (model.MarketFilter{Gift: "freenet_coin", Model: "onyx", Sort: "number"}) || h.gifts.marketCursor != "c" || h.gifts.marketLimit != 5 {
		t.Fatalf("%d %+v, asked %+v", w.Code, got, h.gifts.filter)
	}

	if w = h.do("GET", "/api/v1/auctions?cursor=a", ""); w.Code != http.StatusOK || h.gifts.marketCursor != "a" {
		t.Fatalf("auctions: %d", w.Code)
	}
}

func TestGiftTrade(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	auth := []string{"Authorization", "Bearer tok"}

	if w := h.do("POST", "/api/v1/gifts/g1/upgrade", "", auth...); w.Code != http.StatusOK || h.gifts.upgrade != "g1" || decode[struct {
		Number int `json:"number"`
	}](t, w).Number != 1 {
		t.Fatalf("upgrade: %d", w.Code)
	}

	if w := h.do("PUT", "/api/v1/gifts/g1/sale", `{"price":40}`, auth...); w.Code != http.StatusOK || h.gifts.asked != "sale" || h.gifts.amount != 40 {
		t.Fatalf("sale: %d %+v", w.Code, h.gifts.fakeMarket)
	}
	if w := h.do("DELETE", "/api/v1/gifts/g1/sale", "", auth...); w.Code != http.StatusOK || h.gifts.amount != 0 {
		t.Fatalf("off sale: %d", w.Code)
	}

	w := h.do("POST", "/api/v1/gifts/g1/purchase", `{"price":40}`, auth...)
	if w.Code != http.StatusOK || decode[struct {
		Balance int64 `json:"balance"`
	}](t, w).Balance != 77 || h.gifts.asked != "buy" || h.gifts.amount != 40 {
		t.Fatalf("buy: %d %s", w.Code, w.Body)
	}

	if w = h.do("POST", "/api/v1/gifts/g1/auction", `{"start":5,"hours":24}`, auth...); w.Code != http.StatusNoContent || h.gifts.hours != 24 || h.gifts.amount != 5 {
		t.Fatalf("auction: %d", w.Code)
	}
	if w = h.do("POST", "/api/v1/gifts/g1/bids", `{"amount":9}`, auth...); w.Code != http.StatusOK || h.gifts.asked != "bid" || h.gifts.amount != 9 {
		t.Fatalf("bid: %d", w.Code)
	}
	if w = h.do("DELETE", "/api/v1/gifts/g1/auction", "", auth...); w.Code != http.StatusNoContent || h.gifts.cancelled != "g1" {
		t.Fatalf("cancel: %d", w.Code)
	}

	if w = h.do("POST", "/api/v1/auctions", `{"gift":"freenet_coin","model":"onyx","backdrop":"gold","symbol":"star","start":50,"hours":2}`, auth...); w.Code != http.StatusCreated ||
		h.gifts.edition != (model.GiftEdition{Gift: "freenet_coin", Model: "onyx", Backdrop: "gold", Symbol: "star", Start: 50, Hours: 2}) {
		t.Fatalf("mint: %d %+v", w.Code, h.gifts.edition)
	}
	if w = h.do("PUT", "/api/v1/gifts/freenet_coin/supply", `{"supply":100}`, auth...); w.Code != http.StatusNoContent || h.gifts.supplied != "freenet_coin" || h.gifts.supply != 100 {
		t.Fatalf("supply: %d", w.Code)
	}

	// Without a token nothing changes hands.
	if w = h.do("POST", "/api/v1/gifts/g1/bids", `{"amount":9}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("bid without a token: %d", w.Code)
	}

	// A gift outbid meanwhile.
	h.gifts.marketErr = failure.New(failure.Conflict, "the bid is too low")
	if w = h.do("POST", "/api/v1/gifts/g1/bids", `{"amount":9}`, auth...); w.Code != http.StatusConflict || decode[errorBody](t, w).Error.Message != "the bid is too low" {
		t.Fatalf("low bid: %d %s", w.Code, w.Body)
	}
}

func TestTransferGift(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	if w := h.do("POST", "/api/v1/gifts/g1/transfer", `{"nickname":"bob"}`, "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest || h.gifts.asked != "" {
		t.Fatalf("without a profile: %d", w.Code)
	}

	h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}
	if w := h.do("POST", "/api/v1/gifts/g1/transfer", `{"nickname":"bob"}`, "Authorization", "Bearer tok"); w.Code != http.StatusNoContent || h.gifts.to != "u2" || h.gifts.giftID != "g1" {
		t.Fatalf("transfer: %d %+v", w.Code, h.gifts.fakeMarket)
	}
	if w := h.do("POST", "/api/v1/gifts/g1/transfer", `{"nickname":"nobody"}`, "Authorization", "Bearer tok"); w.Code != http.StatusNotFound {
		t.Fatalf("to nobody: %d", w.Code)
	}
}
