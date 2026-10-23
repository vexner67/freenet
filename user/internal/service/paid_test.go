package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/inmem"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

func newPaidService(w walletWorld) *service.PaidService {
	follows := inmem.NewFollowRepository(w.users)
	permissions := service.NewPermissionService(w.users, follows, inmem.NewBlockRepository(follows))

	return service.NewPaidService(w.users, w.users, inmem.NewPurchaseRepository(), inmem.NewEconomyRepository(), permissions, inmem.TxManager{})
}

func TestPaidService_BuyPost(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newWalletWorld(t)
	paid := newPaidService(w)
	admin, ann, bob := w.person(t, "admin"), w.person(t, "ann"), w.person(t, "bob")
	post := uuid.Must(uuid.NewV7())

	if _, err := paid.BuyPost(ctx, ann, post, ann, 10); !errors.Is(err, user.ErrInvalidPostPrice) {
		t.Fatalf("own post: %v", err)
	}
	if _, err := paid.BuyPost(ctx, bob, post, ann, 0); !errors.Is(err, user.ErrInvalidPostPrice) {
		t.Fatalf("free post: %v", err)
	}
	if _, err := paid.BuyPost(ctx, bob, post, ann, 10); !errors.Is(err, user.ErrNotEnoughVotes) {
		t.Fatalf("without votes: %v", err)
	}

	w.credit(t, admin, bob, 100)

	left, err := paid.BuyPost(ctx, bob, post, ann, 10)
	if err != nil || left != 90 {
		t.Fatalf("BuyPost = %d, %v", left, err)
	}
	// Ann gets the price less the 10 % tax.
	if got := w.votes(t, ann); got != 9 {
		t.Fatalf("ann has %d", got)
	}

	// Bought again, it costs nothing.
	if left, err = paid.BuyPost(ctx, bob, post, ann, 10); err != nil || left != 90 {
		t.Fatalf("BuyPost again = %d, %v", left, err)
	}

	bought, err := paid.Bought(ctx, bob, []uuid.UUID{post, uuid.Must(uuid.NewV7())})
	if err != nil || len(bought) != 1 || bought[0] != post {
		t.Fatalf("Bought = %v, %v", bought, err)
	}
	if _, err = paid.Bought(ctx, bob, make([]uuid.UUID, service.MaxBoughtBatch+1)); !errors.Is(err, service.ErrTooManyPosts) {
		t.Fatalf("too many: %v", err)
	}
}

func TestPaidService_Messages(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newWalletWorld(t)
	paid := newPaidService(w)
	admin, ann, bob := w.person(t, "admin"), w.person(t, "ann"), w.person(t, "bob")

	setPrivacy := func(change func(*user.Privacy)) {
		t.Helper()

		u, err := w.users.GetByID(ctx, ann)
		if err != nil {
			t.Fatal(err)
		}
		privacy := u.Privacy()
		change(&privacy)
		if _, err = w.userSvc.UpdatePrivacy(ctx, ann, privacy); err != nil {
			t.Fatal(err)
		}
	}

	// Only Freenet Prime: bob has none.
	setPrivacy(func(p *user.Privacy) { p.MessagesPrime = true })
	terms, err := paid.Terms(ctx, bob, ann, false)
	if err != nil || terms.Allowed || !terms.PrimeOnly {
		t.Fatalf("Terms prime only = %+v, %v", terms, err)
	}
	// Ann wrote first: bob may answer.
	if terms, err = paid.Terms(ctx, bob, ann, true); err != nil || !terms.Allowed {
		t.Fatalf("Terms after ann wrote = %+v, %v", terms, err)
	}
	if _, _, err = paid.PayMessage(ctx, bob, ann, false, 0); !errors.Is(err, user.ErrCannotMessage) {
		t.Fatalf("PayMessage without Prime: %v", err)
	}

	// Messages for 20 votes.
	setPrivacy(func(p *user.Privacy) {
		p.MessagesPrime = false
		p.MessagePrice, _ = user.NewMessagePrice(20)
	})
	if terms, err = paid.Terms(ctx, bob, ann, false); err != nil || !terms.Allowed || terms.Price != 20 {
		t.Fatalf("Terms paid = %+v, %v", terms, err)
	}
	if terms, err = paid.Terms(ctx, bob, ann, true); err != nil || terms.Price != 0 {
		t.Fatalf("Terms after ann wrote = %+v, %v", terms, err)
	}
	if terms, err = paid.Terms(ctx, ann, ann, false); err != nil || terms.Price != 0 {
		t.Fatalf("Terms to oneself = %+v, %v", terms, err)
	}

	if _, _, err = paid.PayMessage(ctx, bob, ann, false, 10); !errors.Is(err, user.ErrMessagePriceChanged) {
		t.Fatalf("PayMessage at an old price: %v", err)
	}
	if _, _, err = paid.PayMessage(ctx, bob, ann, false, 20); !errors.Is(err, user.ErrNotEnoughVotes) {
		t.Fatalf("PayMessage without votes: %v", err)
	}

	w.credit(t, admin, bob, 100)

	payment, left, err := paid.PayMessage(ctx, bob, ann, false, 20)
	if err != nil || left != 80 || payment.Tax != 2 {
		t.Fatalf("PayMessage = %+v, %d, %v", payment, left, err)
	}
	if got := w.votes(t, ann); got != 18 {
		t.Fatalf("ann has %d", got)
	}

	// The message was not sent: bob gets it all back.
	if err = paid.RefundMessage(ctx, bob, payment.ID); err != nil {
		t.Fatal(err)
	}
	if got := w.votes(t, bob); got != 100 {
		t.Fatalf("bob has %d after the refund", got)
	}
	if got := w.votes(t, ann); got != 0 {
		t.Fatalf("ann has %d after the refund", got)
	}
	if err = paid.RefundMessage(ctx, bob, payment.ID); !errors.Is(err, user.ErrPaymentNotFound) {
		t.Fatalf("refund twice: %v", err)
	}

	// Free messages need no payment.
	payment, _, err = paid.PayMessage(ctx, bob, ann, true, 0)
	if err != nil || payment.ID != uuid.Nil {
		t.Fatalf("PayMessage free = %+v, %v", payment, err)
	}
}
