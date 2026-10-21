package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/inmem"
	"github.com/vexner67/freenet/user/internal/purgeclient"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

// fakeVotesNotifier records the transfers told to the notification
// service, and fails with err when set.
type fakeVotesNotifier struct {
	sent []giftNote
	err  error
}

func (f *fakeVotesNotifier) VotesSent(_ context.Context, transfer, sender, recipient uuid.UUID) error {
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, giftNote{transfer, sender, recipient})
	return nil
}

type walletWorld struct {
	users    *inmem.UserRepository
	userSvc  *service.UserService
	accounts *service.AccountService
	blocks   *service.BlockService
	gifts    *service.GiftService
	wallets  *service.WalletService
	notifier *fakeVotesNotifier
}

// newWalletWorld is a site that admin@example.com runs.
func newWalletWorld(t *testing.T) walletWorld {
	t.Helper()

	users := inmem.NewUserRepository()
	follows := inmem.NewFollowRepository(users)
	blocks := inmem.NewBlockRepository(follows)
	permissions := service.NewPermissionService(users, follows, blocks)
	notifier := &fakeVotesNotifier{}

	admins, err := user.NewAdmins([]string{"admin@example.com"})
	if err != nil {
		t.Fatal(err)
	}

	gifts := inmem.NewGiftRepository(users)

	return walletWorld{
		users:    users,
		userSvc:  service.NewUserService(users),
		accounts: service.NewAccountService(users, inmem.TxManager{}, purgeclient.NoReactions{}, time.Hour),
		blocks:   service.NewBlockService(users, blocks, inmem.TxManager{}, &fakeNotifier{}),
		gifts: service.NewGiftService(service.GiftDeps{
			Users: users, Gifts: gifts, Prices: gifts, Permissions: permissions,
			Tx: inmem.TxManager{}, Wallets: users, Notifier: &fakeGiftNotifier{}, Admins: admins,
		}),
		wallets: service.NewWalletService(service.WalletDeps{
			Users: users, Wallets: users, Statements: users, Bonuses: users, Vouchers: inmem.NewVoucherRepository(users), Rates: inmem.NewVoteRateRepository(), Permissions: permissions,
			Tx: inmem.TxManager{}, Notifier: notifier, Admins: admins,
		}),
		notifier: notifier,
	}
}

func (w walletWorld) person(t *testing.T, name string) uuid.UUID {
	t.Helper()

	id, err := w.userSvc.GetOrCreateByEmail(context.Background(), mustEmail(t, name+"@example.com"))
	if err != nil {
		t.Fatal(err)
	}

	return id
}

func (w walletWorld) votes(t *testing.T, id uuid.UUID) int64 {
	t.Helper()

	u, err := w.users.GetByID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}

	return u.Wallet().Votes
}

