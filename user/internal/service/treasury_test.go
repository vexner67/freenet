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

	// A week later: the first bonus mints into the treasury too.
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
	if err != nil || treasury.Votes != user.TreasuryStart+user.MemberMint-user.MaxBonus || treasury.Minted != user.MemberMint {
		t.Fatalf("Treasury = %+v, %v", treasury, err)
	}

	// The next day there is another, and no mint.
	if claimed, err = w.wallets.ClaimBonus(ctx, ann, now.Add(24*time.Hour)); err != nil || claimed.Wallet.Votes != 2*user.MaxBonus {
		t.Fatalf("next ClaimBonus = %+v, %v", claimed, err)
	}
	if treasury, err = w.wallets.Treasury(ctx); err != nil || treasury.Minted != user.MemberMint {
		t.Fatalf("Treasury after the next = %+v, %v", treasury, err)
	}

	ops, err := w.wallets.Statement(ctx, ann, uuid.Nil, user.VoteFlowAll, 10, now)
	if err != nil || len(ops.Operations) != 2 || ops.Operations[0].Kind != user.VoteBonus {
		t.Fatalf("Operations = %+v, %v", ops, err)
	}
}

// A voucher promises votes the treasury must have free.
func TestWalletService_VoucherCover(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newWalletWorld(t)
	admin := w.person(t, "admin")
	free := int64(user.TreasuryStart - user.TreasuryReserve)

	if _, err := w.wallets.CreateVoucher(ctx, admin, service.VoucherDraft{Votes: free/2 + 1, Usages: 2}); !errors.Is(err, user.ErrVoucherCover) {
		t.Fatalf("CreateVoucher past the free votes: %v", err)
	}
	if _, err := w.wallets.CreateVoucher(ctx, admin, service.VoucherDraft{Votes: free / 2, Usages: 2}); err != nil {
		t.Fatal(err)
	}
	// What it promises is not free any more.
	if _, err := w.wallets.CreateVoucher(ctx, admin, service.VoucherDraft{Votes: 1, Usages: 1}); !errors.Is(err, user.ErrVoucherCover) {
		t.Fatalf("CreateVoucher past the promised: %v", err)
	}
	if treasury, err := w.wallets.Treasury(ctx); err != nil || treasury.Free() != 0 || treasury.Outstanding != free {
		t.Fatalf("Treasury = %+v, %v", treasury, err)
	}
	// Rating costs the treasury nothing.
	if _, err := w.wallets.CreateVoucher(ctx, admin, service.VoucherDraft{Rating: 5, Usages: user.UnlimitedUsages}); err != nil {
		t.Fatal(err)
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
