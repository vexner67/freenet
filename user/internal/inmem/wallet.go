package inmem

import (
	"bytes"
	"context"
	"slices"
	"sync"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// Spend, Move and Credit make UserRepository the wallets as well: the
// votes and the rating are kept with the users, as in the table.
func (r *UserRepository) Spend(ctx context.Context, userID uuid.UUID, votes int64) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	return r.spend(userID, votes)
}

func (r *UserRepository) Move(ctx context.Context, fromID, toID uuid.UUID, votes int64) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	to, ok := r.users[toID]
	if !ok {
		return 0, user.ErrNotFound
	}

	left, err := r.spend(fromID, votes)
	if err != nil {
		return 0, err
	}

	wallet := to.Wallet()
	wallet.Votes += votes
	r.users[toID] = *to.WithWallet(wallet)

	return left, nil
}

func (r *UserRepository) Credit(ctx context.Context, userID uuid.UUID, votes int64, rating int) (user.Wallet, error) {
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
	wallet.Votes += votes
	wallet.Rating += rating
	r.users[userID] = *u.WithWallet(wallet)

	return wallet, nil
}

// spend takes votes from userID; the caller holds the write lock.
func (r *UserRepository) spend(userID uuid.UUID, votes int64) (int64, error) {
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

	return wallet.Votes, nil
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

	return r
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
