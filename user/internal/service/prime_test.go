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

// fakePrimeNotifier records Prime given as a present.
type fakePrimeNotifier struct {
	given []giftNote
}

func (f *fakePrimeNotifier) PrimeGiven(_ context.Context, purchase, buyer, recipient uuid.UUID) error {
	f.given = append(f.given, giftNote{purchase, buyer, recipient})
	return nil
}

// credit gives id votes through a voucher of the administrator.
func (w walletWorld) credit(t *testing.T, admin, id uuid.UUID, votes int64) {
	t.Helper()

	voucher, err := w.wallets.CreateVoucher(context.Background(), admin, service.VoucherDraft{Votes: votes, Usages: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.wallets.Redeem(context.Background(), id, voucher.Token()); err != nil {
		t.Fatal(err)
	}
}

func TestPrimeService(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newWalletWorld(t)
	admin, ann, bob, cat := w.person(t, "admin"), w.person(t, "ann"), w.person(t, "bob"), w.person(t, "cat")

	prices, err := w.primes.Prices(ctx)
	if err != nil || prices != user.DefaultPrimePrices {
		t.Fatalf("Prices = %+v, %v", prices, err)
	}
	cheap, _ := user.NewPrimePrices(10, 25, 90)
	if err = w.primes.SetPrices(ctx, ann, cheap); !errors.Is(err, user.ErrNotAdmin) {
		t.Fatalf("SetPrices by ann: %v", err)
	}
	if err = w.primes.SetPrices(ctx, admin, cheap); err != nil {
		t.Fatal(err)
	}

	if _, err = w.primes.Buy(ctx, ann, ann, user.PrimeMonth); !errors.Is(err, user.ErrNotEnoughVotes) {
		t.Fatalf("Buy without votes: %v", err)
	}

	w.credit(t, admin, ann, 100)

	bought, err := w.primes.Buy(ctx, ann, ann, user.PrimeMonth)
	if err != nil || bought.Balance != 90 || bought.Until.IsZero() {
		t.Fatalf("Buy = %+v, %v", bought, err)
	}
	// Bought again, the month adds to what is left.
	again, err := w.primes.Buy(ctx, ann, ann, user.PrimeMonth)
	if err != nil || !again.Until.Equal(bought.Until.AddDate(0, 0, 30)) {
		t.Fatalf("Buy again = %+v, %v", again, err)
	}
	if len(w.given.given) != 0 {
		t.Fatalf("one's own Prime told: %+v", w.given.given)
	}

	// A present: bob hears of it.
	if _, err = w.primes.Buy(ctx, ann, bob, user.PrimeQuarter); err != nil {
		t.Fatal(err)
	}
	if len(w.given.given) != 1 || w.given.given[0].sender != ann || w.given.given[0].recipient != bob {
		t.Fatalf("told = %+v", w.given.given)
	}

	// No present through a block.
	if err = w.blocks.Block(ctx, cat, ann); err != nil {
		t.Fatal(err)
	}
	if _, err = w.primes.Buy(ctx, ann, cat, user.PrimeMonth); !errors.Is(err, user.ErrGiftRefused) {
		t.Fatalf("a present through a block: %v", err)
	}
	if _, err = w.primes.Buy(ctx, ann, uuid.New(), user.PrimeMonth); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("a present to nobody: %v", err)
	}

	// The statement tells what went where.
	page, err := w.wallets.Statement(ctx, ann, uuid.Nil, user.VoteFlowAll, 10, time.Now())
	if err != nil || len(page.Operations) == 0 || page.Operations[0].Kind != user.VotePrime || page.Operations[0].PeerID != bob {
		t.Fatalf("statement = %+v, %v", page, err)
	}

	// Invisible with Prime only.
	if err = w.primes.SetInvisible(ctx, cat, true); !errors.Is(err, user.ErrPrimeOnly) {
		t.Fatalf("invisible without Prime: %v", err)
	}
	if err = w.primes.SetInvisible(ctx, ann, true); err != nil {
		t.Fatal(err)
	}
	if u, _ := w.users.GetByID(ctx, ann); !u.Prime().Invisible {
		t.Fatal("not invisible")
	}
}

func TestPrime_Perks(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newWalletWorld(t)
	admin, ann, bob := w.person(t, "admin"), w.person(t, "ann"), w.person(t, "bob")
	w.credit(t, admin, ann, 400)
	w.credit(t, admin, bob, 400)

	// The gifts of "prime" are for Prime alone.
	draft := service.GiftDraft{Kind: "prime_star"}
	if _, err := w.gifts.Send(ctx, bob, ann, draft); !errors.Is(err, user.ErrPrimeOnly) {
		t.Fatalf("a prime gift without Prime: %v", err)
	}

	if _, err := w.primes.Buy(ctx, ann, ann, user.PrimeMonth); err != nil {
		t.Fatal(err)
	}
	if _, err := w.gifts.Send(ctx, ann, bob, draft); err != nil {
		t.Fatalf("a prime gift with Prime: %v", err)
	}

	// No fee on a transfer with Prime.
	before := w.votes(t, ann)
	left, fee, err := w.wallets.Transfer(ctx, ann, bob, 100, "")
	if err != nil || fee != 0 || left != before-100 {
		t.Fatalf("Transfer with Prime = %d, %d, %v", left, fee, err)
	}
	if _, fee, err = w.wallets.Transfer(ctx, bob, ann, 100, ""); err != nil || fee != user.TransferFee(100) {
		t.Fatalf("Transfer without Prime: fee %d, %v", fee, err)
	}
}
