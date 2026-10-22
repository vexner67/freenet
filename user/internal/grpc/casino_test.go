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
	if got, err := s.client.GetCasino(context.Background(), &userv1.GetCasinoRequest{}); err != nil || got.GetPrice() != 10 || len(got.GetPrizes()) != 12 {
		t.Fatalf("GetCasino = %v, %v", got, err)
	}

	if _, err := s.client.GetCasinoSettings(ann, &userv1.GetCasinoSettingsRequest{}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("GetCasinoSettings by ann: %v", err)
	}
	if _, err := s.client.SetCasino(admin, &userv1.SetCasinoRequest{Price: 4, Prizes: []*userv1.CasinoPrize{{Votes: 3}, {Votes: 9}}}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("SetCasino without odds: %v", err)
	}

	if _, err := s.client.SetCasino(admin, &userv1.SetCasinoRequest{Price: 0, Prizes: []*userv1.CasinoPrize{{Votes: 0, Weight: 1}, {Votes: 9, Weight: 5}}}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("SetCasino for free: %v", err)
	}

	// A casino that always pays 8.
	tuned := &userv1.SetCasinoRequest{Price: 10, Prizes: []*userv1.CasinoPrize{{Votes: 0, Weight: 0}, {Votes: 8, Weight: 5}}}
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

	created, err := s.client.CreateVoucher(admin, &userv1.CreateVoucherRequest{Votes: 10, Usages: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.client.RedeemVoucher(ann, &userv1.RedeemVoucherRequest{Token: created.GetVoucher().GetToken()}); err != nil {
		t.Fatal(err)
	}

	spin, err := s.client.SpinCasino(ann, &userv1.SpinCasinoRequest{})
	if err != nil || spin.GetPrize() != 1 || spin.GetVotes() != 8 || spin.GetBalance() != 8 {
		t.Fatalf("SpinCasino = %v, %v", spin, err)
	}

	settings, err := s.client.GetCasinoSettings(admin, &userv1.GetCasinoSettingsRequest{})
	if err != nil || settings.GetPrice() != 10 || settings.GetPrizes()[1].GetWeight() != 5 || settings.GetSpins() != 1 || settings.GetSpent() != 10 || settings.GetWon() != 8 {
		t.Fatalf("GetCasinoSettings = %v, %v", settings, err)
	}
}

func TestCasinoBets(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	admin := s.signIn(t, adminEmail)
	ann, _ := s.member(t, "ann")

	got, err := s.client.GetCasino(context.Background(), &userv1.GetCasinoRequest{})
	if err != nil || got.GetBetLeast() != 1 || got.GetBetMost() != 100 || len(got.GetAim()) != 6 || got.GetAim()[0].GetMultiplier() != 200 || got.GetAim()[0].GetChance() != 4250 {
		t.Fatalf("GetCasino = %v, %v", got, err)
	}

	if _, err := s.client.SetCasinoBets(admin, &userv1.SetCasinoBetsRequest{Least: 10, Most: 5, Aim: []*userv1.AimMultiplier{{Multiplier: 200, Chance: 4000}}}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("SetCasinoBets upside down: %v", err)
	}
	if _, err := s.client.SetCasinoBets(admin, &userv1.SetCasinoBetsRequest{Least: 2, Most: 20}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("SetCasinoBets without multipliers: %v", err)
	}
	if _, err := s.client.SetCasinoBets(admin, &userv1.SetCasinoBetsRequest{Least: 2, Most: 20, Aim: []*userv1.AimMultiplier{{Multiplier: 200, Chance: 0}}}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("SetCasinoBets without a chance: %v", err)
	}
	tuned := &userv1.SetCasinoBetsRequest{Least: 2, Most: 20, Aim: []*userv1.AimMultiplier{{Multiplier: 200, Chance: 4250}, {Multiplier: 700, Chance: 1000}}}
	if _, err := s.client.SetCasinoBets(ann, tuned); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("SetCasinoBets by ann: %v", err)
	}
	if _, err := s.client.SetCasinoBets(admin, tuned); err != nil {
		t.Fatalf("SetCasinoBets: %v", err)
	}

	if _, err := s.client.BetCasino(ann, &userv1.BetCasinoRequest{Amount: 21, Multiplier: 200}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("BetCasino above the most: %v", err)
	}
	if _, err := s.client.BetCasino(ann, &userv1.BetCasinoRequest{Amount: 10, Multiplier: 7}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("BetCasino on ×0.07: %v", err)
	}
	if _, err := s.client.BetCasino(ann, &userv1.BetCasinoRequest{Amount: 10, Multiplier: 200}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("BetCasino without votes: %v", err)
	}

	created, err := s.client.CreateVoucher(admin, &userv1.CreateVoucherRequest{Votes: 10, Usages: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.client.RedeemVoucher(ann, &userv1.RedeemVoucherRequest{Token: created.GetVoucher().GetToken()}); err != nil {
		t.Fatal(err)
	}

	// Win or lose, the balance is the bet taken and the win paid.
	bet, err := s.client.BetCasino(ann, &userv1.BetCasinoRequest{Amount: 10, Multiplier: 200})
	if err != nil || bet.GetChance() != 4250 || bet.GetWon() != (bet.GetRoll() >= 2875 && bet.GetRoll() < 7125) ||
		(bet.GetWon() && (bet.GetVotes() != 20 || bet.GetBalance() != 20)) ||
		(!bet.GetWon() && (bet.GetVotes() != 0 || bet.GetBalance() != 0)) {
		t.Fatalf("BetCasino = %v, %v", bet, err)
	}

	settings, err := s.client.GetCasinoSettings(admin, &userv1.GetCasinoSettingsRequest{})
	if err != nil || settings.GetBetMost() != 20 || settings.GetBets() != 1 || settings.GetBetSpent() != 10 || settings.GetBetWon() != bet.GetVotes() ||
		len(settings.GetAim()) != 2 || settings.GetAim()[1].GetMultiplier() != 700 || settings.GetAim()[1].GetChance() != 1000 {
		t.Fatalf("GetCasinoSettings = %v, %v", settings, err)
	}
}
