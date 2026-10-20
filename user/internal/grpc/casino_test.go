package grpc_test

import (
	"context"
	"testing"

	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestCasino(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	admin := s.signIn(t, adminEmail)
	ann, _ := s.member(t, "ann")

	// Anyone, even without a token, sees the price and the prizes, not the
	// odds.
	if got, err := s.client.GetCasino(context.Background(), &userv1.GetCasinoRequest{}); err != nil || got.GetPrice() != 10 || len(got.GetPrizes()) != 6 {
		t.Fatalf("GetCasino = %v, %v", got, err)
	}

	if _, err := s.client.GetCasinoSettings(ann, &userv1.GetCasinoSettingsRequest{}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("GetCasinoSettings by ann: %v", err)
	}
	if _, err := s.client.SetCasino(admin, &userv1.SetCasinoRequest{Price: 4, Prizes: []*userv1.CasinoPrize{{Votes: 3}, {Votes: 9}}}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("SetCasino without odds: %v", err)
	}

	// A casino that always pays 9.
	tuned := &userv1.SetCasinoRequest{Price: 4, Prizes: []*userv1.CasinoPrize{{Votes: 0, Weight: 0}, {Votes: 9, Weight: 5}}}
	if _, err := s.client.SetCasino(ann, tuned); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("SetCasino by ann: %v", err)
	}
	if _, err := s.client.SetCasino(admin, tuned); err != nil {
		t.Fatalf("SetCasino: %v", err)
	}

	if _, err := s.client.SpinCasino(ann, &userv1.SpinCasinoRequest{}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("SpinCasino without votes: %v", err)
	}
	if _, err := s.client.SpinCasino(context.Background(), &userv1.SpinCasinoRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("SpinCasino by nobody: %v", err)
	}

	created, err := s.client.CreateVoucher(admin, &userv1.CreateVoucherRequest{Votes: 4, Usages: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.client.RedeemVoucher(ann, &userv1.RedeemVoucherRequest{Token: created.GetVoucher().GetToken()}); err != nil {
		t.Fatal(err)
	}

	spin, err := s.client.SpinCasino(ann, &userv1.SpinCasinoRequest{})
	if err != nil || spin.GetPrize() != 1 || spin.GetVotes() != 9 || spin.GetBalance() != 9 {
		t.Fatalf("SpinCasino = %v, %v", spin, err)
	}

	settings, err := s.client.GetCasinoSettings(admin, &userv1.GetCasinoSettingsRequest{})
	if err != nil || settings.GetPrice() != 4 || settings.GetPrizes()[1].GetWeight() != 5 || settings.GetSpins() != 1 || settings.GetSpent() != 4 || settings.GetWon() != 9 {
		t.Fatalf("GetCasinoSettings = %v, %v", settings, err)
	}
}
