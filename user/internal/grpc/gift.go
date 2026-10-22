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

type GiftService interface {
	Send(ctx context.Context, senderID, recipientID uuid.UUID, d service.GiftDraft) (service.SentGift, error)
	Delete(ctx context.Context, userID, giftID uuid.UUID) error
	List(ctx context.Context, viewerID, ownerID, cursor uuid.UUID, size int) (service.GiftPage, error)
	Catalog(ctx context.Context) ([]service.GiftShelf, error)
	SetPrice(ctx context.Context, adminID uuid.UUID, kind user.GiftKind, price user.GiftPrice) error
	Upgrade(ctx context.Context, ownerID, giftID uuid.UUID) (service.SentGift, error)
	Transfer(ctx context.Context, ownerID, toID, giftID uuid.UUID) error
	SetSale(ctx context.Context, ownerID, giftID uuid.UUID, price int64) (service.SentGift, error)
	Buy(ctx context.Context, buyerID, giftID uuid.UUID, price int64) (int64, error)
	Get(ctx context.Context, viewerID, giftID uuid.UUID) (service.Collectible, error)
	Market(ctx context.Context, viewerID uuid.UUID, f user.MarketFilter, cursor string, size int) (service.CollectiblePage, error)
	StartAuction(ctx context.Context, ownerID, giftID uuid.UUID, start int64, hours user.AuctionHours) error
	MintAuction(ctx context.Context, adminID uuid.UUID, kind user.GiftKind, traits user.GiftTraits, start int64, hours user.AuctionHours) (uuid.UUID, error)
	CancelAuction(ctx context.Context, userID, giftID uuid.UUID) error
	Bid(ctx context.Context, bidderID, giftID uuid.UUID, amount int64) (int64, error)
	Auctions(ctx context.Context, viewerID uuid.UUID, cursor string, size int) (service.CollectiblePage, error)
	SetSupply(ctx context.Context, adminID uuid.UUID, kind user.GiftKind, supply user.GiftSupply) error
}

func (h *UserHandler) GetGiftCatalog(ctx context.Context, _ *userv1.GetGiftCatalogRequest) (*userv1.GetGiftCatalogResponse, error) {
	catalog, err := h.giftService.Catalog(ctx)
	if err != nil {
		return nil, err
	}

	resp := &userv1.GetGiftCatalogResponse{
		Collections:   make([]*userv1.GiftCollection, 0, len(catalog)),
		Models:        toProtoTraits(user.GiftModels()),
		Backdrops:     toProtoTraits(user.GiftBackdrops()),
		Symbols:       toProtoTraits(user.GiftSymbols()),
		UpgradePrice:  user.UpgradePrice,
		TransferPrice: user.GiftTransferPrice,
	}
	for _, c := range catalog {
		collection := &userv1.GiftCollection{
			Id:       c.ID,
			Gifts:    make([]string, 0, len(c.Gifts)),
			Price:    c.Price.Int64(),
			Prices:   make([]int64, 0, len(c.Gifts)),
			Prime:    c.Prime,
			Supplies: make([]int32, 0, len(c.Gifts)),
			Sold:     make([]int32, 0, len(c.Gifts)),
		}

		for _, g := range c.Gifts {
			collection.Gifts = append(collection.Gifts, g.Kind.String())
			collection.Prices = append(collection.Prices, g.Price.Int64())
			collection.Supplies = append(collection.Supplies, int32(g.Stock.Supply.Int())) //nolint:gosec // editions are up to a million
			collection.Sold = append(collection.Sold, int32(g.Stock.Sold))                 //nolint:gosec // as many as sent
		}

		resp.Collections = append(resp.Collections, collection)
	}

	return resp, nil
}

func (h *UserHandler) SetGiftPrice(ctx context.Context, req *userv1.SetGiftPriceRequest) (*emptypb.Empty, error) {
	adminID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	kind, err := user.NewGiftKind(req.GetGift())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	price, err := user.NewGiftPrice(req.GetPrice())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if err = h.giftService.SetPrice(ctx, adminID, kind, price); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *UserHandler) SendGift(ctx context.Context, req *userv1.SendGiftRequest) (*userv1.SendGiftResponse, error) {
	senderID, recipientID, err := callerAnd(ctx, req.GetUserId())
	if err != nil {
		return nil, err
	}

	kind, err := user.NewGiftKind(req.GetGift())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	caption, err := user.NewGiftCaption(req.GetCaption())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	sent, err := h.giftService.Send(ctx, senderID, recipientID, service.GiftDraft{
		Kind:      kind,
		Caption:   caption,
		Anonymous: req.GetAnonymous(),
	})
	if err != nil {
		return nil, err
	}

	return &userv1.SendGiftResponse{Gift: toProtoGift(sent)}, nil
}

func (h *UserHandler) ListGifts(ctx context.Context, req *userv1.ListGiftsRequest) (*userv1.ListGiftsResponse, error) {
	ownerID, err := parseID(req.GetUserId(), "user id")
	if err != nil {
		return nil, err
	}

	var cursor uuid.UUID
	if req.GetCursor() != "" {
		if cursor, err = parseID(req.GetCursor(), "cursor"); err != nil {
			return nil, err
		}
	}

	page, err := h.giftService.List(ctx, viewerID(ctx), ownerID, cursor, int(req.GetLimit()))
	if err != nil {
		return nil, err
	}

	resp := &userv1.ListGiftsResponse{
		Gifts: make([]*userv1.Gift, 0, len(page.Gifts)),
		Total: int32(page.Total), //nolint:gosec // counts of gifts fit in int32
	}

	for _, g := range page.Gifts {
		resp.Gifts = append(resp.Gifts, toProtoGift(g))
	}

	if page.NextCursor != uuid.Nil {
		resp.NextCursor = page.NextCursor.String()
	}

	return resp, nil
}

func (h *UserHandler) DeleteGift(ctx context.Context, req *userv1.DeleteGiftRequest) (*emptypb.Empty, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	giftID, err := parseID(req.GetGiftId(), "gift id")
	if err != nil {
		return nil, err
	}

	if err = h.giftService.Delete(ctx, userID, giftID); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func toProtoGift(g service.SentGift) *userv1.Gift {
	gift := &userv1.Gift{
		Id:        g.ID.String(),
		Gift:      g.Kind.String(),
		Caption:   g.Caption.String(),
		Anonymous: g.Anonymous,
		SentAt:    timestamppb.New(g.SentAt),
	}

	if g.Sender != nil {
		gift.Sender = toProtoPublicProfile(*g.Sender)
	}

	if g.Number > 0 {
		gift.Number = int32(g.Number) //nolint:gosec // numbers of an edition fit in int32
		gift.Model = g.Traits.Model
		gift.Backdrop = g.Traits.Backdrop
		gift.Symbol = g.Traits.Symbol
		gift.SalePrice = g.SalePrice
		gift.Auctioned = g.Auctioned
	}

	return gift
}

func toProtoTraits(traits []user.GiftTrait) []*userv1.GiftTrait {
	out := make([]*userv1.GiftTrait, 0, len(traits))
	for _, t := range traits {
		out = append(out, &userv1.GiftTrait{Name: t.Name, Weight: int32(t.Weight)}) //nolint:gosec // weights are up to 1000
	}

	return out
}
