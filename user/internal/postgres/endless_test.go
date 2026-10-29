package postgres_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/user/internal/postgres"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestWalletRepository_Endow(t *testing.T) {
	repo, pool := newRepo(t)
	wallets := postgres.NewWalletRepository(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	name := unique("endless")
	admin, err := user.New(user.Email(strings.ToUpper(name[:1]) + name[1:] + "@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	ann, err := user.New(user.Email(unique("endless") + "@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range []*user.User{admin, ann} {
		if err = repo.Save(ctx, u); err != nil {
			t.Fatal(err)
		}
	}

	emails := []string{name + "@example.com"}
	if raised, err := wallets.Endow(ctx, emails, user.EndlessVotes); err != nil || raised != 1 {
		t.Fatalf("Endow = %d, %v; want 1", raised, err)
	}
	if raised, err := wallets.Endow(ctx, emails, user.EndlessVotes); err != nil || raised != 0 {
		t.Fatalf("Endow again = %d, %v; want 0", raised, err)
	}

	for id, want := range map[*user.User]int64{admin: user.EndlessVotes, ann: 0} {
		got, err := repo.GetByID(ctx, id.ID())
		if err != nil || got.Wallet().Votes != want {
			t.Fatalf("votes of %s = %v, %v; want %d", id.Email(), got, err, want)
		}
	}

	// The treasury does not count the endless votes on the accounts.
	var treasury user.Treasury
	err = pgpool.NewTxManager(pool).WithinTx(ctx, func(ctx context.Context) error {
		treasury, err = wallets.Treasury(ctx)

		return err
	})
	if err != nil || user.IsEndless(treasury.Held) {
		t.Fatalf("Treasury = %+v, %v", treasury, err)
	}
}
