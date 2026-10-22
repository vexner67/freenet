package grpc_test

import (
	"context"
	"testing"

	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestCollectibles(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	admin := s.signIn(t, adminEmail)
	ann, annID := s.member(t, "ann")
	bob, _ := s.member(t, "bob")
	anonymous := context.Background()

	catalog, err := s.client.GetGiftCatalog(anonymous, &userv1.GetGiftCatalogRequest{})
	if err != nil || len(catalog.GetModels()) == 0 || catalog.GetUpgradePrice() == 0 || len(catalog.GetCollections()[0].GetSold()) != len(catalog.GetCollections()[0].GetGifts()) {
		t.Fatalf("GetGiftCatalog = %v, %v", catalog, err)
	}
	kind := catalog.GetCollections()[0].GetGifts()[0]

	// Votes for both: a gift, its upgrade and a purchase.
	voucher, err := s.client.CreateVoucher(admin, &userv1.CreateVoucherRequest{Votes: 300, Usages: 2})
	if err != nil {
		t.Fatalf("CreateVoucher: %v", err)
	}
	for _, who := range []context.Context{ann, bob} {
		if _, err = s.client.RedeemVoucher(who, &userv1.RedeemVoucherRequest{Token: voucher.GetVoucher().GetToken()}); err != nil {
			t.Fatalf("RedeemVoucher: %v", err)
		}
	}

	if _, err = s.client.SetGiftPrice(admin, &userv1.SetGiftPriceRequest{Gift: kind, Price: 0}); err != nil {
		t.Fatal(err)
	}
	sent, err := s.client.SendGift(ann, &userv1.SendGiftRequest{UserId: annID, Gift: kind})
	if err != nil {
		t.Fatalf("SendGift: %v", err)
	}
	id := sent.GetGift().GetId()

	if _, err = s.client.UpgradeGift(bob, &userv1.UpgradeGiftRequest{GiftId: id}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("UpgradeGift by another: %v", err)
	}
	up, err := s.client.UpgradeGift(ann, &userv1.UpgradeGiftRequest{GiftId: id})
	if err != nil || up.GetGift().GetNumber() != 1 || up.GetGift().GetModel() == "" {
		t.Fatalf("UpgradeGift = %v, %v", up, err)
	}
	if _, err = s.client.UpgradeGift(ann, &userv1.UpgradeGiftRequest{GiftId: id}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("UpgradeGift twice: %v", err)
	}

	if _, err = s.client.SetGiftSale(ann, &userv1.SetGiftSaleRequest{GiftId: id, Price: 100}); err != nil {
		t.Fatalf("SetGiftSale: %v", err)
	}

	market, err := s.client.ListGiftMarket(anonymous, &userv1.ListGiftMarketRequest{Gift: kind})
	if err != nil || len(market.GetCollectibles()) != 1 || market.GetCollectibles()[0].GetGift().GetSalePrice() != 100 || market.GetCollectibles()[0].GetOwner().GetNickname() != "ann" {
		t.Fatalf("ListGiftMarket = %v, %v", market, err)
	}
	if _, err = s.client.ListGiftMarket(anonymous, &userv1.ListGiftMarketRequest{Sort: "cheap"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("ListGiftMarket of an unknown order: %v", err)
	}

	_, err = s.client.BuyGift(bob, &userv1.BuyGiftRequest{GiftId: id, Price: 90})
	if st := status.Convert(err); st.Code() != codes.FailedPrecondition || st.Message() != "the price of the gift changed" {
		t.Fatalf("BuyGift for another price: %v", err)
	}
	bought, err := s.client.BuyGift(bob, &userv1.BuyGiftRequest{GiftId: id, Price: 100})
	if err != nil || bought.GetBalance() != 200 {
		t.Fatalf("BuyGift = %v, %v", bought, err)
	}

	got, err := s.client.GetCollectible(anonymous, &userv1.GetCollectibleRequest{GiftId: id})
	if err != nil || got.GetCollectible().GetOwner().GetNickname() != "bob" || len(got.GetCollectible().GetHistory()) != 2 ||
		got.GetCollectible().GetHistory()[0].GetKind() != "sale" {
		t.Fatalf("GetCollectible = %v, %v", got, err)
	}

	// An auction of bob's, bid on by ann.
	if _, err = s.client.StartGiftAuction(bob, &userv1.StartGiftAuctionRequest{GiftId: id, Start: 10, Hours: 0}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("StartGiftAuction for no time: %v", err)
	}
	if _, err = s.client.StartGiftAuction(bob, &userv1.StartGiftAuctionRequest{GiftId: id, Start: 10, Hours: 24}); err != nil {
		t.Fatalf("StartGiftAuction: %v", err)
	}
	if _, err = s.client.BidGift(ann, &userv1.BidGiftRequest{GiftId: id, Amount: 9}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("BidGift under the start: %v", err)
	}
	if _, err = s.client.BidGift(ann, &userv1.BidGiftRequest{GiftId: id, Amount: 10}); err != nil {
		t.Fatalf("BidGift: %v", err)
	}

	auctions, err := s.client.ListGiftAuctions(ann, &userv1.ListGiftAuctionsRequest{})
	if err != nil || len(auctions.GetCollectibles()) != 1 || !auctions.GetCollectibles()[0].GetAuction().GetMine() ||
		auctions.GetCollectibles()[0].GetAuction().GetMinBid() != 11 {
		t.Fatalf("ListGiftAuctions = %v, %v", auctions, err)
	}
	if _, err = s.client.CancelGiftAuction(bob, &userv1.CancelGiftAuctionRequest{GiftId: id}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("CancelGiftAuction with a bid: %v", err)
	}

	// The administrators' editions.
	if _, err = s.client.SetGiftSupply(ann, &userv1.SetGiftSupplyRequest{Gift: kind, Supply: 5}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("SetGiftSupply by ann: %v", err)
	}
	minted, err := s.client.MintGiftAuction(admin, &userv1.MintGiftAuctionRequest{Gift: kind, Model: "amethyst", Backdrop: "black", Symbol: "rocket", Start: 50, Hours: 1})
	if err != nil {
		t.Fatalf("MintGiftAuction: %v", err)
	}
	if site, err := s.client.GetCollectible(anonymous, &userv1.GetCollectibleRequest{GiftId: minted.GetGiftId()}); err != nil || !site.GetCollectible().GetSite() || !site.GetCollectible().GetAuction().GetSite() {
		t.Fatalf("the edition of the site = %v, %v", site, err)
	}
	if _, err = s.client.MintGiftAuction(admin, &userv1.MintGiftAuctionRequest{Gift: kind, Model: "gold", Backdrop: "black", Symbol: "rocket", Start: 50, Hours: 1}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("MintGiftAuction of an unknown model: %v", err)
	}
}
