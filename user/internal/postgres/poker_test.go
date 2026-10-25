package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/user/internal/postgres"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestPokerRepository(t *testing.T) {
	repo, pool := newRepo(t)
	pokers := postgres.NewPokerRepository(pool)
	tx := pgpool.NewTxManager(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// The migration starts it as the default table; the test leaves it so.
	t.Cleanup(func() {
		if err := pokers.SetPoker(context.Background(), user.DefaultPoker()); err != nil {
			t.Error(err)
		}
	})

	if got, err := pokers.Poker(ctx); err != nil || got != user.DefaultPoker() {
		t.Fatalf("Poker = %+v, %v; want the default", got, err)
	}

	tuned, err := user.NewPoker(5, 50, [user.PokerPays]int{200, 40, 20, 4, 3, 1})
	if err != nil {
		t.Fatal(err)
	}
	if err = pokers.SetPoker(ctx, tuned); err != nil {
		t.Fatal(err)
	}
	if got, err := pokers.Poker(ctx); err != nil || got != tuned {
		t.Fatalf("Poker = %+v, %v; want the tuned one", got, err)
	}

	before, err := pokers.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = pokers.Count(ctx, 1, 30, 70); err != nil {
		t.Fatal(err)
	}
	if after, err := pokers.Stats(ctx); err != nil || after != (user.PokerStats{Hands: before.Hands + 1, Spent: before.Spent + 30, Won: before.Won + 70}) {
		t.Fatalf("Stats = %+v, %v; were %+v", after, err, before)
	}

	ann, err := user.New(user.Email(unique("poker") + "@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.Save(ctx, ann); err != nil {
		t.Fatal(err)
	}

	deck := make([]user.Card, user.DeckSize)
	for i := range deck {
		deck[i] = user.Card(user.DeckSize - 1 - i)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)

	dealt, err := tuned.Deal(uuid.New(), ann.ID(), 10, deck, now.Add(-48*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err = pokers.AddHand(ctx, dealt); err != nil {
		t.Fatal(err)
	}
	if got, err := pokers.Hand(ctx, dealt.ID()); err != nil || *got != *dealt {
		t.Fatalf("Hand = %+v, %v; want %+v", got, err, dealt)
	}
	if got, err := pokers.Dealt(ctx, ann.ID()); err != nil || got.ID() != dealt.ID() {
		t.Fatalf("Dealt = %+v, %v", got, err)
	}

	other, err := tuned.Deal(uuid.New(), ann.ID(), 10, deck, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := pokers.AddHand(ctx, other); !errors.Is(err, user.ErrPokerOpen) {
		t.Fatalf("AddHand of a second hand dealt: %v", err)
	}

	called, err := dealt.Call(tuned)
	if err != nil {
		t.Fatal(err)
	}
	err = tx.WithinTx(ctx, func(ctx context.Context) error {
		if _, err := pokers.LockHand(ctx, dealt.ID()); err != nil {
			return err
		}

		return pokers.SetOver(ctx, called)
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := pokers.Hand(ctx, dealt.ID()); err != nil || *got != *called {
		t.Fatalf("Hand after SetOver = %+v, %v; want %+v", got, err, called)
	}
	if _, err := pokers.Dealt(ctx, ann.ID()); !errors.Is(err, user.ErrPokerNotFound) {
		t.Fatalf("Dealt after the call: %v", err)
	}

	if err = pokers.AddHand(ctx, other); err != nil {
		t.Fatal(err)
	}
	if err = pokers.DeleteOver(ctx, ann.ID(), now.Add(-24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := pokers.Hand(ctx, dealt.ID()); !errors.Is(err, user.ErrPokerNotFound) {
		t.Fatalf("Hand of an old hand over: %v", err)
	}
	if _, err := pokers.Hand(ctx, other.ID()); err != nil {
		t.Fatalf("Hand of the hand dealt: %v", err)
	}
	if err := pokers.SetOver(ctx, called); !errors.Is(err, user.ErrPokerNotFound) {
		t.Fatalf("SetOver of nothing: %v", err)
	}
}
