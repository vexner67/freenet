package service_test

import (
	"context"
	"testing"

	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestEndlessService_Endow(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newWalletWorld(t)
	admin, ann := w.person(t, "admin"), w.person(t, "ann")

	admins, err := user.NewAdmins([]string{"Admin@Example.com"})
	if err != nil {
		t.Fatal(err)
	}
	endless := service.NewEndlessService(w.users, admins)

	if raised, err := endless.Endow(ctx); err != nil || raised != 1 {
		t.Fatalf("Endow = %d, %v; want 1", raised, err)
	}
	if got := w.votes(t, admin); got != user.EndlessVotes {
		t.Fatalf("the administrator has %d votes", got)
	}
	if got := w.votes(t, ann); got != 0 {
		t.Fatalf("ann has %d votes", got)
	}

	// What the administrator spends comes back; a balance kept is not
	// raised again.
	if _, err = w.users.Spend(ctx, admin, 1000, user.Entry(user.VoteGift, ann, "")); err != nil {
		t.Fatal(err)
	}
	if raised, err := endless.Endow(ctx); err != nil || raised != 1 || w.votes(t, admin) != user.EndlessVotes {
		t.Fatalf("Endow after a spend = %d, %v, %d votes", raised, err, w.votes(t, admin))
	}
	if raised, err := endless.Endow(ctx); err != nil || raised != 0 {
		t.Fatalf("Endow again = %d, %v; want 0", raised, err)
	}

	// The endless votes are no votes on the accounts of the economy.
	treasury, err := w.wallets.Treasury(ctx)
	if err != nil || user.IsEndless(treasury.Held) {
		t.Fatalf("Treasury = %+v, %v", treasury, err)
	}
}

func TestEndlessService_NoAdmins(t *testing.T) {
	t.Parallel()

	w := newWalletWorld(t)
	w.person(t, "admin")

	if raised, err := service.NewEndlessService(w.users, user.Admins{}).Endow(context.Background()); err != nil || raised != 0 {
		t.Fatalf("Endow = %d, %v; want 0", raised, err)
	}
}
