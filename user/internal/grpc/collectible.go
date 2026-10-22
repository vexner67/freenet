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
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (h *UserHandler) UpgradeGift(ctx context.Context, req *userv1.UpgradeGiftRequest) (*userv1.UpgradeGiftResponse, error) {
	ownerID, giftID, err := callerAndGift(ctx, req.GetGiftId())
	if err != nil {
		return nil, err
	}

	g, err := h.giftService.Upgrade(ctx, ownerID, giftID)
	if err != nil {
		return nil, err
	}

	return &userv1.UpgradeGiftResponse{Gift: toProtoGift(g)}, nil
}

func (h *UserHandler) TransferGift(ctx context.Context, req *userv1.TransferGiftRequest) (*emptypb.Empty, error) {
	ownerID, giftID, err := callerAndGift(ctx, req.GetGiftId())
	if err != nil {
		return nil, err
	}

	toID, err := parseID(req.GetUserId(), "user id")
	if err != nil {
		return nil, err
	}

	if err = h.giftService.Transfer(ctx, ownerID, toID, giftID); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *UserHandler) SetGiftSale(ctx context.Context, req *userv1.SetGiftSaleRequest) (*userv1.SetGiftSaleResponse, error) {
	ownerID, giftID, err := callerAndGift(ctx, req.GetGiftId())
	if err != nil {
		return nil, err
	}

	g, err := h.giftService.SetSale(ctx, ownerID, giftID, req.GetPrice())
	if err != nil {
		return nil, err
	}

	return &userv1.SetGiftSaleResponse{Gift: toProtoGift(g)}, nil
}

func (h *UserHandler) BuyGift(ctx context.Context, req *userv1.BuyGiftRequest) (*userv1.BuyGiftResponse, error) {
	buyerID, giftID, err := callerAndGift(ctx, req.GetGiftId())
	if err != nil {
		return nil, err
	}

	balance, err := h.giftService.Buy(ctx, buyerID, giftID, req.GetPrice())
	if err != nil {
		return nil, err
	}

	return &userv1.BuyGiftResponse{Balance: balance}, nil
}

func (h *UserHandler) GetCollectible(ctx context.Context, req *userv1.GetCollectibleRequest) (*userv1.GetCollectibleResponse, error) {
	giftID, err := parseID(req.GetGiftId(), "gift id")
	if err != nil {
		return nil, err
	}

	c, err := h.giftService.Get(ctx, viewerID(ctx), giftID)
	if err != nil {
		return nil, err
	}

	return &userv1.GetCollectibleResponse{Collectible: toProtoCollectible(c)}, nil
}

func (h *UserHandler) ListGiftMarket(ctx context.Context, req *userv1.ListGiftMarketRequest) (*userv1.ListGiftMarketResponse, error) {
	f, err := marketFilter(req)
	if err != nil {
		return nil, err
	}

	page, err := h.giftService.Market(ctx, viewerID(ctx), f, req.GetCursor(), int(req.GetLimit()))
	if err != nil {
		return nil, err
	}

	return &userv1.ListGiftMarketResponse{Collectibles: toProtoCollectibles(page.Items), NextCursor: page.NextCursor}, nil
}

func marketFilter(req *userv1.ListGiftMarketRequest) (user.MarketFilter, error) {
	var (
		f   user.MarketFilter
		err error
	)

	if req.GetGift() != "" {
		if f.Kind, err = user.NewGiftKind(req.GetGift()); err != nil {
			return f, status.Error(codes.InvalidArgument, err.Error())
		}
	}

	for _, t := range []struct {
		list []user.GiftTrait
		name string
		to   *string
	}{
		{user.GiftModels(), req.GetModel(), &f.Model},
		{user.GiftBackdrops(), req.GetBackdrop(), &f.Backdrop},
		{user.GiftSymbols(), req.GetSymbol(), &f.Symbol},
	} {
		if *t.to, err = user.NewTraitFilter(t.list, t.name); err != nil {
			return f, status.Error(codes.InvalidArgument, err.Error())
		}
	}

	if f.Sort, err = user.NewMarketSort(req.GetSort()); err != nil {
		return f, status.Error(codes.InvalidArgument, err.Error())
	}

	return f, nil
}

