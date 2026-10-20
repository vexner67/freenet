package service

import (
	"context"
	"crypto/rand"
	"math/big"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// CasinoRepository keeps the casino of the site and how it does.
type CasinoRepository interface {
	// Casino is user.DefaultCasino until one is set.
	Casino(ctx context.Context) (user.Casino, error)
	SetCasino(ctx context.Context, casino user.Casino) error
	// CountSpin adds a spin that cost spent votes and won won.
	CountSpin(ctx context.Context, spent, won int64) error
	Stats(ctx context.Context) (user.CasinoStats, error)
}

// Spin is what one spin of the casino won: the index of the prize, its
// votes and the balance after it.
type Spin struct {
	Prize   int
	Votes   int64
	Balance int64
}

// CasinoSettings are the casino with its odds and how it does: for the
// administrators.
type CasinoSettings struct {
	Casino user.Casino
	Stats  user.CasinoStats
}

// CasinoService is the casino: people spend votes on a spin and win votes
// back, as likely as the administrators set.
type CasinoService struct {
	userRepo UserRepository
	casinos  CasinoRepository
	wallets  WalletRepository
	tx       TxManager
	admins   user.Admins
	// roll is a random number in [0, n).
	roll func(n int64) (int64, error)
}

func NewCasinoService(users UserRepository, casinos CasinoRepository, wallets WalletRepository, tx TxManager, admins user.Admins) *CasinoService {
	return &CasinoService{userRepo: users, casinos: casinos, wallets: wallets, tx: tx, admins: admins, roll: cryptoRoll}
}

// WithRoll makes the spins roll with roll: for tests.
func (s *CasinoService) WithRoll(roll func(n int64) (int64, error)) *CasinoService {
	s.roll = roll

	return s
}

// Casino is what a spin costs and may win; anyone may ask.
func (s *CasinoService) Casino(ctx context.Context) (user.Casino, error) {
	casino, err := s.casinos.Casino(ctx)

	return casino, errs.Wrap("s.casinos.Casino", err)
}

// Settings are the casino with the odds and its stats; user.ErrNotAdmin.
func (s *CasinoService) Settings(ctx context.Context, adminID uuid.UUID) (CasinoSettings, error) {
	if err := checkAdmin(ctx, s.userRepo, s.admins, adminID); err != nil {
		return CasinoSettings{}, errs.Wrap("checkAdmin", err)
	}

	casino, err := s.casinos.Casino(ctx)
	if err != nil {
		return CasinoSettings{}, errs.Wrap("s.casinos.Casino", err)
	}

	stats, err := s.casinos.Stats(ctx)
	if err != nil {
		return CasinoSettings{}, errs.Wrap("s.casinos.Stats", err)
	}

	return CasinoSettings{Casino: casino, Stats: stats}, nil
}

// SetCasino changes the price, the prizes and the odds; user.ErrNotAdmin.
func (s *CasinoService) SetCasino(ctx context.Context, adminID uuid.UUID, casino user.Casino) error {
	if err := checkAdmin(ctx, s.userRepo, s.admins, adminID); err != nil {
		return errs.Wrap("checkAdmin", err)
	}

	return errs.Wrap("s.casinos.SetCasino", s.casinos.SetCasino(ctx, casino))
}

// Spin takes the price from userID and pays the prize, in one
// transaction; user.ErrNotEnoughVotes when the price is more than the
// balance.
func (s *CasinoService) Spin(ctx context.Context, userID uuid.UUID) (Spin, error) {
	var spin Spin

	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		casino, err := s.casinos.Casino(ctx)
		if err != nil {
			return errs.Wrap("s.casinos.Casino", err)
		}

		if spin.Balance, err = s.wallets.Spend(ctx, userID, casino.Price()); err != nil {
			return errs.Wrap("s.wallets.Spend", err)
		}

		roll, err := s.roll(casino.TotalWeight())
		if err != nil {
			return errs.Wrap("s.roll", err)
		}

		if spin.Prize, err = casino.Pick(roll); err != nil {
			return errs.Wrap("casino.Pick", err)
		}

		spin.Votes = casino.Prizes()[spin.Prize].Votes

		if spin.Votes > 0 {
			wallet, err := s.wallets.Credit(ctx, userID, spin.Votes, 0)
			if err != nil {
				return errs.Wrap("s.wallets.Credit", err)
			}

			spin.Balance = wallet.Votes
		}

		return errs.Wrap("s.casinos.CountSpin", s.casinos.CountSpin(ctx, casino.Price(), spin.Votes))
	})
	if err != nil {
		return Spin{}, errs.Wrap("s.tx.WithinTx", err)
	}

	return spin, nil
}

func cryptoRoll(n int64) (int64, error) {
	roll, err := rand.Int(rand.Reader, big.NewInt(n))
	if err != nil {
		return 0, errs.Wrap("rand.Int", err)
	}

	return roll.Int64(), nil
}
