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
