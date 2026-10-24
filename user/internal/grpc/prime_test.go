package grpc_test

import (
	"context"
	"testing"

	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestPrime(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	admin := s.signIn(t, adminEmail)
	ann, _ := s.member(t, "ann")
	bob, bobID := s.member(t, "bob")

	// Anyone asks what Prime costs; the administrators price it.
	if got, err := s.client.GetPrime(context.Background(), &userv1.GetPrimeRequest{}); err != nil || got.GetPrices().GetMonth() != 150 {
		t.Fatalf("GetPrime = %v, %v", got, err)
	}
	prices := &userv1.PrimePrices{Month: 100, Quarter: 250, Year: 900}
	if _, err := s.client.SetPrimePrices(ann, &userv1.SetPrimePricesRequest{Prices: prices}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("SetPrimePrices by ann: %v", err)
	}
	if _, err := s.client.SetPrimePrices(admin, &userv1.SetPrimePricesRequest{Prices: &userv1.PrimePrices{Month: 0, Quarter: 1, Year: 1}}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("SetPrimePrices of 0: %v", err)
	}
	if _, err := s.client.SetPrimePrices(admin, &userv1.SetPrimePricesRequest{Prices: prices}); err != nil {
		t.Fatalf("SetPrimePrices: %v", err)
	}

	if _, err := s.client.BuyPrime(ann, &userv1.BuyPrimeRequest{Period: "week"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("BuyPrime of a week: %v", err)
	}
	if _, err := s.client.BuyPrime(ann, &userv1.BuyPrimeRequest{Period: "month"}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("BuyPrime without votes: %v", err)
	}
	// Hiding takes Prime.
	if _, err := s.client.SetPrimeInvisible(ann, &userv1.SetPrimeInvisibleRequest{Invisible: true}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("SetPrimeInvisible without Prime: %v", err)
	}
	// So does dressing the page, but not undressing it.
	stars := &userv1.PrimeStyle{Name: "rainbow", Frame: "stars", Background: "hearts", Mood: "love"}
	if _, err := s.client.SetPrimeStyle(ann, &userv1.SetPrimeStyleRequest{Style: stars}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("SetPrimeStyle without Prime: %v", err)
	}
	if _, err := s.client.SetPrimeStyle(ann, &userv1.SetPrimeStyleRequest{Style: &userv1.PrimeStyle{}}); err != nil {
		t.Fatalf("SetPrimeStyle back to the default: %v", err)
	}

	voucher, err := s.client.CreateVoucher(admin, &userv1.CreateVoucherRequest{Votes: 300, Usages: 1})
	if err != nil {
		t.Fatalf("CreateVoucher: %v", err)
	}
	if _, err = s.client.RedeemVoucher(ann, &userv1.RedeemVoucherRequest{Token: voucher.GetVoucher().GetToken()}); err != nil {
		t.Fatalf("RedeemVoucher: %v", err)
	}

	bought, err := s.client.BuyPrime(ann, &userv1.BuyPrimeRequest{Period: "month"})
	if err != nil || bought.GetBalance() != 200 || bought.GetUntil() == nil {
		t.Fatalf("BuyPrime = %v, %v", bought, err)
	}
	me, err := s.client.GetMe(ann, &userv1.GetMeRequest{})
	if err != nil || !me.GetUser().GetPrimeUntil().AsTime().Equal(bought.GetUntil().AsTime()) {
		t.Fatalf("GetMe after = %v, %v", me.GetUser(), err)
	}
	if _, err = s.client.SetPrimeInvisible(ann, &userv1.SetPrimeInvisibleRequest{Invisible: true}); err != nil {
		t.Fatalf("SetPrimeInvisible: %v", err)
	}
	if me, err = s.client.GetMe(ann, &userv1.GetMeRequest{}); err != nil || !me.GetUser().GetPrimeInvisible() {
		t.Fatalf("GetMe invisible = %v, %v", me.GetUser(), err)
	}
	if _, err = s.client.SetPrimeStyle(ann, &userv1.SetPrimeStyleRequest{Style: &userv1.PrimeStyle{Name: "pink"}}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("SetPrimeStyle pink: %v", err)
	}
	if _, err = s.client.SetPrimeStyle(ann, &userv1.SetPrimeStyleRequest{Style: stars}); err != nil {
		t.Fatalf("SetPrimeStyle: %v", err)
	}
	if me, err = s.client.GetMe(ann, &userv1.GetMeRequest{}); err != nil || me.GetUser().GetPrimeStyle().GetFrame() != "stars" {
		t.Fatalf("GetMe dressed = %v, %v", me.GetUser(), err)
	}
	page, err := s.client.GetProfile(context.Background(), &userv1.GetProfileRequest{Nickname: "ann"})
	if err != nil || page.GetProfile().GetPrimeStyle().GetMood() != "love" || page.GetProfile().GetPrimeStyle().GetName() != "rainbow" {
		t.Fatalf("ann's dressed page = %v, %v", page.GetProfile(), err)
	}

	// A present: bob has it and everybody sees the badge.
	if bought, err = s.client.BuyPrime(ann, &userv1.BuyPrimeRequest{Period: "month", UserId: bobID}); err != nil || bought.GetBalance() != 100 {
		t.Fatalf("BuyPrime for bob = %v, %v", bought, err)
	}
	got, err := s.client.GetProfile(context.Background(), &userv1.GetProfileRequest{Nickname: "bob"})
	if err != nil || !got.GetProfile().GetPrime() || got.GetProfile().GetPrimeStyle().GetName() != "amethyst" {
		t.Fatalf("bob's profile = %v, %v", got.GetProfile(), err)
	}
	if me, err = s.client.GetMe(bob, &userv1.GetMeRequest{}); err != nil || me.GetUser().GetPrimeUntil() == nil {
		t.Fatalf("bob's GetMe = %v, %v", me.GetUser(), err)
	}
	if _, err = s.client.BuyPrime(ann, &userv1.BuyPrimeRequest{Period: "quarter", UserId: bobID}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("BuyPrime with too few votes left: %v", err)
	}
}
