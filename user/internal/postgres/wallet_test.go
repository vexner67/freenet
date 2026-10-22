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

	got, err := wallets.Credit(ctx, ann.ID(), 10, 30, user.Entry(user.VoteVoucher, uuid.Nil, ""))
	if err != nil || got != (user.Wallet{Votes: 10, Rating: 30}) || wallet(ann.ID()) != got {
		t.Fatalf("Credit = %+v, %v; stored %+v", got, err, wallet(ann.ID()))
	}
	if _, err = wallets.Credit(ctx, uuid.New(), 1, 0, user.Entry(user.VoteVoucher, uuid.Nil, "")); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("Credit of nobody: %v", err)
	}

	// Saving the profile keeps the wallet.
	stale, err := repo.GetByID(ctx, bob.ID())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = wallets.Credit(ctx, bob.ID(), 2, 0, user.Entry(user.VoteVoucher, uuid.Nil, "")); err != nil {
		t.Fatal(err)
	}
	stale.UpdateStatus("Привет")
	if err = repo.Update(ctx, stale); err != nil {
		t.Fatal(err)
	}
	if wallet(bob.ID()).Votes != 2 {
		t.Fatalf("after Update bob has %+v", wallet(bob.ID()))
	}

	left, err := wallets.Spend(ctx, ann.ID(), 3, user.Entry(user.VoteVoucher, uuid.Nil, ""))
	if err != nil || left != 7 {
		t.Fatalf("Spend = %d, %v", left, err)
	}
	if _, err = wallets.Spend(ctx, ann.ID(), 8, user.Entry(user.VoteVoucher, uuid.Nil, "")); !errors.Is(err, user.ErrNotEnoughVotes) {
		t.Fatalf("Spend of more than there is: %v", err)
	}
	if _, err = wallets.Spend(ctx, uuid.New(), 1, user.Entry(user.VoteVoucher, uuid.Nil, "")); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("Spend of nobody: %v", err)
	}
	if wallet(ann.ID()).Votes != 7 {
		t.Fatalf("after the refusals ann has %+v", wallet(ann.ID()))
	}

	move := func(from, to uuid.UUID, votes int64) (int64, error) {
		var left int64

		err := tx.WithinTx(ctx, func(ctx context.Context) error {
			var err error
			left, err = wallets.Move(ctx, from, to, votes, user.Entry(user.VoteVoucher, uuid.Nil, ""))

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

func TestWalletRepository_Statement(t *testing.T) {
	repo, pool := newRepo(t)
	wallets := postgres.NewWalletRepository(pool)
	tx := pgpool.NewTxManager(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	newUser := func() *user.User {
		t.Helper()

		u, err := user.New(user.Email(unique("statement") + "@example.com"))
		if err != nil {
			t.Fatal(err)
		}
		if err = repo.Save(ctx, u); err != nil {
			t.Fatal(err)
		}

		return u
	}

	ann, bob := newUser(), newUser()

	// Rating alone changes no votes and is not written down.
	if _, err := wallets.Credit(ctx, ann.ID(), 0, 5, user.Entry(user.VoteVoucher, uuid.Nil, "")); err != nil {
		t.Fatal(err)
	}
	if _, err := wallets.Credit(ctx, ann.ID(), 20, 0, user.Entry(user.VoteVoucher, uuid.Nil, "")); err != nil {
		t.Fatal(err)
	}
	if _, err := wallets.Spend(ctx, ann.ID(), 3, user.Entry(user.VoteGift, bob.ID(), "freenet_coin")); err != nil {
		t.Fatal(err)
	}
	// A refused spend writes nothing.
	if _, err := wallets.Spend(ctx, ann.ID(), 100, user.Entry(user.VoteCasino, uuid.Nil, "")); !errors.Is(err, user.ErrNotEnoughVotes) {
		t.Fatal(err)
	}
	err := tx.WithinTx(ctx, func(ctx context.Context) error {
		_, err := wallets.Move(ctx, ann.ID(), bob.ID(), 7, user.Entry(user.VoteTransfer, uuid.Nil, "за пиццу"))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	ops, err := wallets.Operations(ctx, ann.ID(), uuid.Nil, user.VoteFlowAll, 10)
	if err != nil || len(ops) != 3 {
		t.Fatalf("Operations = %+v, %v", ops, err)
	}

	want := []struct {
		kind            user.VoteKind
		amount, balance int64
		peer            uuid.UUID
		note            string
	}{
		{user.VoteTransfer, -7, 10, bob.ID(), "за пиццу"},
		{user.VoteGift, -3, 17, bob.ID(), "freenet_coin"},
		{user.VoteVoucher, 20, 20, uuid.Nil, ""},
	}
	for i, w := range want {
		op := ops[i]
		if op.Kind != w.kind || op.Amount != w.amount || op.Balance != w.balance || op.PeerID != w.peer || op.Note != w.note || op.At.IsZero() {
			t.Fatalf("operation %d = %+v, want %+v", i, op, w)
		}
	}

	bobs, err := wallets.Operations(ctx, bob.ID(), uuid.Nil, user.VoteFlowAll, 10)
	if err != nil || len(bobs) != 1 || bobs[0].Amount != 7 || bobs[0].Balance != 7 || bobs[0].PeerID != ann.ID() {
		t.Fatalf("bob's operations = %+v, %v", bobs, err)
	}

	in, err := wallets.Operations(ctx, ann.ID(), uuid.Nil, user.VoteFlowIn, 10)
	if err != nil || len(in) != 1 || in[0].Kind != user.VoteVoucher {
		t.Fatalf("what came in = %+v, %v", in, err)
	}
	older, err := wallets.Operations(ctx, ann.ID(), ops[0].ID, user.VoteFlowOut, 10)
	if err != nil || len(older) != 1 || older[0].Kind != user.VoteGift {
		t.Fatalf("what went out before the transfer = %+v, %v", older, err)
	}

	income, expense, err := wallets.Totals(ctx, ann.ID(), time.Now().Add(-time.Hour))
	if err != nil || income != 20 || expense != 10 {
		t.Fatalf("Totals = %d, %d, %v", income, expense, err)
	}
	if income, expense, err = wallets.Totals(ctx, ann.ID(), time.Now().Add(time.Hour)); err != nil || income != 0 || expense != 0 {
		t.Fatalf("Totals from the future = %d, %d, %v", income, expense, err)
	}
}
