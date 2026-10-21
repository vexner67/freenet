package inmem

import (
	"bytes"
	"context"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// Spend, Move and Credit make UserRepository the wallets as well: the
// votes and the rating are kept with the users, as in the table, and so
// is the statement of each page.
func (r *UserRepository) Spend(ctx context.Context, userID uuid.UUID, votes int64, e user.VoteEntry) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	left, err := r.spend(userID, votes, e)
	if err == nil {
		r.treasury.Votes += votes
	}

	return left, err
}

func (r *UserRepository) Move(ctx context.Context, fromID, toID uuid.UUID, votes int64, e user.VoteEntry) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.users[toID]; !ok {
		return 0, user.ErrNotFound
	}

	left, err := r.spend(fromID, votes, user.Entry(e.Kind, toID, e.Note))
	if err != nil {
		return 0, err
	}

	if _, err = r.credit(toID, votes, 0, user.Entry(e.Kind, fromID, e.Note)); err != nil {
		return 0, err
	}

	return left, nil
}

func (r *UserRepository) Credit(ctx context.Context, userID uuid.UUID, votes int64, rating int, e user.VoteEntry) (user.Wallet, error) {
	if err := ctx.Err(); err != nil {
		return user.Wallet{}, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.users[userID]; !ok {
		return user.Wallet{}, user.ErrNotFound
	}
	if r.treasury.Votes < votes {
		return user.Wallet{}, user.ErrTreasuryEmpty
	}

	wallet, err := r.credit(userID, votes, rating, e)
	if err == nil {
		r.treasury.Votes -= votes
	}

	return wallet, err
}

func (r *UserRepository) RaiseRating(ctx context.Context, userID uuid.UUID, percent int) (user.Wallet, error) {
	if err := ctx.Err(); err != nil {
		return user.Wallet{}, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	u, ok := r.users[userID]
	if !ok {
		return user.Wallet{}, user.ErrNotFound
	}

	wallet := u.Wallet()
	if wallet.Rating+percent > user.MaxRating {
		return user.Wallet{}, user.ErrRatingLimit
	}

	wallet.Rating += percent
	r.users[userID] = *u.WithWallet(wallet)

	return wallet, nil
}

// Treasury is the treasury with what the vouchers promise and what the
// pages hold.
func (r *UserRepository) Treasury(ctx context.Context) (user.Treasury, error) {
	if err := ctx.Err(); err != nil {
		return user.Treasury{}, errs.Wrap("ctx.Err", err)
	}

	promised := r.promised()

	r.mu.RLock()
	defer r.mu.RUnlock()

	t := r.treasury
	t.Outstanding = promised
	for _, u := range r.users {
		t.Held += u.Wallet().Votes
	}

	return t, nil
}

// Mint adds user.MemberMint to the treasury for userID, the first time.
func (r *UserRepository) Mint(ctx context.Context, userID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.users[userID]; !ok {
		return user.ErrNotFound
	}
	if _, ok := r.members[userID]; ok {
		return nil
	}

	r.members[userID] = struct{}{}
	r.treasury.Votes += user.MemberMint
	r.treasury.Minted += user.MemberMint

	return nil
}

// TakeBonus marks the bonus of day taken by userID; user.ErrBonusTaken.
func (r *UserRepository) TakeBonus(ctx context.Context, userID uuid.UUID, day time.Time) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.users[userID]; !ok {
		return user.ErrNotFound
	}
	if last, ok := r.bonuses[userID]; ok && !last.Before(day) {
		return user.ErrBonusTaken
	}

	r.bonuses[userID] = day

	return nil
}

// LastBonus is the day userID took the last bonus on, zero for never.
func (r *UserRepository) LastBonus(ctx context.Context, userID uuid.UUID) (time.Time, error) {
	if err := ctx.Err(); err != nil {
		return time.Time{}, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.bonuses[userID], nil
}

// Fund adds votes to the treasury, as if they had been spent: for tests
// that pay more than the treasury starts with.
func (r *UserRepository) Fund(votes int64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.treasury.Votes += votes
}

// Operations are the statement of userID, newest first.
func (r *UserRepository) Operations(ctx context.Context, userID, before uuid.UUID, flow user.VoteFlow, limit int) ([]user.VoteOperation, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	ops := r.operations[userID]

	var found []user.VoteOperation

	for i := len(ops) - 1; i >= 0 && len(found) < limit; i-- {
		op := ops[i]
		if (before == uuid.Nil || bytes.Compare(op.ID[:], before[:]) < 0) && flow.Matches(op.Amount) {
			found = append(found, op)
		}
	}

	return found, nil
}

func (r *UserRepository) Totals(ctx context.Context, userID uuid.UUID, since time.Time) (income, expense int64, err error) {
	if err = ctx.Err(); err != nil {
		return 0, 0, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, op := range r.operations[userID] {
		switch {
		case op.At.Before(since), op.Kind == user.VoteDenomination:
		case op.Amount > 0:
			income += op.Amount
		default:
			expense -= op.Amount
		}
	}

	return income, expense, nil
}

// spend takes votes from userID; the caller holds the write lock.
func (r *UserRepository) spend(userID uuid.UUID, votes int64, e user.VoteEntry) (int64, error) {
	u, ok := r.users[userID]
	if !ok {
		return 0, user.ErrNotFound
	}

	wallet := u.Wallet()
	if wallet.Votes < votes {
		return 0, user.ErrNotEnoughVotes
	}

	wallet.Votes -= votes
	r.users[userID] = *u.WithWallet(wallet)

	return wallet.Votes, r.write(userID, -votes, wallet.Votes, e)
}

// credit adds to the wallet of userID; the caller holds the write lock.
func (r *UserRepository) credit(userID uuid.UUID, votes int64, rating int, e user.VoteEntry) (user.Wallet, error) {
	u, ok := r.users[userID]
	if !ok {
		return user.Wallet{}, user.ErrNotFound
	}

	wallet := u.Wallet()
	wallet.Votes += votes
	wallet.Rating += rating
	r.users[userID] = *u.WithWallet(wallet)

	return wallet, r.write(userID, votes, wallet.Votes, e)
}

// write adds a line to the statement of userID unless no votes changed;
// the caller holds the write lock.
func (r *UserRepository) write(userID uuid.UUID, amount, balance int64, e user.VoteEntry) error {
	if amount == 0 {
		return nil
	}

	id, err := uuid.NewV7()
	if err != nil {
		return errs.Wrap("uuid.NewV7", err)
	}

	r.operations[userID] = append(r.operations[userID], user.VoteOperation{
		ID:      id,
		Kind:    e.Kind,
		Amount:  amount,
		Balance: balance,
		PeerID:  e.PeerID,
		Note:    e.Note,
		At:      time.Now().UTC(),
	})

	return nil
}

// VoucherRepository keeps vouchers in memory and behaves as the tables
// do: a serial number is used once, an account uses a voucher once.
type VoucherRepository struct {
	mu       sync.Mutex
	vouchers map[uuid.UUID]user.StoredVoucher
	used     map[uuid.UUID]map[uuid.UUID]struct{}
	users    *UserRepository
}

func NewVoucherRepository(users *UserRepository) *VoucherRepository {
	r := &VoucherRepository{
		vouchers: make(map[uuid.UUID]user.StoredVoucher),
		used:     make(map[uuid.UUID]map[uuid.UUID]struct{}),
		users:    users,
	}
	users.onDelete(r.forget)
	users.mu.Lock()
	users.promised = r.promised
	users.mu.Unlock()

	return r
}

// promised is what the vouchers still promise: votes times usages left.
func (r *VoucherRepository) promised() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()

	var total int64
	for _, s := range r.vouchers {
		if s.UsagesLeft > 0 {
			total += s.Votes * int64(s.UsagesLeft)
		}
	}

	return total
}

// forget drops the usages of a deleted user.
func (r *VoucherRepository) forget(userID uuid.UUID) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, users := range r.used {
		delete(users, userID)
	}
}

func (r *VoucherRepository) Save(ctx context.Context, v *user.Voucher) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, s := range r.vouchers {
		if s.Token == v.Token() {
			return user.ErrVoucherExists
		}
	}

	r.vouchers[v.ID()] = user.StoredVoucher{
		ID:         v.ID(),
		Token:      v.Token(),
		Votes:      v.Votes(),
		Rating:     v.Rating(),
		UsagesLeft: v.UsagesLeft(),
	}
	r.used[v.ID()] = make(map[uuid.UUID]struct{})

	return nil
}

