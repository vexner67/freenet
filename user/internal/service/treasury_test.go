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
	if b, err := w.wallets.Bonus(ctx, ann, now); err != nil || b.Eligible || b.Taken || b.Votes != user.DefaultEconomy.Bonus {
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
	if err != nil || claimed.Votes != user.DefaultEconomy.Bonus || claimed.Wallet.Votes != user.DefaultEconomy.Bonus {
		t.Fatalf("ClaimBonus = %+v, %v", claimed, err)
	}
	if _, err := w.wallets.ClaimBonus(ctx, ann, now); !errors.Is(err, user.ErrBonusTaken) {
		t.Fatalf("second ClaimBonus: %v", err)
	}
	if b, err := w.wallets.Bonus(ctx, ann, now); err != nil || !b.Eligible || !b.Taken {
		t.Fatalf("Bonus after ClaimBonus = %+v, %v", b, err)
	}

	treasury, err := w.wallets.Treasury(ctx)
	if err != nil || treasury.Votes != user.TreasuryStart-user.DefaultEconomy.Bonus {
		t.Fatalf("Treasury = %+v, %v", treasury, err)
	}

	// The next day there is another.
	if claimed, err = w.wallets.ClaimBonus(ctx, ann, now.Add(24*time.Hour)); err != nil || claimed.Wallet.Votes != 2*user.DefaultEconomy.Bonus {
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
	// Ann's page is filled 10 %: with it the rating stays within
	// MaxRating, so she may buy up to 990.
	if _, err = w.users.Credit(ctx, ann, 40, user.MaxRating-20, user.VoteEntry{}); err != nil {
		t.Fatal(err)
	}

	before, err := w.wallets.Treasury(ctx)
	if err != nil {
		t.Fatal(err)
	}

	wallet, err := w.wallets.RaiseRating(ctx, ann, five)
	if err != nil || wallet.Votes != 40-five.Cost() || wallet.Rating != user.MaxRating-15 {
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

// The administrators set the bonus and the fees; the next bonus, transfer
// and sale take them.
func TestWalletService_SetEconomy(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newWalletWorld(t)
	admin, ann, bob := w.person(t, "admin"), w.person(t, "ann"), w.person(t, "bob")

	economy := user.Economy{Bonus: 10_000, TransferFee: 10, StickerTax: 20, PrimeStickerTax: 1}
	if err := w.wallets.SetEconomy(ctx, ann, economy); !errors.Is(err, user.ErrNotAdmin) {
		t.Fatalf("SetEconomy by ann: %v", err)
	}
	if err := w.wallets.SetEconomy(ctx, admin, economy); err != nil {
		t.Fatal(err)
	}
	if got, err := w.wallets.Economy(ctx); err != nil || got != economy {
		t.Fatalf("Economy = %+v, %v", got, err)
	}

	if b, err := w.wallets.Bonus(ctx, ann, time.Now()); err != nil || b.Votes != 10_000 {
		t.Fatalf("Bonus = %+v, %v", b, err)
	}

	for id, name := range map[uuid.UUID]string{ann: "ann", bob: "bob"} {
		if _, err := w.userSvc.UpdateProfile(ctx, id, profile(t, name), ""); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := w.users.Credit(ctx, ann, 1000, 0, user.VoteEntry{}); err != nil {
		t.Fatal(err)
	}
	if _, fee, err := w.wallets.Transfer(ctx, ann, bob, 100, ""); err != nil || fee != 10 {
		t.Fatalf("Transfer: fee %d, %v", fee, err)
	}
}
