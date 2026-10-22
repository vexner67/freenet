package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// PrimeRepository keeps Freenet Prime: the subscriptions of pages and what
// they cost.
type PrimeRepository interface {
	// Prices are user.DefaultPrimePrices until the administrators set
	// others.
	Prices(ctx context.Context) (user.PrimePrices, error)
	SetPrices(ctx context.Context, prices user.PrimePrices) error
	// Extend adds period to the subscription of userID at now, as
	// user.Prime.Extended does, and returns its end; user.ErrNotFound.
	Extend(ctx context.Context, userID uuid.UUID, period user.PrimePeriod, now time.Time) (time.Time, error)
	// SetInvisible: user.ErrNotFound.
	SetInvisible(ctx context.Context, userID uuid.UUID, invisible bool) error
}

// PrimeNotifier records what the notification service has to hear about
// Prime given as a present, in the transaction of the purchase, as
// Notifier does.
type PrimeNotifier interface {
	PrimeGiven(ctx context.Context, purchaseID, buyerID, recipientID uuid.UUID) error
}

// PrimePurchase is what a purchase of Prime did: until when the recipient
// has it and the votes the buyer has left.
type PrimePurchase struct {
	Until   time.Time
	Balance int64
}

// PrimeService sells Freenet Prime, the subscription of the site, for
// votes into the treasury: for oneself or as a present.
type PrimeService struct {
	userRepo    UserRepository
	primes      PrimeRepository
	wallets     WalletRepository
	permissions *PermissionService
	tx          TxManager
	notifier    PrimeNotifier
	admins      user.Admins
	now         func() time.Time
}

// PrimeDeps are what PrimeService works with.
type PrimeDeps struct {
	Users       UserRepository
	Primes      PrimeRepository
	Wallets     WalletRepository
	Permissions *PermissionService
	Tx          TxManager
	Notifier    PrimeNotifier
	// Admins are who price Prime.
	Admins user.Admins
}

func NewPrimeService(d PrimeDeps) *PrimeService {
	return &PrimeService{
		userRepo:    d.Users,
		primes:      d.Primes,
		wallets:     d.Wallets,
		permissions: d.Permissions,
		tx:          d.Tx,
		notifier:    d.Notifier,
		admins:      d.Admins,
		now:         time.Now,
	}
}

// Prices are what the periods of Prime cost now; anyone may ask.
func (s *PrimeService) Prices(ctx context.Context) (user.PrimePrices, error) {
	prices, err := s.primes.Prices(ctx)

	return prices, errs.Wrap("s.primes.Prices", err)
}

// SetPrices prices Prime from now on; user.ErrNotAdmin.
func (s *PrimeService) SetPrices(ctx context.Context, adminID uuid.UUID, prices user.PrimePrices) error {
	if err := checkAdmin(ctx, s.userRepo, s.admins, adminID); err != nil {
		return errs.Wrap("checkAdmin", err)
	}

	return errs.Wrap("s.primes.SetPrices", s.primes.SetPrices(ctx, prices))
}

// Buy sells buyerID period of Prime for recipientID — the buyer, or
// another as a present, who hears of it — in one transaction: the votes go
// to the treasury and the period adds to what the recipient has.
// user.ErrNotEnoughVotes; for a present what a gift would be refused with
// (user.ErrGiftRefused, user.ErrDeactivated), user.ErrNotFound.
func (s *PrimeService) Buy(ctx context.Context, buyerID, recipientID uuid.UUID, period user.PrimePeriod) (PrimePurchase, error) {
	if recipientID != buyerID {
		if err := s.checkPresent(ctx, buyerID, recipientID); err != nil {
			return PrimePurchase{}, errs.Wrap("s.checkPresent", err)
		}
	}

	prices, err := s.primes.Prices(ctx)
	if err != nil {
		return PrimePurchase{}, errs.Wrap("s.primes.Prices", err)
	}

	// The notification of a present names the purchase: one per purchase.
	purchaseID, err := uuid.NewV7()
	if err != nil {
		return PrimePurchase{}, errs.Wrap("uuid.NewV7", err)
	}

	peerID := uuid.Nil
	if recipientID != buyerID {
		peerID = recipientID
	}

	var bought PrimePurchase

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		// The votes go first: refused, they leave nothing extended even
		// where no transaction rolls back (inmem).
		if bought.Balance, err = s.wallets.Spend(ctx, buyerID, prices.Of(period),
			user.Entry(user.VotePrime, peerID, period.String())); err != nil {
			return errs.Wrap("s.wallets.Spend", err)
		}

		if bought.Until, err = s.primes.Extend(ctx, recipientID, period, s.now()); err != nil {
			return errs.Wrap("s.primes.Extend", err)
		}

		if peerID == uuid.Nil {
			return nil
		}

		return errs.Wrap("s.notifier.PrimeGiven", s.notifier.PrimeGiven(ctx, purchaseID, buyerID, recipientID))
	})
	if err != nil {
		return PrimePurchase{}, errs.Wrap("s.tx.WithinTx", err)
	}

	return bought, nil
}

// checkPresent refuses Prime as a present where a gift would be refused.
func (s *PrimeService) checkPresent(ctx context.Context, buyerID, recipientID uuid.UUID) error {
	recipient, ties, err := s.permissions.ties(ctx, buyerID, recipientID)
	if err != nil {
		return errs.Wrap("s.permissions.ties", err)
	}

	return errs.Wrap("recipient.CheckGiftFrom", recipient.CheckGiftFrom(buyerID, ties))
}

// SetInvisible hides when userID is online from others, or shows it;
// hiding takes Prime (user.ErrPrimeOnly).
func (s *PrimeService) SetInvisible(ctx context.Context, userID uuid.UUID, invisible bool) error {
	u, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return errs.Wrap("s.userRepo.GetByID", err)
	}

	if err = u.CheckInvisible(invisible, s.now()); err != nil {
		return errs.Wrap("u.CheckInvisible", err)
	}

	return errs.Wrap("s.primes.SetInvisible", s.primes.SetInvisible(ctx, userID, invisible))
}
