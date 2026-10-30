package service

import (
	"context"
	"strconv"
	"time"

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
	// The change is written to the statement of the page as e.
	Spend(ctx context.Context, userID uuid.UUID, votes int64, e user.VoteEntry) (int64, error)
	// Move hands votes of fromID to toID and returns what fromID has
	// left; user.ErrNotEnoughVotes, user.ErrNotFound if either is gone.
	// It has to run in a transaction (TxManager). Each is written down
	// with the other as the peer.
	Move(ctx context.Context, fromID, toID uuid.UUID, votes int64, e user.VoteEntry) (int64, error)
	// Credit adds votes and rating to userID; user.ErrNotFound.
	// The votes come out of the treasury, as Spend puts them in; it has
	// no bottom. Move goes past it.
	Credit(ctx context.Context, userID uuid.UUID, votes int64, rating int, e user.VoteEntry) (user.Wallet, error)
	// Hold takes votes from userID and keeps them out of the treasury:
	// the bid of an auction, until Release gives them back or Deposit
	// pays them on. As Spend otherwise.
	Hold(ctx context.Context, userID uuid.UUID, votes int64, e user.VoteEntry) (int64, error)
	// Release gives held votes to userID, past the treasury;
	// user.ErrNotFound.
	Release(ctx context.Context, userID uuid.UUID, votes int64, e user.VoteEntry) (int64, error)
	// Deposit puts held votes into the treasury.
	Deposit(ctx context.Context, votes int64) error
	// Treasury is the treasury now; in a transaction it stays so until
	// the end, as every change of votes waits for it.
	Treasury(ctx context.Context) (user.Treasury, error)
	// RaiseRating adds percent to the rating of userID and returns the
	// wallet; user.ErrRatingLimit above most, user.ErrNotFound.
	RaiseRating(ctx context.Context, userID uuid.UUID, percent, most int) (user.Wallet, error)
}

// BonusRepository keeps the daily bonuses.
type BonusRepository interface {
	// TakeBonus marks the bonus of day taken by userID and says how many
	// days in a row that makes; user.ErrBonusTaken for a day taken already.
	TakeBonus(ctx context.Context, userID uuid.UUID, day time.Time) (int, error)
	// LastBonus is the day of the last bonus of userID, zero for never,
	// and how many days in a row that made.
	LastBonus(ctx context.Context, userID uuid.UUID) (time.Time, int, error)
}

