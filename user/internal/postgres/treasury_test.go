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

func TestTreasury(t *testing.T) {
	repo, pool := newRepo(t)
	wallets := postgres.NewWalletRepository(pool)
	tx := pgpool.NewTxManager(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	treasury := func() user.Treasury {
		t.Helper()

		var got user.Treasury
		// The treasury is locked, so it is read in a transaction.
		err := tx.WithinTx(ctx, func(ctx context.Context) error {
			var err error
			got, err = wallets.Treasury(ctx)

			return err
		})
		if err != nil {
			t.Fatal(err)
		}

		return got
	}

	ann, err := user.New(user.Email(unique("treasury") + "@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	bob, err := user.New(user.Email(unique("treasury") + "@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range []*user.User{ann, bob} {
		if err = repo.Save(ctx, u); err != nil {
			t.Fatal(err)
		}
	}

	start := treasury()

	// Credit takes out of it, Spend puts in, Move goes past it.
	if _, err = wallets.Credit(ctx, ann.ID(), 10, 0, user.Entry(user.VoteBonus, uuid.Nil, "")); err != nil {
		t.Fatal(err)
	}
	if _, err = wallets.Spend(ctx, ann.ID(), 3, user.Entry(user.VoteCasino, uuid.Nil, "")); err != nil {
		t.Fatal(err)
	}
	if _, err = wallets.Move(ctx, ann.ID(), bob.ID(), 2, user.Entry(user.VoteTransfer, uuid.Nil, "")); err != nil {
		t.Fatal(err)
	}
	if got := treasury(); got.Votes != start.Votes-10+3 || got.Held != start.Held+10-3 {
		t.Fatalf("Treasury = %+v; want %d votes, %d held", got, start.Votes-7, start.Held+7)
	}
	// It has no bottom: it pays more than it holds and goes below zero.
	big := treasury().Votes + 5
	if _, err = wallets.Credit(ctx, ann.ID(), big, 0, user.Entry(user.VoteBonus, uuid.Nil, "")); err != nil {
		t.Fatalf("Credit past the treasury: %v", err)
	}
	if got := treasury().Votes; got != -5 {
		t.Fatalf("Treasury after paying past it = %d; want -5", got)
	}
	if _, err = wallets.Credit(ctx, uuid.New(), 1, 0, user.Entry(user.VoteBonus, uuid.Nil, "")); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("Credit of nobody: %v", err)
	}

	// The fee and rating bought go into it, each a line of the statement.
	before := treasury().Votes
	if _, err = wallets.Spend(ctx, ann.ID(), 1, user.Entry(user.VoteFee, bob.ID(), "")); err != nil {
		t.Fatal(err)
	}
	if _, err = wallets.Spend(ctx, ann.ID(), 3, user.Entry(user.VoteRating, uuid.Nil, "1")); err != nil {
		t.Fatal(err)
	}
	if _, err = wallets.Spend(ctx, bob.ID(), 1, user.Entry(user.VoteTax, ann.ID(), "Котики")); err != nil {
		t.Fatal(err)
	}
	if got := treasury().Votes; got != before+5 {
		t.Fatalf("Treasury = %d; want %d", got, before+5)
	}

	// Rating is raised up to the limit.
	if w, err := wallets.RaiseRating(ctx, ann.ID(), user.MaxRating-1); err != nil || w.Rating != user.MaxRating-1 {
		t.Fatalf("RaiseRating = %+v, %v", w, err)
	}
	if _, err = wallets.RaiseRating(ctx, ann.ID(), 2); !errors.Is(err, user.ErrRatingLimit) {
		t.Fatalf("RaiseRating past the limit: %v", err)
	}
	if _, err = wallets.RaiseRating(ctx, uuid.New(), 1); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("RaiseRating of nobody: %v", err)
	}

	// A bonus once a day.
	day := user.BonusDay(time.Now())
	if last, err := wallets.LastBonus(ctx, ann.ID()); err != nil || !last.IsZero() {
		t.Fatalf("LastBonus of a new page = %v, %v", last, err)
	}
	if err = wallets.TakeBonus(ctx, ann.ID(), day); err != nil {
		t.Fatal(err)
	}
	if err = wallets.TakeBonus(ctx, ann.ID(), day); !errors.Is(err, user.ErrBonusTaken) {
		t.Fatalf("TakeBonus twice: %v", err)
	}
	next := day.AddDate(0, 0, 1)
	if err = wallets.TakeBonus(ctx, ann.ID(), next); err != nil {
		t.Fatal(err)
	}
	if last, err := wallets.LastBonus(ctx, ann.ID()); err != nil || !last.Equal(next) {
		t.Fatalf("LastBonus = %v, %v; want %v", last, err, next)
	}
}
