package grpc_test

import (
	"context"
	"testing"

	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"github.com/vexner67/freenet/user/internal/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestTreasury(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	admin := s.signIn(t, adminEmail)
	ann, _ := s.member(t, "ann")

	// Anyone sees the treasury.
	got, err := s.client.GetTreasury(context.Background(), &userv1.GetTreasuryRequest{})
	if err != nil || got.GetVotes() != user.TreasuryStart || got.GetReserve() != user.TreasuryReserve ||
		got.GetFree() != user.TreasuryStart-user.TreasuryReserve || got.GetBonus() != user.MaxBonus || got.GetMaxWin() != user.MaxWin {
		t.Fatalf("GetTreasury = %v, %v", got, err)
	}

	// A voucher takes the free votes.
	if _, err := s.client.CreateVoucher(admin, &userv1.CreateVoucherRequest{Votes: got.GetFree() + 1, Usages: 1}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("CreateVoucher past the free votes: %v", err)
	}
	if _, err := s.client.CreateVoucher(admin, &userv1.CreateVoucherRequest{Votes: 100, Usages: -1}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("CreateVoucher of votes without end: %v", err)
	}
	if _, err := s.client.CreateVoucher(admin, &userv1.CreateVoucherRequest{Votes: 100, Usages: 2}); err != nil {
		t.Fatal(err)
	}
	if got, err = s.client.GetTreasury(context.Background(), &userv1.GetTreasuryRequest{}); err != nil || got.GetOutstanding() != 200 {
		t.Fatalf("GetTreasury after a voucher = %v, %v", got, err)
	}

	// A page made today takes no bonus yet.
	if _, err := s.client.GetBonus(context.Background(), &userv1.GetBonusRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("GetBonus by nobody: %v", err)
	}
	bonus, err := s.client.GetBonus(ann, &userv1.GetBonusRequest{})
	if err != nil || bonus.GetEligible() || bonus.GetTaken() || bonus.GetVotes() != user.MaxBonus {
		t.Fatalf("GetBonus = %v, %v", bonus, err)
	}
	if _, err := s.client.ClaimBonus(ann, &userv1.ClaimBonusRequest{}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("ClaimBonus of a new page: %v", err)
	}
}

func TestRaiseRatingAndJackpot(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	admin := s.signIn(t, adminEmail)
	ann, _ := s.member(t, "ann")

	made, err := s.client.CreateVoucher(admin, &userv1.CreateVoucherRequest{Votes: 10, Usages: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.client.RedeemVoucher(ann, &userv1.RedeemVoucherRequest{Token: made.GetVoucher().GetToken()}); err != nil {
		t.Fatal(err)
	}

	raised, err := s.client.RaiseRating(ann, &userv1.RaiseRatingRequest{Percent: 2})
	if err != nil || raised.GetBalance() != 10-2*user.RatingPrice || raised.GetRating() != 2 {
		t.Fatalf("RaiseRating = %v, %v", raised, err)
	}
	if _, err = s.client.RaiseRating(ann, &userv1.RaiseRatingRequest{}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("RaiseRating by nothing: %v", err)
	}
	if _, err = s.client.RaiseRating(ann, &userv1.RaiseRatingRequest{Percent: 100}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("RaiseRating past the votes: %v", err)
	}
	if _, err = s.client.RaiseRating(context.Background(), &userv1.RaiseRatingRequest{Percent: 1}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("RaiseRating by nobody: %v", err)
	}

	// The jackpot is the administrators' to set.
	if casino, err := s.client.GetCasino(context.Background(), &userv1.GetCasinoRequest{}); err != nil || casino.GetJackpot() != user.DefaultJackpot {
		t.Fatalf("GetCasino = %v, %v", casino, err)
	}
	if _, err = s.client.SetCasinoJackpot(ann, &userv1.SetCasinoJackpotRequest{Jackpot: 500}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("SetCasinoJackpot by ann: %v", err)
	}
	if _, err = s.client.SetCasinoJackpot(admin, &userv1.SetCasinoJackpotRequest{Jackpot: user.MaxWin + 1}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("SetCasinoJackpot past the most win: %v", err)
	}
	if _, err = s.client.SetCasinoJackpot(admin, &userv1.SetCasinoJackpotRequest{Jackpot: 500}); err != nil {
		t.Fatal(err)
	}
	if settings, err := s.client.GetCasinoSettings(admin, &userv1.GetCasinoSettingsRequest{}); err != nil || settings.GetJackpot() != 500 {
		t.Fatalf("GetCasinoSettings = %v, %v", settings, err)
	}
}