func (h *UserHandler) StartGiftAuction(ctx context.Context, req *userv1.StartGiftAuctionRequest) (*emptypb.Empty, error) {
	ownerID, giftID, err := callerAndGift(ctx, req.GetGiftId())
	if err != nil {
		return nil, err
	}

	hours, err := user.NewAuctionHours(int(req.GetHours()))
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if err = h.giftService.StartAuction(ctx, ownerID, giftID, req.GetStart(), hours); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *UserHandler) MintGiftAuction(ctx context.Context, req *userv1.MintGiftAuctionRequest) (*userv1.MintGiftAuctionResponse, error) {
	adminID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	kind, err := user.NewGiftKind(req.GetGift())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	traits, err := user.NewGiftTraits(req.GetModel(), req.GetBackdrop(), req.GetSymbol())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	hours, err := user.NewAuctionHours(int(req.GetHours()))
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	id, err := h.giftService.MintAuction(ctx, adminID, kind, traits, req.GetStart(), hours)
	if err != nil {
		return nil, err
	}

	return &userv1.MintGiftAuctionResponse{GiftId: id.String()}, nil
}

func (h *UserHandler) CancelGiftAuction(ctx context.Context, req *userv1.CancelGiftAuctionRequest) (*emptypb.Empty, error) {
	userID, giftID, err := callerAndGift(ctx, req.GetGiftId())
	if err != nil {
		return nil, err
	}

	if err = h.giftService.CancelAuction(ctx, userID, giftID); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *UserHandler) BidGift(ctx context.Context, req *userv1.BidGiftRequest) (*userv1.BidGiftResponse, error) {
	bidderID, giftID, err := callerAndGift(ctx, req.GetGiftId())
	if err != nil {
		return nil, err
	}

	balance, err := h.giftService.Bid(ctx, bidderID, giftID, req.GetAmount())
	if err != nil {
		return nil, err
	}

	return &userv1.BidGiftResponse{Balance: balance}, nil
}

func (h *UserHandler) ListGiftAuctions(ctx context.Context, req *userv1.ListGiftAuctionsRequest) (*userv1.ListGiftAuctionsResponse, error) {
	page, err := h.giftService.Auctions(ctx, viewerID(ctx), req.GetCursor(), int(req.GetLimit()))
	if err != nil {
		return nil, err
	}

	return &userv1.ListGiftAuctionsResponse{Collectibles: toProtoCollectibles(page.Items), NextCursor: page.NextCursor}, nil
}

func (h *UserHandler) SetGiftSupply(ctx context.Context, req *userv1.SetGiftSupplyRequest) (*emptypb.Empty, error) {
	adminID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	kind, err := user.NewGiftKind(req.GetGift())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	supply, err := user.NewGiftSupply(req.GetSupply())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if err = h.giftService.SetSupply(ctx, adminID, kind, supply); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func callerAndGift(ctx context.Context, giftID string) (uuid.UUID, uuid.UUID, error) {
	callerID, err := currentUserID(ctx)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	id, err := parseID(giftID, "gift id")
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	return callerID, id, nil
}

func toProtoCollectibles(items []service.Collectible) []*userv1.Collectible {
	out := make([]*userv1.Collectible, 0, len(items))
	for _, c := range items {
		out = append(out, toProtoCollectible(c))
	}

	return out
}

func toProtoCollectible(c service.Collectible) *userv1.Collectible {
	out := &userv1.Collectible{
		Gift: toProtoGift(c.Gift),
		Site: c.Site,
		Mine: c.Mine,
	}

	if c.Owner != nil {
		out.Owner = toProtoPublicProfile(*c.Owner)
	}

	if a := c.Auction; a != nil {
		out.Auction = &userv1.GiftAuction{
			Start:  a.Start,
			TopBid: a.TopBid,
			MinBid: a.MinBid,
			Bids:   int32(a.Bids), //nolint:gosec // counts of bids fit in int32
			EndsAt: timestamppb.New(a.EndsAt),
			Site:   a.Site,
			Seller: profileOrNil(a.Seller),
			Bidder: profileOrNil(a.Bidder),
			Mine:   a.Mine,
		}
	}

	for _, l := range c.History {
		out.History = append(out.History, &userv1.GiftHistoryLine{
			Kind:  string(l.Kind),
			At:    timestamppb.New(l.At),
			Price: l.Price,
			From:  profileOrNil(l.From),
			To:    profileOrNil(l.To),
		})
	}

	return out
}

func profileOrNil(p *user.PublicProfile) *userv1.PublicProfile {
	if p == nil {
		return nil
	}

	return toProtoPublicProfile(*p)
}
