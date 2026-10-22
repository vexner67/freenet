package backend

import (
	"context"

	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/pkg/errs"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
)

// Upgrade turns the caller's gift into a collectible.
func (g *Gifts) Upgrade(ctx context.Context, accessToken, giftID string) (model.Gift, error) {
	resp, err := g.api.UpgradeGift(withToken(ctx, accessToken), &userv1.UpgradeGiftRequest{GiftId: giftID})
	if err != nil {
		return model.Gift{}, errs.Wrap("g.api.UpgradeGift", giftRefusal(err))
	}

	return toGift(resp.GetGift()), nil
}

// Transfer gives the caller's collectible to userID.
func (g *Gifts) Transfer(ctx context.Context, accessToken, giftID, userID string) error {
	_, err := g.api.TransferGift(withToken(ctx, accessToken), &userv1.TransferGiftRequest{GiftId: giftID, UserId: userID})
	if err != nil {
		return errs.Wrap("g.api.TransferGift", giftRefusal(err))
	}

	return nil
}

// SetSale puts the caller's collectible up for price votes, 0 takes it
// off.
func (g *Gifts) SetSale(ctx context.Context, accessToken, giftID string, price int64) (model.Gift, error) {
	resp, err := g.api.SetGiftSale(withToken(ctx, accessToken), &userv1.SetGiftSaleRequest{GiftId: giftID, Price: price})
	if err != nil {
		return model.Gift{}, errs.Wrap("g.api.SetGiftSale", giftRefusal(err))
	}

	return toGift(resp.GetGift()), nil
}

// Buy buys a collectible for the price the caller was shown and says the
// votes left.
func (g *Gifts) Buy(ctx context.Context, accessToken, giftID string, price int64) (int64, error) {
	resp, err := g.api.BuyGift(withToken(ctx, accessToken), &userv1.BuyGiftRequest{GiftId: giftID, Price: price})
	if err != nil {
		return 0, errs.Wrap("g.api.BuyGift", giftRefusal(err))
	}

	return resp.GetBalance(), nil
}

// Collectible is one collectible with its auction and history.
func (g *Gifts) Collectible(ctx context.Context, accessToken, giftID string) (model.Collectible, error) {
	resp, err := g.api.GetCollectible(withToken(ctx, accessToken), &userv1.GetCollectibleRequest{GiftId: giftID})
	if err != nil {
		return model.Collectible{}, errs.Wrap("g.api.GetCollectible", giftRefusal(err))
	}

	return toCollectible(resp.GetCollectible()), nil
}

// Market pages the collectibles on sale.
func (g *Gifts) Market(ctx context.Context, accessToken string, f model.MarketFilter, cursor string, limit int) (model.CollectiblePage, error) {
	resp, err := g.api.ListGiftMarket(withToken(ctx, accessToken), &userv1.ListGiftMarketRequest{
		Gift:     f.Gift,
		Model:    f.Model,
		Backdrop: f.Backdrop,
		Symbol:   f.Symbol,
		Sort:     f.Sort,
		Cursor:   cursor,
		Limit:    toLimit(limit),
	})
	if err != nil {
		return model.CollectiblePage{}, errs.Wrap("g.api.ListGiftMarket", fromStatus(err))
	}

	return model.CollectiblePage{Collectibles: toCollectibles(resp.GetCollectibles()), NextCursor: resp.GetNextCursor()}, nil
}

// StartAuction puts the caller's collectible up for the highest bid.
func (g *Gifts) StartAuction(ctx context.Context, accessToken, giftID string, start int64, hours int) error {
	_, err := g.api.StartGiftAuction(withToken(ctx, accessToken), &userv1.StartGiftAuctionRequest{
		GiftId: giftID,
		Start:  start,
		Hours:  int32(hours), //nolint:gosec // the user service bounds the hours
	})
	if err != nil {
		return errs.Wrap("g.api.StartGiftAuction", giftRefusal(err))
	}

	return nil
}

