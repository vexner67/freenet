package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestWalletService_Bonus(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newWalletWorld(t)
	ann := w.person(t, "ann")
	now := time.Now()

	// A page without a profile, and a page younger than a week, takes no
	// bonus.
	if b, err := w.wallets.Bonus(ctx, ann, now); err != nil || b.Eligible || b.Taken || b.Votes != user.MaxBonus {
		t.Fatalf("Bonus = %+v, %v", b, err)
	}
	if _, err := w.wallets.ClaimBonus(ctx, ann, now); !errors.Is(err, user.ErrBonusNotYet) {
		t.Fatalf("ClaimBonus without a profile: %v", err)
	}
	if _, err := w.userSvc.UpdateProfile(ctx, ann, profile(t, "ann"), ""); err != nil {
		t.Fatal(err)
	}
	if _, err := w.wallets.ClaimBonus(ctx, ann, now); !errors.Is(err, user.ErrBonusNotYet) {
		t.Fatalf("ClaimBonus of a new page: %v", err)
	}

	// A week later.
	now = now.Add(user.BonusAge + time.Hour)

	claimed, err := w.wallets.ClaimBonus(ctx, ann, now)
	if err != nil || claimed.Votes != user.MaxBonus || claimed.Wallet.Votes != user.MaxBonus {
		t.Fatalf("ClaimBonus = %+v, %v", claimed, err)
	}
	if _, err := w.wallets.ClaimBonus(ctx, ann, now); !errors.Is(err, user.ErrBonusTaken) {
		t.Fatalf("second ClaimBonus: %v", err)
	}
	if b, err := w.wallets.Bonus(ctx, ann, now); err != nil || !b.Eligible || !b.Taken {
		t.Fatalf("Bonus after ClaimBonus = %+v, %v", b, err)
	}

	treasury, err := w.wallets.Treasury(ctx)
	if err != nil || treasury.Votes != user.TreasuryStart-user.MaxBonus {
		t.Fatalf("Treasury = %+v, %v", treasury, err)
	}

	// The next day there is another.
	if claimed, err = w.wallets.ClaimBonus(ctx, ann, now.Add(24*time.Hour)); err != nil || claimed.Wallet.Votes != 2*user.MaxBonus {
		t.Fatalf("next ClaimBonus = %+v, %v", claimed, err)
	}

	ops, err := w.wallets.Statement(ctx, ann, uuid.Nil, user.VoteFlowAll, 10, now)
	if err != nil || len(ops.Operations) != 2 || ops.Operations[0].Kind != user.VoteBonus {
		t.Fatalf("Operations = %+v, %v", ops, err)
	}
}

// A voucher promises any votes: the treasury has no bottom.
func TestWalletService_VoucherOutstanding(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newWalletWorld(t)
	admin := w.person(t, "admin")

	if _, err := w.wallets.CreateVoucher(ctx, admin, service.VoucherDraft{Votes: user.TreasuryStart, Usages: 2}); err != nil {
		t.Fatalf("CreateVoucher past the treasury: %v", err)
	}
	if treasury, err := w.wallets.Treasury(ctx); err != nil || treasury.Outstanding != 2*user.TreasuryStart {
		t.Fatalf("Treasury = %+v, %v", treasury, err)
	}
}

// Rating is bought for votes, into the treasury, up to user.MaxRating.
func TestWalletService_RaiseRating(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newWalletWorld(t)
	ann := w.person(t, "ann")

	five, err := user.NewRatingRaise(5)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.wallets.RaiseRating(ctx, ann, five); !errors.Is(err, user.ErrNotEnoughVotes) {
		t.Fatalf("RaiseRating without votes: %v", err)
	}
	if _, err = w.users.Credit(ctx, ann, 40, user.MaxRating-10, user.VoteEntry{}); err != nil {
		t.Fatal(err)
	}

	before, err := w.wallets.Treasury(ctx)
	if err != nil {
		t.Fatal(err)
	}

	wallet, err := w.wallets.RaiseRating(ctx, ann, five)
	if err != nil || wallet.Votes != 40-five.Cost() || wallet.Rating != user.MaxRating-5 {
		t.Fatalf("RaiseRating = %+v, %v", wallet, err)
	}
	if after, err := w.wallets.Treasury(ctx); err != nil || after.Votes != before.Votes+five.Cost() {
		t.Fatalf("Treasury = %+v, %v; before %+v", after, err, before)
	}

	ops, err := w.wallets.Statement(ctx, ann, uuid.Nil, user.VoteFlowAll, 1, time.Now())
	if err != nil || len(ops.Operations) != 1 || ops.Operations[0].Kind != user.VoteRating || ops.Operations[0].Note != "5" {
		t.Fatalf("Statement = %+v, %v", ops, err)
	}

	// Past the limit nothing is bought.
	six, err := user.NewRatingRaise(6)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.wallets.RaiseRating(ctx, ann, six); !errors.Is(err, user.ErrRatingLimit) {
		t.Fatalf("RaiseRating past the limit: %v", err)
	}
	if got := w.votes(t, ann); got != 40-five.Cost() {
		t.Fatalf("after the refused raise ann has %d votes", got)
	}
}
