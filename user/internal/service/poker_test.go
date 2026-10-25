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

func TestPokerService(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newWalletWorld(t)
	admin, ann, bob := w.person(t, "admin"), w.person(t, "ann"), w.person(t, "bob")

	admins, err := user.NewAdmins([]string{"admin@example.com"})
	if err != nil {
		t.Fatal(err)
	}

	// The deck is not shuffled: the player gets 2s 2h, the dealer 2d 2c
	// and the board is 3s 3h 3d 3c 4s — four threes with a four for both,
	// a tie.
	pokers := service.NewPokerService(w.users, inmem.NewPokerRepository(), w.users, inmem.TxManager{}, admins).
		WithRoll(func(int64) (int64, error) { return 0, nil })

	tuned, err := user.NewPoker(10, 100, [user.PokerPays]int{100, 20, 10, 3, 2, 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := pokers.SetPoker(ctx, ann, tuned); !errors.Is(err, user.ErrNotAdmin) {
		t.Fatalf("SetPoker by ann: %v", err)
	}
	if err := pokers.SetPoker(ctx, admin, tuned); err != nil {
		t.Fatal(err)
	}

	if _, err := pokers.Deal(ctx, ann, 10); !errors.Is(err, user.ErrNotEnoughVotes) {
		t.Fatalf("Deal without votes: %v", err)
	}
	if _, err := pokers.Deal(ctx, ann, 5); !errors.Is(err, user.ErrPokerAnte) {
		t.Fatalf("Deal below the least: %v", err)
	}
	if _, err := w.users.Credit(ctx, ann, 1000, 0, user.VoteEntry{}); err != nil {
		t.Fatal(err)
	}

	dealt, err := pokers.Deal(ctx, ann, 10)
	if err != nil || dealt.Balance != 990 || dealt.Hand.State() != user.PokerDealt {
		t.Fatalf("Deal = %+v, %v", dealt, err)
	}
	if _, err := pokers.Deal(ctx, ann, 10); !errors.Is(err, user.ErrPokerOpen) {
		t.Fatalf("Deal with a hand waiting: %v", err)
	}
	if table, err := pokers.Table(ctx, ann); err != nil || table.Dealt == nil || table.Dealt.ID() != dealt.Hand.ID() || table.Poker != tuned {
		t.Fatalf("Table = %+v, %v", table, err)
	}
	if table, err := pokers.Table(ctx, uuid.Nil); err != nil || table.Dealt != nil {
		t.Fatalf("Table of a guest = %+v, %v", table, err)
	}

	id := dealt.Hand.ID()
	if _, err := pokers.Call(ctx, bob, id); !errors.Is(err, user.ErrPokerNotFound) {
		t.Fatalf("Call by bob: %v", err)
	}

	called, err := pokers.Call(ctx, ann, id)
	if err != nil || called.Hand.State() != user.PokerTied || called.Hand.Payout() != 30 || called.Balance != 1000 {
		t.Fatalf("Call = %+v, %v", called, err)
	}
	if _, err := pokers.Fold(ctx, ann, id); !errors.Is(err, user.ErrPokerOver) {
		t.Fatalf("Fold after the call: %v", err)
	}

	again, err := pokers.Deal(ctx, ann, 20)
	if err != nil {
		t.Fatal(err)
	}
	folded, err := pokers.Fold(ctx, ann, again.Hand.ID())
	if err != nil || folded.Hand.State() != user.PokerFolded || folded.Balance != 980 {
		t.Fatalf("Fold = %+v, %v", folded, err)
	}
	if table, err := pokers.Table(ctx, ann); err != nil || table.Dealt != nil {
		t.Fatalf("Table after the fold = %+v, %v", table, err)
	}

	if _, err := pokers.Settings(ctx, ann); !errors.Is(err, user.ErrNotAdmin) {
		t.Fatalf("Settings by ann: %v", err)
	}
	settings, err := pokers.Settings(ctx, admin)
	if err != nil || settings.Poker != tuned || settings.Stats != (user.PokerStats{Hands: 2, Spent: 50, Won: 30}) {
		t.Fatalf("Settings = %+v, %v", settings, err)
	}
}
