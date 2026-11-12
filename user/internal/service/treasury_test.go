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

// Rating is bought for votes, a vote a percent, into the treasury, up to
// user.MaxRating.
func TestWalletService_RaiseRating(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newWalletWorld(t)
	ann := w.person(t, "ann")

	five, err := user.NewRatingRaise(5)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.wallets.RaiseRating(ctx, ann, ann, five); !errors.Is(err, user.ErrNotEnoughVotes) {
		t.Fatalf("RaiseRating without votes: %v", err)
	}
	// Ann's page is empty: the rating alone stays within MaxRating, so
	// with this much she may buy 10 more.
	if _, err = w.users.Credit(ctx, ann, 40, user.MaxRating-15, user.VoteEntry{}); err != nil {
		t.Fatal(err)
	}

	before, err := w.wallets.Treasury(ctx)
	if err != nil {
		t.Fatal(err)
	}

	raised, err := w.wallets.RaiseRating(ctx, ann, ann, five)
	if err != nil || raised.Balance != 40-five.Cost() || raised.Rating != user.MaxRating-10 || raised.Completeness != user.MaxRating-10 {
		t.Fatalf("RaiseRating = %+v, %v", raised, err)
	}
	if after, err := w.wallets.Treasury(ctx); err != nil || after.Votes != before.Votes+five.Cost() {
		t.Fatalf("Treasury = %+v, %v; before %+v", after, err, before)
	}

	ops, err := w.wallets.Statement(ctx, ann, uuid.Nil, user.VoteFlowAll, 1, time.Now())
	if err != nil || len(ops.Operations) != 1 || ops.Operations[0].Kind != user.VoteRating || ops.Operations[0].Note != "5" ||
		ops.Operations[0].PeerID != uuid.Nil {
		t.Fatalf("Statement = %+v, %v", ops, err)
	}

	// Past the limit nothing is bought.
	eleven, err := user.NewRatingRaise(11)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.wallets.RaiseRating(ctx, ann, ann, eleven); !errors.Is(err, user.ErrRatingLimit) {
		t.Fatalf("RaiseRating past the limit: %v", err)
	}
	if got := w.votes(t, ann); got != 40-five.Cost() {
		t.Fatalf("after the refused raise ann has %d votes", got)
	}
}

// Anyone may raise the rating of another page with their votes, but not
// of a deleted one or across a blacklist.
func TestWalletService_RaiseRatingOfAnother(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newWalletWorld(t)
	ann, bob, cat := w.person(t, "ann"), w.person(t, "bob"), w.person(t, "cat")

	if _, err := w.users.Credit(ctx, ann, 100, 0, user.VoteEntry{}); err != nil {
		t.Fatal(err)
	}

	thirty, err := user.NewRatingRaise(30)
	if err != nil {
		t.Fatal(err)
	}

	raised, err := w.wallets.RaiseRating(ctx, ann, bob, thirty)
	if err != nil || raised.Balance != 70 || raised.Rating != 30 || raised.Completeness != 30 {
		t.Fatalf("RaiseRating of bob = %+v, %v", raised, err)
	}
	if got, err := w.users.GetByID(ctx, bob); err != nil || got.Wallet().Rating != 30 {
		t.Fatalf("bob = %+v, %v", got, err)
	}
	if got, err := w.users.GetByID(ctx, ann); err != nil || got.Wallet().Rating != 0 {
		t.Fatalf("ann = %+v, %v", got, err)
	}

	// Ann's statement names whose rating she raised.
	ops, err := w.wallets.Statement(ctx, ann, uuid.Nil, user.VoteFlowAll, 1, time.Now())
	if err != nil || len(ops.Operations) != 1 || ops.Operations[0].PeerID != bob || ops.Operations[0].Amount != -30 {
		t.Fatalf("Statement = %+v, %v", ops, err)
	}

	if err = w.blocks.Block(ctx, cat, ann); err != nil {
		t.Fatal(err)
	}
	if _, err = w.wallets.RaiseRating(ctx, ann, cat, thirty); !errors.Is(err, user.ErrRatingRefused) {
		t.Fatalf("RaiseRating across a blacklist: %v", err)
	}

	if _, err = w.accounts.Deactivate(ctx, bob, ""); err != nil {
		t.Fatal(err)
	}
	if _, err = w.wallets.RaiseRating(ctx, ann, bob, thirty); !errors.Is(err, user.ErrDeactivated) {
		t.Fatalf("RaiseRating of a deleted page: %v", err)
	}
	if _, err = w.wallets.RaiseRating(ctx, ann, uuid.New(), thirty); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("RaiseRating of nobody: %v", err)
	}
	if got := w.votes(t, ann); got != 70 {
		t.Fatalf("after the refused raises ann has %d votes", got)
	}
}

// The administrators set the fees and taxes; the next transfer
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
