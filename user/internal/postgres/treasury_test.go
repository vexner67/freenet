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

	// Mint brings the votes of a member in once.
	start := treasury()
	if err = wallets.Mint(ctx, ann.ID()); err != nil {
		t.Fatal(err)
	}
	if err = wallets.Mint(ctx, ann.ID()); err != nil {
		t.Fatal(err)
	}
	if err = wallets.Mint(ctx, uuid.New()); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("Mint of nobody: %v", err)
	}
	minted := treasury()
	if minted.Votes != start.Votes+user.MemberMint || minted.Minted != start.Minted+user.MemberMint {
		t.Fatalf("Treasury after Mint = %+v; was %+v", minted, start)
	}

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
	if got := treasury(); got.Votes != minted.Votes-10+3 {
		t.Fatalf("Treasury = %+v; want %d votes", got, minted.Votes-7)
	}
	if _, err = wallets.Credit(ctx, ann.ID(), treasury().Votes+1, 0, user.Entry(user.VoteBonus, uuid.Nil, "")); !errors.Is(err, user.ErrTreasuryEmpty) {
		t.Fatalf("Credit past the treasury: %v", err)
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
