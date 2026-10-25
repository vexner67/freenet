package postgres_test

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/user/internal/postgres"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestBankRepository(t *testing.T) {
	repo, pool := newRepo(t)
	banks := postgres.NewBankRepository(pool)
	tx := pgpool.NewTxManager(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// The migration starts it as the default bank; the test leaves it so.
	t.Cleanup(func() {
		if err := banks.SetBank(context.Background(), user.DefaultBank); err != nil {
			t.Error(err)
		}
	})

	if got, err := banks.Bank(ctx); err != nil || got != user.DefaultBank {
		t.Fatalf("Bank = %+v, %v; want the default", got, err)
	}

	tuned := user.DefaultBank
	tuned.LoanDays = 3
	if err := banks.SetBank(ctx, tuned); err != nil {
		t.Fatal(err)
	}
	if got, err := banks.Bank(ctx); err != nil || got != tuned {
		t.Fatalf("Bank = %+v, %v; want the tuned one", got, err)
	}

	if _, err := banks.Account(ctx, uuid.Must(uuid.NewV7())); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("Account of nobody: %v", err)
	}

	ann, err := user.New(user.Email(unique("bank") + "@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.Save(ctx, ann); err != nil {
		t.Fatal(err)
	}

	empty, err := banks.Account(ctx, ann.ID())
	if err != nil || empty.Deposit() != 0 || empty.HasLoan() {
		t.Fatalf("a fresh Account = %+v, %v", empty, err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	a, err := empty.Put(tuned, 50, now)
	if err != nil {
		t.Fatal(err)
	}
	if a, err = a.Borrow(tuned, 20, now.Add(-10*24*time.Hour)); err != nil {
		t.Fatal(err)
	}

	err = tx.WithinTx(ctx, func(ctx context.Context) error {
		if _, err := banks.Account(ctx, ann.ID()); err != nil {
			return err
		}

		return banks.SaveAccount(ctx, ann.ID(), a)
	})
	if err != nil {
		t.Fatal(err)
	}

	if got, err := banks.Account(ctx, ann.ID()); err != nil || !reflect.DeepEqual(got, a) {
		t.Fatalf("Account = %+v, %v; want %+v", got, err, a)
	}

	due, err := banks.Overdue(ctx, now, 1000)
	if err != nil || !slices.Contains(due, ann.ID()) {
		t.Fatalf("Overdue = %v, %v; want ann among them", due, err)
	}

	stats, err := banks.Stats(ctx)
	if err != nil || stats.Deposits < a.Deposit() || stats.Depositors < 1 || stats.Debts < a.Debt() || stats.Debtors < 1 {
		t.Fatalf("Stats = %+v, %v", stats, err)
	}
}
