package postgres_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/user/internal/postgres"
	"github.com/vexner67/freenet/user/internal/user"
)

// voucherToken is a serial number no other test run has used.
func voucherToken(t *testing.T) user.VoucherToken {
	t.Helper()

	token, err := user.NewVoucherToken(strings.ReplaceAll(uuid.NewString(), "-", "")[:user.VoucherTokenLength])
	if err != nil {
		t.Fatal(err)
	}

	return token
}

func TestWalletRepository(t *testing.T) {
	repo, pool := newRepo(t)
	wallets := postgres.NewWalletRepository(pool)
	tx := pgpool.NewTxManager(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	newUser := func() *user.User {
		t.Helper()

		u, err := user.New(user.Email(unique("wallet") + "@example.com"))
		if err != nil {
			t.Fatal(err)
		}
		if err = repo.Save(ctx, u); err != nil {
			t.Fatal(err)
		}

		return u
	}

	wallet := func(id uuid.UUID) user.Wallet {
		t.Helper()

		u, err := repo.GetByID(ctx, id)
		if err != nil {
			t.Fatal(err)
		}

		return u.Wallet()
	}

	ann, bob := newUser(), newUser()

	if wallet(ann.ID()) != (user.Wallet{}) {
		t.Fatalf("a new page has %+v", wallet(ann.ID()))
	}

	got, err := wallets.Credit(ctx, ann.ID(), 10, 30)
	if err != nil || got != (user.Wallet{Votes: 10, Rating: 30}) || wallet(ann.ID()) != got {
		t.Fatalf("Credit = %+v, %v; stored %+v", got, err, wallet(ann.ID()))
	}
	if _, err = wallets.Credit(ctx, uuid.New(), 1, 0); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("Credit of nobody: %v", err)
	}

	// Saving the profile keeps the wallet.
	stale, err := repo.GetByID(ctx, bob.ID())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = wallets.Credit(ctx, bob.ID(), 2, 0); err != nil {
		t.Fatal(err)
	}
	stale.UpdateStatus("Привет")
	if err = repo.Update(ctx, stale); err != nil {
		t.Fatal(err)
	}
	if wallet(bob.ID()).Votes != 2 {
		t.Fatalf("after Update bob has %+v", wallet(bob.ID()))
	}

	left, err := wallets.Spend(ctx, ann.ID(), 3)
	if err != nil || left != 7 {
		t.Fatalf("Spend = %d, %v", left, err)
	}
	if _, err = wallets.Spend(ctx, ann.ID(), 8); !errors.Is(err, user.ErrNotEnoughVotes) {
		t.Fatalf("Spend of more than there is: %v", err)
	}
	if _, err = wallets.Spend(ctx, uuid.New(), 1); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("Spend of nobody: %v", err)
	}
	if wallet(ann.ID()).Votes != 7 {
		t.Fatalf("after the refusals ann has %+v", wallet(ann.ID()))
	}

	move := func(from, to uuid.UUID, votes int64) (int64, error) {
		var left int64

		err := tx.WithinTx(ctx, func(ctx context.Context) error {
			var err error
			left, err = wallets.Move(ctx, from, to, votes)

			return err
		})

		return left, err
	}

	if left, err = move(ann.ID(), bob.ID(), 5); err != nil || left != 2 || wallet(bob.ID()).Votes != 7 {
		t.Fatalf("Move = %d, %v; bob has %+v", left, err, wallet(bob.ID()))
	}
	if _, err = move(ann.ID(), bob.ID(), 3); !errors.Is(err, user.ErrNotEnoughVotes) {
		t.Fatalf("Move of more than there is: %v", err)
	}
	if _, err = move(ann.ID(), uuid.New(), 1); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("Move to nobody: %v", err)
	}
	if wallet(ann.ID()).Votes != 2 || wallet(bob.ID()).Votes != 7 {
		t.Fatalf("after the refusals: ann %+v, bob %+v", wallet(ann.ID()), wallet(bob.ID()))
	}

	// Two people handing each other votes at once neither deadlock nor
	// lose any.
	var wg sync.WaitGroup
	failures := make(chan error, 40)
	for range 20 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if _, err := move(ann.ID(), bob.ID(), 1); err != nil && !errors.Is(err, user.ErrNotEnoughVotes) {
				failures <- err
			}
		}()
		go func() {
			defer wg.Done()
			if _, err := move(bob.ID(), ann.ID(), 1); err != nil && !errors.Is(err, user.ErrNotEnoughVotes) {
				failures <- err
			}
		}()
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Fatalf("a concurrent Move: %v", err)
	}
	if a, b := wallet(ann.ID()).Votes, wallet(bob.ID()).Votes; a+b != 9 || a < 0 || b < 0 {
		t.Fatalf("after concurrent moves: ann %d, bob %d", a, b)
	}
}