// StatementRepository reads the statements of pages: every change of
// their votes.
type StatementRepository interface {
	// Operations returns up to limit operations of userID of the flow,
	// newest first, with IDs below before (uuid.Nil: from the newest).
	Operations(ctx context.Context, userID, before uuid.UUID, flow user.VoteFlow, limit int) ([]user.VoteOperation, error)
	// Totals is what came in and went out since.
	Totals(ctx context.Context, userID uuid.UUID, since time.Time) (income, expense int64, err error)
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

// EconomyRepository keeps the economy the administrators tune: the daily
// bonus and the fees.
type EconomyRepository interface {
	// Economy is user.DefaultEconomy until one is set.
	Economy(ctx context.Context) (user.Economy, error)
	SetEconomy(ctx context.Context, e user.Economy) error
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

// StatementPage is one screen of the statement of a page, newest first;
// NextCursor is uuid.Nil on the last. Income and Expense are what came in
// and went out over StatementPeriod.
type StatementPage struct {
	Operations []user.VoteOperation
	NextCursor uuid.UUID
	Income     int64
	Expense    int64
}

// Bonus is the daily bonus of a page: what it gives now, whether it is
// taken today and whether the page may take it at all. Streak is the day
// in a row the page is on, counting today's bonus.
type Bonus struct {
	Votes    int64
	Streak   int
	Taken    bool
	Eligible bool
}

// StatementPeriod is how far back the totals of a statement go.
const StatementPeriod = 30 * 24 * time.Hour

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
	statements  StatementRepository
	bonuses     BonusRepository
	vouchers    VoucherRepository
	rates       VoteRateRepository
	economy     EconomyRepository
	permissions *PermissionService
	tx          TxManager
	notifier    VotesNotifier
	admins      user.Admins
	now         func() time.Time
}

// WalletDeps are what WalletService works with.
type WalletDeps struct {
	Users       UserRepository
	Wallets     WalletRepository
	Statements  StatementRepository
	Bonuses     BonusRepository
	Vouchers    VoucherRepository
	Rates       VoteRateRepository
	Economy     EconomyRepository
	Permissions *PermissionService
	Tx          TxManager
	Notifier    VotesNotifier
	Admins      user.Admins
}

func NewWalletService(d WalletDeps) *WalletService {
	return &WalletService{
		userRepo:    d.Users,
		wallets:     d.Wallets,
		statements:  d.Statements,
		bonuses:     d.Bonuses,
		vouchers:    d.Vouchers,
		rates:       d.Rates,
		economy:     d.Economy,
		permissions: d.Permissions,
		tx:          d.Tx,
		notifier:    d.Notifier,
		admins:      d.Admins,
		now:         time.Now,
	}
}

// IsAdmin tells whether u makes the vouchers.
func (s *WalletService) IsAdmin(u *user.User) bool {
	return s.admins.Has(u)
}

// Transfer hands votes of senderID to recipientID, who hears of it, and
// returns what the sender has left and the fee: the sender pays
// User.TransferFeeAt on top, into the treasury (nothing with Prime).
// user.ErrNotFound for an unknown recipient,
// user.ErrTransferSelf, user.ErrDeactivated, user.ErrTransferRefused and
// user.ErrNotEnoughVotes for votes that do not cover the fee as well.
// The note goes with it to the statements of both.
func (s *WalletService) Transfer(
	ctx context.Context, senderID, recipientID uuid.UUID, votes user.Votes, note user.TransferNote,
) (left, fee int64, err error) {
	recipient, ties, err := s.permissions.ties(ctx, senderID, recipientID)
	if err != nil {
		return 0, 0, errs.Wrap("s.permissions.ties", err)
	}

	if err = recipient.CheckTransferFrom(senderID, ties); err != nil {
		return 0, 0, errs.Wrap("recipient.CheckTransferFrom", err)
	}

	sender, err := s.userRepo.GetByID(ctx, senderID)
	if err != nil {
		return 0, 0, errs.Wrap("s.userRepo.GetByID", err)
	}

	economy, err := s.economy.Economy(ctx)
	if err != nil {
		return 0, 0, errs.Wrap("s.economy.Economy", err)
	}

	fee = sender.TransferFeeAt(economy, votes.Int64(), s.now())

	// The notification names the transfer: one per transfer.
	transferID, err := uuid.NewV7()
	if err != nil {
		return 0, 0, errs.Wrap("uuid.NewV7", err)
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if fee > 0 {
			if _, err = s.wallets.Spend(ctx, senderID, fee, user.Entry(user.VoteFee, recipientID, "")); err != nil {
				return errs.Wrap("s.wallets.Spend", err)
			}
		}

		if left, err = s.wallets.Move(ctx, senderID, recipientID, votes.Int64(),
			user.Entry(user.VoteTransfer, uuid.Nil, note.String())); err != nil {
			return errs.Wrap("s.wallets.Move", err)
		}

		return errs.Wrap("s.notifier.VotesSent", s.notifier.VotesSent(ctx, transferID, senderID, recipientID))
	})
	if err != nil {
		return 0, 0, errs.Wrap("s.tx.WithinTx", err)
	}

	return left, fee, nil
}

// Statement is one screen of the statement of userID: the operations of
// the flow, newest first, and the totals of StatementPeriod.
func (s *WalletService) Statement(ctx context.Context, userID, cursor uuid.UUID, flow user.VoteFlow, size int, now time.Time) (StatementPage, error) {
	if size <= 0 || size > MaxPageSize {
		size = DefaultPageSize
	}

	ops, err := s.statements.Operations(ctx, userID, cursor, flow, size+1)
	if err != nil {
		return StatementPage{}, errs.Wrap("s.statements.Operations", err)
	}

	page := StatementPage{Operations: ops}
	if len(ops) > size {
		page.Operations = ops[:size]
		page.NextCursor = ops[size-1].ID
	}

	if page.Income, page.Expense, err = s.statements.Totals(ctx, userID, now.Add(-StatementPeriod)); err != nil {
		return StatementPage{}, errs.Wrap("s.statements.Totals", err)
	}

	return page, nil
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

		wallet, err := s.wallets.Credit(ctx, userID, voucher.Votes(), voucher.Rating(),
			user.Entry(user.VoteVoucher, uuid.Nil, ""))
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

	if err := s.vouchers.Save(ctx, voucher); err != nil {
		return nil, errs.Wrap("s.vouchers.Save", err)
	}

	return voucher, nil
}

// Treasury is the treasury of the site now; anyone may ask.
func (s *WalletService) Treasury(ctx context.Context) (user.Treasury, error) {
	treasury, err := s.wallets.Treasury(ctx)

	return treasury, errs.Wrap("s.wallets.Treasury", err)
}