func TestWalletService_Vouchers(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newWalletWorld(t)
	admin, ann, bob, cat := w.person(t, "admin"), w.person(t, "ann"), w.person(t, "bob"), w.person(t, "cat")

	// Only the administrators make, read and delete vouchers.
	if _, err := w.wallets.CreateVoucher(ctx, ann, service.VoucherDraft{Votes: 5, Usages: 1}); !errors.Is(err, user.ErrNotAdmin) {
		t.Fatalf("CreateVoucher by ann: %v", err)
	}
	if _, err := w.wallets.ListVouchers(ctx, ann, uuid.Nil, 0); !errors.Is(err, user.ErrNotAdmin) {
		t.Fatalf("ListVouchers by ann: %v", err)
	}
	if err := w.wallets.DeleteVoucher(ctx, ann, uuid.New()); !errors.Is(err, user.ErrNotAdmin) {
		t.Fatalf("DeleteVoucher by ann: %v", err)
	}

	// A generated serial number, two usages.
	twice, err := w.wallets.CreateVoucher(ctx, admin, service.VoucherDraft{Votes: 10, Rating: 5, Usages: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(twice.Token()) != user.VoucherTokenLength || twice.UsagesLeft() != 2 {
		t.Fatalf("voucher = %+v", twice)
	}

	if _, err = w.wallets.CreateVoucher(ctx, admin, service.VoucherDraft{Token: twice.Token(), Votes: 1, Usages: 1}); !errors.Is(err, user.ErrVoucherExists) {
		t.Fatalf("the same serial number: %v", err)
	}
	if _, err = w.wallets.CreateVoucher(ctx, admin, service.VoucherDraft{Usages: 1}); !errors.Is(err, user.ErrVoucherEmpty) {
		t.Fatalf("a voucher of nothing: %v", err)
	}

	got, err := w.wallets.Redeem(ctx, ann, twice.Token())
	if err != nil {
		t.Fatal(err)
	}
	if got.Votes != 10 || got.Rating != 5 || got.Wallet != (user.Wallet{Votes: 10, Rating: 5}) {
		t.Fatalf("redeemed = %+v", got)
	}

	// Once per account.
	if _, err = w.wallets.Redeem(ctx, ann, twice.Token()); !errors.Is(err, user.ErrVoucherInvalid) {
		t.Fatalf("the second time: %v", err)
	}
	if _, err = w.wallets.Redeem(ctx, bob, twice.Token()); err != nil {
		t.Fatalf("the second account: %v", err)
	}
	// It has run out.
	if _, err = w.wallets.Redeem(ctx, cat, twice.Token()); !errors.Is(err, user.ErrVoucherInvalid) {
		t.Fatalf("the third account: %v", err)
	}
	if _, err = w.wallets.Redeem(ctx, cat, "ZZZZZZZZZZZZZZZZZZZZZZZZ"); !errors.Is(err, user.ErrVoucherInvalid) {
		t.Fatalf("an unknown voucher: %v", err)
	}
	if w.votes(t, ann) != 10 || w.votes(t, bob) != 10 || w.votes(t, cat) != 0 {
		t.Fatalf("balances: %d %d %d", w.votes(t, ann), w.votes(t, bob), w.votes(t, cat))
	}

	// A named voucher for everyone.
	const named = user.VoucherToken("WELCOME00000000000000000")
	if _, err = w.wallets.CreateVoucher(ctx, admin, service.VoucherDraft{Token: named, Rating: 20, Usages: user.UnlimitedUsages}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []uuid.UUID{ann, bob, cat} {
		if _, err = w.wallets.Redeem(ctx, id, named); err != nil {
			t.Fatalf("a voucher for everyone: %v", err)
		}
	}

	// Newest first, with what is left and who has used them.
	first, err := w.wallets.ListVouchers(ctx, admin, uuid.Nil, 1)
	if err != nil || len(first.Vouchers) != 1 || first.NextCursor == uuid.Nil {
		t.Fatalf("the first page = %+v, %v", first, err)
	}
	if v := first.Vouchers[0]; v.Token() != named || v.UsagesLeft() != user.UnlimitedUsages || v.Activations() != 3 || v.Expired() {
		t.Fatalf("the newest voucher = %+v", v)
	}
	second, err := w.wallets.ListVouchers(ctx, admin, first.NextCursor, 1)
	if err != nil || len(second.Vouchers) != 1 || second.NextCursor != uuid.Nil {
		t.Fatalf("the second page = %+v, %v", second, err)
	}
	if v := second.Vouchers[0]; v.ID() != twice.ID() || v.UsagesLeft() != 0 || v.Activations() != 2 || !v.Expired() {
		t.Fatalf("the used voucher = %+v", v)
	}

	// A deleted voucher works for nobody; what it gave stays.
	fresh, err := w.wallets.CreateVoucher(ctx, admin, service.VoucherDraft{Votes: 1, Usages: 5})
	if err != nil {
		t.Fatal(err)
	}
	if err = w.wallets.DeleteVoucher(ctx, admin, fresh.ID()); err != nil {
		t.Fatal(err)
	}
	if err = w.wallets.DeleteVoucher(ctx, admin, fresh.ID()); err != nil {
		t.Fatalf("deleting twice: %v", err)
	}
	if _, err = w.wallets.Redeem(ctx, cat, fresh.Token()); !errors.Is(err, user.ErrVoucherInvalid) {
		t.Fatalf("a deleted voucher: %v", err)
	}
	if err = w.wallets.DeleteVoucher(ctx, admin, twice.ID()); err != nil || w.votes(t, ann) != 10 {
		t.Fatalf("after deleting the used voucher ann has %d: %v", w.votes(t, ann), err)
	}
}

func TestWalletService_Transfer(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newWalletWorld(t)
	admin, ann, bob, cat := w.person(t, "admin"), w.person(t, "ann"), w.person(t, "bob"), w.person(t, "cat")

	voucher, err := w.wallets.CreateVoucher(ctx, admin, service.VoucherDraft{Votes: 10, Usages: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.wallets.Redeem(ctx, ann, voucher.Token()); err != nil {
		t.Fatal(err)
	}

	// The sender pays the fee on top: 4 and 1.
	left, err := w.wallets.Transfer(ctx, ann, bob, 4, "")
	if err != nil || left != 5 || w.votes(t, ann) != 5 || w.votes(t, bob) != 4 {
		t.Fatalf("Transfer = %d, %v; ann %d, bob %d", left, err, w.votes(t, ann), w.votes(t, bob))
	}
	if len(w.notifier.sent) != 1 || w.notifier.sent[0].sender != ann || w.notifier.sent[0].recipient != bob || w.notifier.sent[0].gift == uuid.Nil {
		t.Fatalf("told = %+v", w.notifier.sent)
	}

	tests := []struct {
		name     string
		from, to uuid.UUID
		votes    user.Votes
		want     error
	}{
		{"more than there is", ann, bob, 6, user.ErrNotEnoughVotes},
		{"not the fee", ann, bob, 5, user.ErrNotEnoughVotes},
		{"with nothing", cat, bob, 1, user.ErrNotEnoughVotes},
		{"to oneself", ann, ann, 1, user.ErrTransferSelf},
		{"to nobody", ann, uuid.New(), 1, user.ErrNotFound},
	}
	for _, tt := range tests {
		if _, err = w.wallets.Transfer(ctx, tt.from, tt.to, tt.votes, ""); !errors.Is(err, tt.want) {
			t.Errorf("%s: %v, want %v", tt.name, err, tt.want)
		}
	}

	// The blacklist closes it both ways.
	if err = w.blocks.Block(ctx, cat, ann); err != nil {
		t.Fatal(err)
	}
	if _, err = w.wallets.Transfer(ctx, ann, cat, 1, ""); !errors.Is(err, user.ErrTransferRefused) {
		t.Fatalf("to the one who blocked: %v", err)
	}
	if _, err = w.wallets.Transfer(ctx, bob, cat, 1, ""); err != nil {
		t.Fatalf("bob to cat: %v", err)
	}
	if _, err = w.wallets.Transfer(ctx, cat, ann, 1, ""); !errors.Is(err, user.ErrTransferRefused) {
		t.Fatalf("to the blocked one: %v", err)
	}

	// A deleted page takes nothing.
	if _, err = w.accounts.Deactivate(ctx, bob, ""); err != nil {
		t.Fatal(err)
	}
	if _, err = w.wallets.Transfer(ctx, ann, bob, 1, ""); !errors.Is(err, user.ErrDeactivated) {
		t.Fatalf("to a deleted page: %v", err)
	}

	// A transfer that was not recorded for the notification service did
	// not tell it either; in memory the votes are not undone.
	w.notifier.err = errors.New("outbox is down")
	if _, err = w.wallets.Transfer(ctx, cat, admin, 1, ""); err == nil {
		t.Fatal("a transfer nobody was told of succeeded")
	}
	if len(w.notifier.sent) != 2 {
		t.Fatalf("told = %+v", w.notifier.sent)
	}
}

func TestGiftService_Prices(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newWalletWorld(t)
	admin, ann, bob := w.person(t, "admin"), w.person(t, "ann"), w.person(t, "bob")

	var free, paid user.GiftKind
	for _, c := range user.GiftCatalog() {
		if c.Price == 0 {
			free = c.Gifts[0]
		} else if paid == "" || c.Price < paid.DefaultPrice() {
			paid = c.Gifts[0]
		}
	}
	price := paid.DefaultPrice().Int64()

	if _, err := w.gifts.Send(ctx, ann, bob, service.GiftDraft{Kind: free}); err != nil {
		t.Fatalf("a free gift with no votes: %v", err)
	}
	if _, err := w.gifts.Send(ctx, ann, bob, service.GiftDraft{Kind: paid}); !errors.Is(err, user.ErrNotEnoughVotes) {
		t.Fatalf("a paid gift with no votes: %v", err)
	}
	if page, err := w.gifts.List(ctx, bob, bob, uuid.Nil, 0); err != nil || page.Total != 1 {
		t.Fatalf("after the refusal bob has %+v, %v", page, err)
	}

	voucher, err := w.wallets.CreateVoucher(ctx, admin, service.VoucherDraft{Votes: price + 1, Usages: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.wallets.Redeem(ctx, ann, voucher.Token()); err != nil {
		t.Fatal(err)
	}

	if _, err = w.gifts.Send(ctx, ann, bob, service.GiftDraft{Kind: paid}); err != nil {
		t.Fatalf("a paid gift: %v", err)
	}
	if w.votes(t, ann) != 1 {
		t.Fatalf("after a gift of %d ann has %d", price, w.votes(t, ann))
	}
	if price > 1 {
		if _, err = w.gifts.Send(ctx, ann, bob, service.GiftDraft{Kind: paid}); !errors.Is(err, user.ErrNotEnoughVotes) {
			t.Fatalf("the second paid gift: %v", err)
		}
	}
}

func TestWalletService_Rate(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newWalletWorld(t)
	admin, ann := w.person(t, "admin"), w.person(t, "ann")

	if rate, err := w.wallets.Rate(ctx); err != nil || rate != user.DefaultVoteRate {
		t.Fatalf("Rate = %d, %v; want the default", rate, err)
	}

	// Only the administrators say what a vote is worth.
	if err := w.wallets.SetRate(ctx, ann, 1000); !errors.Is(err, user.ErrNotAdmin) {
		t.Fatalf("SetRate by ann: %v", err)
	}
	if err := w.wallets.SetRate(ctx, admin, 1250); err != nil {
		t.Fatal(err)
	}
	if rate, err := w.wallets.Rate(ctx); err != nil || rate.Kopecks() != 1250 {
		t.Fatalf("Rate = %d, %v; want 1250", rate, err)
	}
}

func TestWalletService_Statement(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newWalletWorld(t)
	admin, ann, bob := w.person(t, "admin"), w.person(t, "ann"), w.person(t, "bob")

	voucher, err := w.wallets.CreateVoucher(ctx, admin, service.VoucherDraft{Votes: 10, Rating: 5, Usages: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = w.wallets.Redeem(ctx, ann, voucher.Token()); err != nil {
		t.Fatal(err)
	}
	if _, err = w.wallets.Transfer(ctx, ann, bob, 4, "За пиццу"); err != nil {
		t.Fatal(err)
	}

	page, err := w.wallets.Statement(ctx, ann, uuid.Nil, user.VoteFlowAll, 0, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	ops := page.Operations
	if len(ops) != 3 || page.NextCursor != uuid.Nil || page.Income != 10 || page.Expense != 5 {
		t.Fatalf("Statement = %+v", page)
	}
	if ops[0].Kind != user.VoteTransfer || ops[0].Amount != -4 || ops[0].Balance != 5 || ops[0].PeerID != bob || ops[0].Note != "За пиццу" {
		t.Fatalf("the transfer = %+v", ops[0])
	}
	if ops[1].Kind != user.VoteFee || ops[1].Amount != -1 || ops[1].Balance != 9 || ops[1].PeerID != bob {
		t.Fatalf("the fee = %+v", ops[1])
	}
	if ops[2].Kind != user.VoteVoucher || ops[2].Amount != 10 || ops[2].Balance != 10 || ops[2].PeerID != uuid.Nil {
		t.Fatalf("the voucher = %+v", ops[2])
	}

	// The recipient sees the transfer the other way.
	page, err = w.wallets.Statement(ctx, bob, uuid.Nil, user.VoteFlowAll, 0, time.Now())
	if err != nil || len(page.Operations) != 1 {
		t.Fatalf("bob's statement = %+v, %v", page, err)
	}
	if op := page.Operations[0]; op.Amount != 4 || op.Balance != 4 || op.PeerID != ann || op.Note != "За пиццу" {
		t.Fatalf("bob's transfer = %+v", op)
	}

	// Pages and flows.
	page, err = w.wallets.Statement(ctx, ann, uuid.Nil, user.VoteFlowAll, 1, time.Now())
	if err != nil || len(page.Operations) != 1 || page.NextCursor != page.Operations[0].ID {
		t.Fatalf("first page = %+v, %v", page, err)
	}
	page, err = w.wallets.Statement(ctx, ann, page.NextCursor, user.VoteFlowAll, 1, time.Now())
	if err != nil || len(page.Operations) != 1 || page.Operations[0].Kind != user.VoteFee || page.NextCursor == uuid.Nil {
		t.Fatalf("second page = %+v, %v", page, err)
	}
	for flow, want := range map[user.VoteFlow][]user.VoteKind{user.VoteFlowIn: {user.VoteVoucher}, user.VoteFlowOut: {user.VoteTransfer, user.VoteFee}} {
		page, err = w.wallets.Statement(ctx, ann, uuid.Nil, flow, 0, time.Now())
		if err != nil || len(page.Operations) != len(want) || page.Operations[0].Kind != want[0] {
			t.Fatalf("flow %q = %+v, %v", flow, page, err)
		}
	}

	// The totals are of the last 30 days.
	page, err = w.wallets.Statement(ctx, ann, uuid.Nil, user.VoteFlowAll, 0, time.Now().Add(31*24*time.Hour))
	if err != nil || page.Income != 0 || page.Expense != 0 {
		t.Fatalf("a month later = %+v, %v", page, err)
	}

	// A gift is written down with its recipient and kind.
	if _, err = w.gifts.Send(ctx, ann, bob, service.GiftDraft{Kind: "cake"}); err != nil {
		t.Fatal(err)
	}
	page, err = w.wallets.Statement(ctx, ann, uuid.Nil, user.VoteFlowAll, 1, time.Now())
	if err != nil || len(page.Operations) != 1 {
		t.Fatal(page, err)
	}
	if op := page.Operations[0]; op.Kind != user.VoteGift || op.PeerID != bob || op.Note != "cake" || op.Amount >= 0 {
		t.Fatalf("the gift = %+v", op)
	}
}
