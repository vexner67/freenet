package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// WalletRepository changes the votes and the rating of pages. Every change
// is one that cannot leave a balance below zero, whatever else runs at the
// same time.
type WalletRepository interface {
	// Spend takes votes from userID and returns what is left;
	// user.ErrNotEnoughVotes when there are fewer, user.ErrNotFound.
	Spend(ctx context.Context, userID uuid.UUID, votes int64) (int64, error)
	// Move hands votes of fromID to toID and returns what fromID has
	// left; user.ErrNotEnoughVotes, user.ErrNotFound if either is gone.
	// It has to run in a transaction (TxManager).
	Move(ctx context.Context, fromID, toID uuid.UUID, votes int64) (int64, error)
	// Credit adds votes and rating to userID; user.ErrNotFound.
	Credit(ctx context.Context, userID uuid.UUID, votes int64, rating int) (user.Wallet, error)
}

// VoucherRepository keeps the vouchers and who has used them.
type VoucherRepository interface {
	// Save returns user.ErrVoucherExists for a serial number in use.
	Save(ctx context.Context, v *user.Voucher) error
	// Redeem marks the voucher used by userID and returns it;
	// user.ErrVoucherInvalid if there is none, it has run out or userID
	// has used it. Two accounts cannot take the last usage both.
	Redeem(ctx context.Context, token user.VoucherToken, userID uuid.UUID) (*user.Voucher, error)
	// List returns up to limit vouchers, newest first, with IDs below
	// before (uuid.Nil: from the newest).
	List(ctx context.Context, before uuid.UUID, limit int) ([]*user.Voucher, error)
	// Delete is idempotent.
	Delete(ctx context.Context, id uuid.UUID) error
}

// VoteRateRepository keeps what a vote is worth in rubles.
type VoteRateRepository interface {
	// Rate is user.DefaultVoteRate until one is set.
	Rate(ctx context.Context) (user.VoteRate, error)
	SetRate(ctx context.Context, rate user.VoteRate) error
}

// VotesNotifier records what the notification service has to hear about
// transfers, in the transaction of the transfer, as Notifier does.
type VotesNotifier interface {
	VotesSent(ctx context.Context, transferID, senderID, recipientID uuid.UUID) error
}

// VoucherDraft is a voucher to make. An empty Token asks for a generated
// one.
type VoucherDraft struct {
	Token  user.VoucherToken
	Votes  int64
	Rating int
	Usages int
}

// Redeemed is what a voucher gave and the wallet after it.
type Redeemed struct {
	Votes  int64
	Rating int
	Wallet user.Wallet
}

// VoucherPage is one screen of the vouchers, newest first; NextCursor is
// uuid.Nil on the last.
type VoucherPage struct {
	Vouchers   []*user.Voucher
	NextCursor uuid.UUID
}

// WalletService is openvk's votes ("голоса"): vouchers give them, gifts
// cost them and people hand them to each other.
type WalletService struct {
	userRepo    UserRepository
	wallets     WalletRepository
	vouchers    VoucherRepository
	rates       VoteRateRepository
	permissions *PermissionService
	tx          TxManager
	notifier    VotesNotifier
	admins      user.Admins
}

// WalletDeps are what WalletService works with.
type WalletDeps struct {
	Users       UserRepository
	Wallets     WalletRepository
	Vouchers    VoucherRepository
	Rates       VoteRateRepository
	Permissions *PermissionService
	Tx          TxManager
	Notifier    VotesNotifier
	Admins      user.Admins
}

func NewWalletService(d WalletDeps) *WalletService {
	return &WalletService{
		userRepo:    d.Users,
		wallets:     d.Wallets,
		vouchers:    d.Vouchers,
		rates:       d.Rates,
		permissions: d.Permissions,
		tx:          d.Tx,
		notifier:    d.Notifier,
		admins:      d.Admins,
	}
}

// IsAdmin tells whether u makes the vouchers.
func (s *WalletService) IsAdmin(u *user.User) bool {
	return s.admins.Has(u)
}

// Transfer hands votes of senderID to recipientID, who hears of it, and
// returns what the sender has left. user.ErrNotFound for an unknown
// recipient, user.ErrTransferSelf, user.ErrDeactivated,
// user.ErrTransferRefused and user.ErrNotEnoughVotes.
func (s *WalletService) Transfer(ctx context.Context, senderID, recipientID uuid.UUID, votes user.Votes) (int64, error) {
	recipient, ties, err := s.permissions.ties(ctx, senderID, recipientID)
	if err != nil {
		return 0, errs.Wrap("s.permissions.ties", err)
	}

	if err = recipient.CheckTransferFrom(senderID, ties); err != nil {
		return 0, errs.Wrap("recipient.CheckTransferFrom", err)
	}

	// The notification names the transfer: one per transfer.
	transferID, err := uuid.NewV7()
	if err != nil {
		return 0, errs.Wrap("uuid.NewV7", err)
	}

	var left int64

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if left, err = s.wallets.Move(ctx, senderID, recipientID, votes.Int64()); err != nil {
			return errs.Wrap("s.wallets.Move", err)
		}

		return errs.Wrap("s.notifier.VotesSent", s.notifier.VotesSent(ctx, transferID, senderID, recipientID))
	})
	if err != nil {
		return 0, errs.Wrap("s.tx.WithinTx", err)
	}

	return left, nil
}

