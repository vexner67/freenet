package backend

import (
	"context"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/pkg/errs"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Gifts is rest.Gifts over the user service, which keeps the gifts people
// send each other.
type Gifts struct {
	api userv1.UserServiceClient
}

func NewGifts(api userv1.UserServiceClient) *Gifts {
	return &Gifts{
		api: api,
	}
}

func (g *Gifts) Catalog(ctx context.Context) (model.GiftCatalog, error) {
	resp, err := g.api.GetGiftCatalog(ctx, &userv1.GetGiftCatalogRequest{})
	if err != nil {
		return model.GiftCatalog{}, errs.Wrap("g.api.GetGiftCatalog", fromStatus(err))
	}

	catalog := model.GiftCatalog{
		Collections:   make([]model.GiftCollection, 0, len(resp.GetCollections())),
		Models:        toGiftTraits(resp.GetModels()),
		Backdrops:     toGiftTraits(resp.GetBackdrops()),
		Symbols:       toGiftTraits(resp.GetSymbols()),
		UpgradePrice:  resp.GetUpgradePrice(),
		TransferPrice: resp.GetTransferPrice(),
	}

	for _, c := range resp.GetCollections() {
		catalog.Collections = append(catalog.Collections, model.GiftCollection{
			ID:       c.GetId(),
			Gifts:    c.GetGifts(),
			Price:    c.GetPrice(),
			Prices:   c.GetPrices(),
			Prime:    c.GetPrime(),
			Supplies: toInts(c.GetSupplies()),
			Sold:     toInts(c.GetSold()),
		})
	}

	return catalog, nil
}

func toGiftTraits(traits []*userv1.GiftTrait) []model.GiftTrait {
	out := make([]model.GiftTrait, 0, len(traits))
	for _, t := range traits {
		out = append(out, model.GiftTrait{Name: t.GetName(), Weight: int(t.GetWeight())})
	}

	return out
}

func toInts(ns []int32) []int {
	out := make([]int, 0, len(ns))
	for _, n := range ns {
		out = append(out, int(n))
	}

	return out
}

func (g *Gifts) Send(ctx context.Context, accessToken, userID string, d model.GiftDraft) (model.Gift, error) {
	resp, err := g.api.SendGift(withToken(ctx, accessToken), &userv1.SendGiftRequest{
		UserId:    userID,
		Gift:      d.Gift,
		Caption:   d.Caption,
		Anonymous: d.Anonymous,
	})
	if err != nil {
		return model.Gift{}, errs.Wrap("g.api.SendGift", giftRefusal(err))
	}

	return toGift(resp.GetGift()), nil
}

// giftRefusal keeps what the user service says when it refuses a gift:
// the words are its own and tell nothing but that.
func giftRefusal(err error) error {
	if st, ok := status.FromError(err); ok && st.Code() == codes.PermissionDenied {
		return failure.New(failure.PermissionDenied, st.Message())
	}

	return fromStatus(err)
}

func (g *Gifts) List(ctx context.Context, accessToken, userID, cursor string, limit int) (model.GiftPage, error) {
	resp, err := g.api.ListGifts(withToken(ctx, accessToken), &userv1.ListGiftsRequest{
		UserId: userID,
		Cursor: cursor,
		Limit:  toLimit(limit),
	})
	if err != nil {
		return model.GiftPage{}, errs.Wrap("g.api.ListGifts", giftRefusal(err))
	}

	page := model.GiftPage{
		Gifts:      make([]model.Gift, 0, len(resp.GetGifts())),
		NextCursor: resp.GetNextCursor(),
		Total:      int(resp.GetTotal()),
	}

	for _, pg := range resp.GetGifts() {
		page.Gifts = append(page.Gifts, toGift(pg))
	}

	return page, nil
}

func (g *Gifts) Delete(ctx context.Context, accessToken, giftID string) error {
	if _, err := g.api.DeleteGift(withToken(ctx, accessToken), &userv1.DeleteGiftRequest{GiftId: giftID}); err != nil {
		return errs.Wrap("g.api.DeleteGift", fromStatus(err))
	}

	return nil
}

// SetPrice makes a gift cost price votes from now on; for the
// administrators of the site.
func (g *Gifts) SetPrice(ctx context.Context, accessToken, gift string, price int64) error {
	_, err := g.api.SetGiftPrice(withToken(ctx, accessToken), &userv1.SetGiftPriceRequest{Gift: gift, Price: price})
	if err != nil {
		return errs.Wrap("g.api.SetGiftPrice", fromStatus(err))
	}

	return nil
}

func toGift(pg *userv1.Gift) model.Gift {
	gift := model.Gift{
		ID:        pg.GetId(),
		Gift:      pg.GetGift(),
		Caption:   pg.GetCaption(),
		Anonymous: pg.GetAnonymous(),
		SentAt:    pg.GetSentAt().AsTime(),
	}

	if pg.GetSender() != nil {
		sender := toPublicProfile(pg.GetSender())
		gift.Sender = &sender
	}

	if pg.GetNumber() > 0 {
		gift.Number = int(pg.GetNumber())
		gift.Model = pg.GetModel()
		gift.Backdrop = pg.GetBackdrop()
		gift.Symbol = pg.GetSymbol()
		gift.SalePrice = pg.GetSalePrice()
		gift.Auctioned = pg.GetAuctioned()
	}

	return gift
}