// MintAuction is for the administrators of the site: an edition the site
// auctions. It says the ID of the new gift.
func (g *Gifts) MintAuction(ctx context.Context, accessToken string, e model.GiftEdition) (string, error) {
	resp, err := g.api.MintGiftAuction(withToken(ctx, accessToken), &userv1.MintGiftAuctionRequest{
		Gift:     e.Gift,
		Model:    e.Model,
		Backdrop: e.Backdrop,
		Symbol:   e.Symbol,
		Start:    e.Start,
		Hours:    int32(e.Hours), //nolint:gosec // the user service bounds the hours
	})
	if err != nil {
		return "", errs.Wrap("g.api.MintGiftAuction", fromStatus(err))
	}

	return resp.GetGiftId(), nil
}

// CancelAuction takes back an auction nobody bid on.
func (g *Gifts) CancelAuction(ctx context.Context, accessToken, giftID string) error {
	_, err := g.api.CancelGiftAuction(withToken(ctx, accessToken), &userv1.CancelGiftAuctionRequest{GiftId: giftID})
	if err != nil {
		return errs.Wrap("g.api.CancelGiftAuction", giftRefusal(err))
	}

	return nil
}

// Bid bids amount on a collectible at an auction and says the votes left.
func (g *Gifts) Bid(ctx context.Context, accessToken, giftID string, amount int64) (int64, error) {
	resp, err := g.api.BidGift(withToken(ctx, accessToken), &userv1.BidGiftRequest{GiftId: giftID, Amount: amount})
	if err != nil {
		return 0, errs.Wrap("g.api.BidGift", giftRefusal(err))
	}

	return resp.GetBalance(), nil
}

// Auctions pages the running auctions, ending soonest first.
func (g *Gifts) Auctions(ctx context.Context, accessToken, cursor string, limit int) (model.CollectiblePage, error) {
	resp, err := g.api.ListGiftAuctions(withToken(ctx, accessToken), &userv1.ListGiftAuctionsRequest{Cursor: cursor, Limit: toLimit(limit)})
	if err != nil {
		return model.CollectiblePage{}, errs.Wrap("g.api.ListGiftAuctions", fromStatus(err))
	}

	return model.CollectiblePage{Collectibles: toCollectibles(resp.GetCollectibles()), NextCursor: resp.GetNextCursor()}, nil
}

// SetSupply is for the administrators of the site: how many of a gift may
// ever be sent, 0 for no end.
func (g *Gifts) SetSupply(ctx context.Context, accessToken, gift string, supply int64) error {
	_, err := g.api.SetGiftSupply(withToken(ctx, accessToken), &userv1.SetGiftSupplyRequest{Gift: gift, Supply: supply})
	if err != nil {
		return errs.Wrap("g.api.SetGiftSupply", fromStatus(err))
	}

	return nil
}

func toCollectibles(list []*userv1.Collectible) []model.Collectible {
	out := make([]model.Collectible, 0, len(list))
	for _, c := range list {
		out = append(out, toCollectible(c))
	}

	return out
}

func toCollectible(c *userv1.Collectible) model.Collectible {
	out := model.Collectible{
		Gift:  toGift(c.GetGift()),
		Owner: profileOf(c.GetOwner()),
		Site:  c.GetSite(),
		Mine:  c.GetMine(),
	}

	if a := c.GetAuction(); a != nil {
		out.Auction = &model.GiftAuction{
			Start:  a.GetStart(),
			TopBid: a.GetTopBid(),
			MinBid: a.GetMinBid(),
			Bids:   int(a.GetBids()),
			EndsAt: a.GetEndsAt().AsTime(),
			Site:   a.GetSite(),
			Seller: profileOf(a.GetSeller()),
			Bidder: profileOf(a.GetBidder()),
			Mine:   a.GetMine(),
		}
	}

	for _, l := range c.GetHistory() {
		out.History = append(out.History, model.GiftHistoryLine{
			Kind:  l.GetKind(),
			At:    l.GetAt().AsTime(),
			Price: l.GetPrice(),
			From:  profileOf(l.GetFrom()),
			To:    profileOf(l.GetTo()),
		})
	}

	return out
}

func profileOf(p *userv1.PublicProfile) *model.PublicProfile {
	if p == nil {
		return nil
	}

	profile := toPublicProfile(p)

	return &profile
}
