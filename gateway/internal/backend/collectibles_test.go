package backend

import (
	"context"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type fakeMarketAPI struct {
	userv1.UserServiceClient
	market *userv1.ListGiftMarketRequest
	bid    *userv1.BidGiftRequest
	err    error
}

func (f *fakeMarketAPI) ListGiftMarket(_ context.Context, req *userv1.ListGiftMarketRequest, _ ...grpc.CallOption) (*userv1.ListGiftMarketResponse, error) {
	f.market = req

	return &userv1.ListGiftMarketResponse{
		Collectibles: []*userv1.Collectible{{
			Gift:  &userv1.Gift{Id: "g7", Gift: "freenet_coin", SentAt: timestamppb.New(giftSentAt), Number: 7, Model: "onyx", Backdrop: "gold", Symbol: "star", SalePrice: 40},
			Owner: &userv1.PublicProfile{Id: "u1", Nickname: "ann"},
			Auction: &userv1.GiftAuction{
				Start: 10, TopBid: 20, MinBid: 21, Bids: 2, EndsAt: timestamppb.New(giftSentAt), Bidder: &userv1.PublicProfile{Id: "u2", Nickname: "bob"},
			},
			History: []*userv1.GiftHistoryLine{{Kind: "sale", At: timestamppb.New(giftSentAt), Price: 40, To: &userv1.PublicProfile{Id: "u1"}}},
		}},
		NextCursor: "next",
	}, f.err
}

func (f *fakeMarketAPI) BidGift(_ context.Context, req *userv1.BidGiftRequest, _ ...grpc.CallOption) (*userv1.BidGiftResponse, error) {
	f.bid = req
	return &userv1.BidGiftResponse{Balance: 12}, f.err
}

func TestGiftMarket(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	api := &fakeMarketAPI{}
	gifts := NewGifts(api)

	page, err := gifts.Market(ctx, "", model.MarketFilter{Gift: "freenet_coin", Model: "onyx", Sort: "number"}, "c", 5)
	if err != nil || api.market.GetGift() != "freenet_coin" || api.market.GetModel() != "onyx" || api.market.GetSort() != "number" ||
		api.market.GetCursor() != "c" || api.market.GetLimit() != 5 {
		t.Fatalf("asked %v, %v", api.market, err)
	}

	c := page.Collectibles[0]
	if page.NextCursor != "next" || c.Gift.Number != 7 || c.Gift.SalePrice != 40 || c.Owner.Profile.Nickname != "ann" || c.Auction.MinBid != 21 ||
		c.Auction.Bidder.ID != "u2" || c.Auction.Seller != nil || len(c.History) != 1 || c.History[0].From != nil || c.History[0].To.ID != "u1" {
		t.Fatalf("Market = %+v", page)
	}

	balance, err := gifts.Bid(ctx, "tok", "g7", 21)
	if err != nil || balance != 12 || api.bid.GetGiftId() != "g7" || api.bid.GetAmount() != 21 {
		t.Fatalf("Bid = %d, %v", balance, err)
	}

	// The refusals keep the words of the user service.
	api.err = status.Error(codes.PermissionDenied, "only the owner of a gift can do this")
	if _, err = gifts.Bid(ctx, "tok", "g7", 21); failure.From(err).Message != "only the owner of a gift can do this" {
		t.Fatalf("refused: %v", err)
	}
}