// Bonus is the daily bonus of userID at now.
func (s *WalletService) Bonus(ctx context.Context, userID uuid.UUID, now time.Time) (Bonus, error) {
	u, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return Bonus{}, errs.Wrap("s.userRepo.GetByID", err)
	}

	last, streak, err := s.bonuses.LastBonus(ctx, userID)
	if err != nil {
		return Bonus{}, errs.Wrap("s.bonuses.LastBonus", err)
	}

	economy, err := s.economy.Economy(ctx)
	if err != nil {
		return Bonus{}, errs.Wrap("s.economy.Economy", err)
	}

	today := user.BonusDay(now)
	taken := !last.Before(today)

	switch {
	case taken:
	case last.Equal(today.AddDate(0, 0, -1)):
		streak++
	default:
		streak = 1
	}

	return Bonus{
		Votes:    economy.BonusOf(streak),
		Streak:   streak,
		Taken:    taken,
		Eligible: u.CheckBonusAt(now) == nil,
	}, nil
}

// ClaimBonus gives userID the bonus of the day of now out of the treasury,
// in one transaction; user.ErrBonusNotYet, user.ErrBonusTaken.
func (s *WalletService) ClaimBonus(ctx context.Context, userID uuid.UUID, now time.Time) (Redeemed, error) {
	u, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return Redeemed{}, errs.Wrap("s.userRepo.GetByID", err)
	}

	if err = u.CheckBonusAt(now); err != nil {
		return Redeemed{}, errs.Wrap("u.CheckBonusAt", err)
	}

	economy, err := s.economy.Economy(ctx)
	if err != nil {
		return Redeemed{}, errs.Wrap("s.economy.Economy", err)
	}

	var claimed Redeemed

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		streak, err := s.bonuses.TakeBonus(ctx, userID, user.BonusDay(now))
		if err != nil {
			return errs.Wrap("s.bonuses.TakeBonus", err)
		}

		claimed.Votes = economy.BonusOf(streak)

		claimed.Wallet, err = s.wallets.Credit(ctx, userID, claimed.Votes, 0, user.Entry(user.VoteBonus, uuid.Nil, ""))

		return errs.Wrap("s.wallets.Credit", err)
	})
	if err != nil {
		return Redeemed{}, errs.Wrap("s.tx.WithinTx", err)
	}

	return claimed, nil
}

// RaiseRating sells userID the percent of rating ("Повысить рейтинг" of
// openvk) for user.RatingPrice a percent, into the treasury, and returns
// the wallet after it; user.ErrNotEnoughVotes, user.ErrRatingLimit.
func (s *WalletService) RaiseRating(ctx context.Context, userID uuid.UUID, raise user.RatingRaise) (user.Wallet, error) {
	u, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return user.Wallet{}, errs.Wrap("s.userRepo.GetByID", err)
	}

	// Refused before the votes go; the statement that raises the rating
	// checks it again.
	most := u.RatingMost()
	if u.Wallet().Rating+raise.Int() > most {
		return user.Wallet{}, user.ErrRatingLimit
	}

	var wallet user.Wallet

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		// The rating goes after the votes: refused, it leaves nothing
		// raised even where no transaction rolls back (inmem).
		if _, err := s.wallets.Spend(ctx, userID, raise.Cost(),
			user.Entry(user.VoteRating, uuid.Nil, strconv.Itoa(raise.Int()))); err != nil {
			return errs.Wrap("s.wallets.Spend", err)
		}

		wallet, err = s.wallets.RaiseRating(ctx, userID, raise.Int(), most)

		return errs.Wrap("s.wallets.RaiseRating", err)
	})
	if err != nil {
		return user.Wallet{}, errs.Wrap("s.tx.WithinTx", err)
	}

	return wallet, nil
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

// Economy is the bonus and the fees of the site now; anyone may ask.
func (s *WalletService) Economy(ctx context.Context) (user.Economy, error) {
	e, err := s.economy.Economy(ctx)

	return e, errs.Wrap("s.economy.Economy", err)
}

// SetEconomy sets the bonus and the fees from now on; user.ErrNotAdmin.
func (s *WalletService) SetEconomy(ctx context.Context, adminID uuid.UUID, e user.Economy) error {
	if err := checkAdmin(ctx, s.userRepo, s.admins, adminID); err != nil {
		return errs.Wrap("checkAdmin", err)
	}

	return errs.Wrap("s.economy.SetEconomy", s.economy.SetEconomy(ctx, e))
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