func (r *VoucherRepository) Redeem(ctx context.Context, token user.VoucherToken, userID uuid.UUID) (*user.Voucher, error) {
	if _, err := r.users.GetByID(ctx, userID); err != nil {
		return nil, errs.Wrap("r.users.GetByID", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for id, s := range r.vouchers {
		if s.Token != token {
			continue
		}

		if _, used := r.used[id][userID]; used || s.UsagesLeft == 0 {
			return nil, user.ErrVoucherInvalid
		}

		if s.UsagesLeft != user.UnlimitedUsages {
			s.UsagesLeft--
		}

		r.used[id][userID] = struct{}{}
		r.vouchers[id] = s

		return r.restore(s), nil
	}

	return nil, user.ErrVoucherInvalid
}

func (r *VoucherRepository) List(ctx context.Context, before uuid.UUID, limit int) ([]*user.Voucher, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	var found []*user.Voucher

	for id, s := range r.vouchers {
		if before == uuid.Nil || bytes.Compare(id[:], before[:]) < 0 {
			found = append(found, r.restore(s))
		}
	}

	slices.SortFunc(found, func(a, b *user.Voucher) int {
		aID, bID := a.ID(), b.ID()
		return bytes.Compare(bID[:], aID[:])
	})

	return found[:min(len(found), limit)], nil
}

func (r *VoucherRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.vouchers, id)
	delete(r.used, id)

	return nil
}

// restore is the voucher with its count of usages; the caller holds the
// lock.
func (r *VoucherRepository) restore(s user.StoredVoucher) *user.Voucher {
	s.Activations = len(r.used[s.ID])

	return user.RestoreVoucher(s)
}