func TestVoucherRepository(t *testing.T) {
	repo, pool := newRepo(t)
	vouchers := postgres.NewVoucherRepository(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	newUser := func() *user.User {
		t.Helper()

		u, err := user.New(user.Email(unique("voucher") + "@example.com"))
		if err != nil {
			t.Fatal(err)
		}
		if err = repo.Save(ctx, u); err != nil {
			t.Fatal(err)
		}

		return u
	}

	save := func(votes int64, rating, usages int) *user.Voucher {
		t.Helper()

		v, err := user.NewVoucher(voucherToken(t), votes, rating, usages)
		if err != nil {
			t.Fatal(err)
		}
		if err = vouchers.Save(ctx, v); err != nil {
			t.Fatalf("Save: %v", err)
		}

		return v
	}

	ann, bob, cat := newUser(), newUser(), newUser()

	twice := save(10, 5, 2)

	again, err := user.NewVoucher(twice.Token(), 1, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err = vouchers.Save(ctx, again); !errors.Is(err, user.ErrVoucherExists) {
		t.Fatalf("the same serial number: %v", err)
	}

	got, err := vouchers.Redeem(ctx, twice.Token(), ann.ID())
	if err != nil || got.ID() != twice.ID() || got.Votes() != 10 || got.Rating() != 5 {
		t.Fatalf("Redeem = %+v, %v", got, err)
	}
	if _, err = vouchers.Redeem(ctx, twice.Token(), ann.ID()); !errors.Is(err, user.ErrVoucherInvalid) {
		t.Fatalf("the second time: %v", err)
	}
	if _, err = vouchers.Redeem(ctx, voucherToken(t), ann.ID()); !errors.Is(err, user.ErrVoucherInvalid) {
		t.Fatalf("an unknown voucher: %v", err)
	}

	// The last usage goes to one of two accounts asking at once.
	results := make(chan error, 2)
	for _, id := range []uuid.UUID{bob.ID(), cat.ID()} {
		go func() {
			_, err := vouchers.Redeem(ctx, twice.Token(), id)
			results <- err
		}()
	}
	first, second := <-results, <-results
	if (first == nil) == (second == nil) {
		t.Fatalf("the last usage: %v and %v", first, second)
	}
	if refused := errors.Join(first, second); !errors.Is(refused, user.ErrVoucherInvalid) {
		t.Fatalf("the refusal: %v", refused)
	}

	everyone := save(0, 20, user.UnlimitedUsages)
	for _, u := range []*user.User{ann, bob, cat} {
		if _, err = vouchers.Redeem(ctx, everyone.Token(), u.ID()); err != nil {
			t.Fatalf("a voucher for everyone: %v", err)
		}
	}

	// Newest first, with what is left and how many have used them. Other
	// test runs leave vouchers too: ours are the newest.
	list, err := vouchers.List(ctx, uuid.Nil, 2)
	if err != nil || len(list) != 2 {
		t.Fatalf("List = %v, %v", list, err)
	}
	if v := list[0]; v.ID() != everyone.ID() || v.UsagesLeft() != user.UnlimitedUsages || v.Activations() != 3 || v.Rating() != 20 || v.Votes() != 0 {
		t.Fatalf("the newest voucher = %+v", v)
	}
	if v := list[1]; v.ID() != twice.ID() || v.UsagesLeft() != 0 || v.Activations() != 2 || v.Token() != twice.Token() || !v.Expired() {
		t.Fatalf("the used voucher = %+v", v)
	}

	below, err := vouchers.List(ctx, everyone.ID(), 1)
	if err != nil || len(below) != 1 || below[0].ID() != twice.ID() {
		t.Fatalf("List below the newest = %v, %v", below, err)
	}

	// A deleted voucher works for nobody; a deleted user frees no usage.
	if err = vouchers.Delete(ctx, everyone.ID()); err != nil {
		t.Fatal(err)
	}
	if err = vouchers.Delete(ctx, everyone.ID()); err != nil {
		t.Fatalf("deleting twice: %v", err)
	}
	if _, err = vouchers.Redeem(ctx, everyone.Token(), newUser().ID()); !errors.Is(err, user.ErrVoucherInvalid) {
		t.Fatalf("a deleted voucher: %v", err)
	}
	if _, err = vouchers.Redeem(ctx, twice.Token(), uuid.New()); !errors.Is(err, user.ErrVoucherInvalid) {
		t.Fatalf("a voucher that ran out, by nobody: %v", err)
	}
}