// Redeem gives userID what the voucher carries, once.
// user.ErrVoucherInvalid whatever is wrong with it.
func (s *WalletService) Redeem(ctx context.Context, userID uuid.UUID, token user.VoucherToken) (Redeemed, error) {
	var redeemed Redeemed

	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		voucher, err := s.vouchers.Redeem(ctx, token, userID)
		if err != nil {
			return errs.Wrap("s.vouchers.Redeem", err)
		}

		wallet, err := s.wallets.Credit(ctx, userID, voucher.Votes(), voucher.Rating())
		if err != nil {
			return errs.Wrap("s.wallets.Credit", err)
		}

		redeemed = Redeemed{Votes: voucher.Votes(), Rating: voucher.Rating(), Wallet: wallet}

		return nil
	})
	if err != nil {
		return Redeemed{}, errs.Wrap("s.tx.WithinTx", err)
	}

	return redeemed, nil
}

// CreateVoucher makes a voucher; user.ErrNotAdmin unless adminID runs the
// site, user.ErrVoucherExists and what user.NewVoucher refuses.
func (s *WalletService) CreateVoucher(ctx context.Context, adminID uuid.UUID, d VoucherDraft) (*user.Voucher, error) {
	if err := checkAdmin(ctx, s.userRepo, s.admins, adminID); err != nil {
		return nil, errs.Wrap("checkAdmin", err)
	}

	if d.Token == "" {
		token, err := user.GenerateVoucherToken()
		if err != nil {
			return nil, errs.Wrap("user.GenerateVoucherToken", err)
		}

		d.Token = token
	}

	voucher, err := user.NewVoucher(d.Token, d.Votes, d.Rating, d.Usages)
	if err != nil {
		return nil, errs.Wrap("user.NewVoucher", err)
	}

	if err = s.vouchers.Save(ctx, voucher); err != nil {
		return nil, errs.Wrap("s.vouchers.Save", err)
	}

	return voucher, nil
}

// ListVouchers pages every voucher, newest first; user.ErrNotAdmin.
func (s *WalletService) ListVouchers(ctx context.Context, adminID, cursor uuid.UUID, size int) (VoucherPage, error) {
	if err := checkAdmin(ctx, s.userRepo, s.admins, adminID); err != nil {
		return VoucherPage{}, errs.Wrap("checkAdmin", err)
	}

	if size <= 0 || size > MaxPageSize {
		size = DefaultPageSize
	}

	// One extra row tells whether there is a next page.
	found, err := s.vouchers.List(ctx, cursor, size+1)
	if err != nil {
		return VoucherPage{}, errs.Wrap("s.vouchers.List", err)
	}

	page := VoucherPage{Vouchers: found}

	if len(found) > size {
		page.Vouchers = found[:size]
		page.NextCursor = found[size-1].ID()
	}

	return page, nil
}

// DeleteVoucher removes a voucher: nobody else can use it, what it gave
// stays given. user.ErrNotAdmin.
func (s *WalletService) DeleteVoucher(ctx context.Context, adminID, voucherID uuid.UUID) error {
	if err := checkAdmin(ctx, s.userRepo, s.admins, adminID); err != nil {
		return errs.Wrap("checkAdmin", err)
	}

	return errs.Wrap("s.vouchers.Delete", s.vouchers.Delete(ctx, voucherID))
}

// Rate is what a vote is worth in rubles; anyone may ask.
func (s *WalletService) Rate(ctx context.Context) (user.VoteRate, error) {
	rate, err := s.rates.Rate(ctx)
	if err != nil {
		return 0, errs.Wrap("s.rates.Rate", err)
	}

	return rate, nil
}

// SetRate says what a vote is worth from now on; user.ErrNotAdmin.
func (s *WalletService) SetRate(ctx context.Context, adminID uuid.UUID, rate user.VoteRate) error {
	if err := checkAdmin(ctx, s.userRepo, s.admins, adminID); err != nil {
		return errs.Wrap("checkAdmin", err)
	}

	return errs.Wrap("s.rates.SetRate", s.rates.SetRate(ctx, rate))
}

// checkAdmin is user.ErrNotAdmin unless userID runs the site.
func checkAdmin(ctx context.Context, users UserRepository, admins user.Admins, userID uuid.UUID) error {
	u, err := users.GetByID(ctx, userID)
	if err != nil {
		return errs.Wrap("users.GetByID", err)
	}

	if !admins.Has(u) {
		return user.ErrNotAdmin
	}

	return nil
}
