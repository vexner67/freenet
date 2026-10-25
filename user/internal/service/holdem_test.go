package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/inmem"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestHoldemService(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newWalletWorld(t)
	ann, bob, eve := w.person(t, "ann"), w.person(t, "bob"), w.person(t, "eve")

	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	tables := service.NewHoldemService(w.users, inmem.NewHoldemRepository(), w.users, inmem.TxManager{}).
		WithRoll(func(int64) (int64, error) { return 0, nil }, func() time.Time { return now })

	if _, err := tables.Quick(ctx, ann, 3, 100); !errors.Is(err, user.ErrTableStake) {
		t.Fatalf("Quick of an unknown stake: %v", err)
	}
	if _, err := tables.Quick(ctx, ann, 10, 200); !errors.Is(err, user.ErrNotEnoughVotes) {
		t.Fatalf("Quick without votes: %v", err)
	}
	for _, id := range []uuid.UUID{ann, bob, eve} {
		if _, err := w.users.Credit(ctx, id, 1000, 0, user.VoteEntry{}); err != nil {
			t.Fatal(err)
		}
	}

	// Ann opens a quick table of big blind 10, Bob joins the same one.
	first, err := tables.Quick(ctx, ann, 10, 500)
	if err != nil || first.Balance != 500 {
		t.Fatalf("Quick = %+v, %v", first, err)
	}
	second, err := tables.Quick(ctx, bob, 10, 200)
	if err != nil || second.Table.ID() != first.Table.ID() || second.Table.Players() != 2 {
		t.Fatalf("Quick of Bob = %+v, %v", second, err)
	}

	lobby, err := tables.Tables(ctx, eve, service.TablesPublic)
	if err != nil || len(lobby) != 1 {
		t.Fatalf("Tables = %v, %v", lobby, err)
	}

	// The hand is dealt when the table is read past the start.
	now = now.Add(5 * time.Second)
	table, err := tables.Table(ctx, eve, first.Table.ID())
	if err != nil || table.Street() != user.StreetPreflop {
		t.Fatalf("Table = %v, %v", table, err)
	}

	// Heads-up the first seat is the button and acts first: Ann folds.
	if _, err = tables.Act(ctx, bob, table.ID(), user.ActCall, 0); !errors.Is(err, user.ErrTableNotYourTurn) {
		t.Fatalf("Act out of turn: %v", err)
	}
	if table, err = tables.Act(ctx, ann, table.ID(), user.ActFold, 0); err != nil || table.Street() != user.StreetShowdown {
		t.Fatalf("Act = %v, %v", table, err)
	}

	// Ann leaves with 495 chips, Bob with 205: all the votes are back.
	left, err := tables.Leave(ctx, ann, table.ID())
	if err != nil || left.Balance != 995 || left.Table == nil {
		t.Fatalf("Leave = %+v, %v", left, err)
	}
	left, err = tables.Leave(ctx, bob, table.ID())
	if err != nil || left.Balance != 1005 || left.Table != nil {
		t.Fatalf("Leave of Bob = %+v, %v", left, err)
	}
	if _, err = tables.Table(ctx, ann, table.ID()); !errors.Is(err, user.ErrTableNotFound) {
		t.Fatalf("an empty table: %v", err)
	}

	// A private table: Eve sees and sits only once invited.
	own, err := tables.Create(ctx, ann, service.NewTableInput{Name: "Свои", Private: true, Seats: 4, BigBlind: 2, BuyIn: 100})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tables.Table(ctx, eve, own.Table.ID()); !errors.Is(err, user.ErrTableNotFound) {
		t.Fatalf("a private table to a stranger: %v", err)
	}
	if _, err = w.userSvc.UpdateProfile(ctx, eve, profile(t, "eve"), ""); err != nil {
		t.Fatal(err)
	}
	if _, err = tables.Invite(ctx, ann, own.Table.ID(), []user.Nickname{"eve", "nobody"}); err != nil {
		t.Fatal(err)
	}
	if invited, err := tables.Tables(ctx, eve, service.TablesInvited); err != nil || len(invited) != 1 {
		t.Fatalf("invited = %v, %v", invited, err)
	}
	if seated, err := tables.Sit(ctx, eve, own.Table.ID(), -1, 40); err != nil || seated.Balance != 960 {
		t.Fatalf("Sit = %+v, %v", seated, err)
	}

	// Nobody plays: the sweep folds the turns, sits one out and stands
	// them up with their chips; the other waits, then leaves.
	for range 20 {
		now = now.Add(time.Minute)
		if _, err = tables.SweepDue(ctx); err != nil {
			t.Fatal(err)
		}
	}
	table, err = tables.Table(ctx, ann, own.Table.ID())
	if err != nil || table.Players() != 1 || table.Street() != user.StreetWaiting {
		t.Fatalf("the swept table: %v, %v", table, err)
	}
	for _, id := range []uuid.UUID{ann, eve} {
		if table.SeatOf(id) >= 0 {
			if _, err = tables.Leave(ctx, id, table.ID()); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err = tables.Table(ctx, ann, own.Table.ID()); !errors.Is(err, user.ErrTableNotFound) {
		t.Fatalf("the table left: %v", err)
	}

	a, _ := w.users.GetByID(ctx, ann)
	e, _ := w.users.GetByID(ctx, eve)
	if a.Wallet().Votes+e.Wallet().Votes != 995+1000 {
		t.Fatalf("votes %d and %d", a.Wallet().Votes, e.Wallet().Votes)
	}
}
