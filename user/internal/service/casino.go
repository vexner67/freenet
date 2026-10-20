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
	// Bets are user.DefaultCasinoBets until they are set.
	Bets(ctx context.Context) (user.CasinoBets, error)
	SetBets(ctx context.Context, bets user.CasinoBets) error
	// CountBet adds a bet of spent votes that won won.
	CountBet(ctx context.Context, spent, won int64) error
	Stats(ctx context.Context) (user.CasinoStats, error)
}

// Spin is what one spin of the casino won: the index of the prize, its
// votes and the balance after it.
type Spin struct {
	Prize   int
	Votes   int64
	Balance int64
}

// Wager is how one bet went: whether it won, the roll and the chance in
// user.BetOdds, the votes it won and the balance after it.
type Wager struct {
	Won     bool
	Roll    int
	Chance  int
	Votes   int64
	Balance int64
}

// CasinoSettings are the casino with its odds, its bets and how it does:
// for the administrators.
type CasinoSettings struct {
	Casino user.Casino
	Bets   user.CasinoBets
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

	bets, err := s.casinos.Bets(ctx)
	if err != nil {
		return CasinoSettings{}, errs.Wrap("s.casinos.Bets", err)
	}

	stats, err := s.casinos.Stats(ctx)
	if err != nil {
		return CasinoSettings{}, errs.Wrap("s.casinos.Stats", err)
	}

	return CasinoSettings{Casino: casino, Bets: bets, Stats: stats}, nil
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

// Bets are how much may be bet and how much of it comes back; anyone may
// ask.
func (s *CasinoService) Bets(ctx context.Context) (user.CasinoBets, error) {
	bets, err := s.casinos.Bets(ctx)

	return bets, errs.Wrap("s.casinos.Bets", err)
}

// SetBets changes the limits and the return of the bets; user.ErrNotAdmin.
func (s *CasinoService) SetBets(ctx context.Context, adminID uuid.UUID, bets user.CasinoBets) error {
	if err := checkAdmin(ctx, s.userRepo, s.admins, adminID); err != nil {
		return errs.Wrap("checkAdmin", err)
	}

	return errs.Wrap("s.casinos.SetBets", s.casinos.SetBets(ctx, bets))
}

// Bet takes amount votes from userID and, if the roll wins, pays them
// multiplied, in one transaction; user.ErrBetAmount and
// user.ErrBetMultiplier for a bet the casino does not take,
// user.ErrNotEnoughVotes when the bet is more than the balance.
func (s *CasinoService) Bet(ctx context.Context, userID uuid.UUID, amount int64, multiplier int) (Wager, error) {
	var wager Wager

	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		bets, err := s.casinos.Bets(ctx)
		if err != nil {
			return errs.Wrap("s.casinos.Bets", err)
		}

		bet, err := bets.Bet(amount, multiplier)
		if err != nil {
			return errs.Wrap("bets.Bet", err)
		}

		if wager.Balance, err = s.wallets.Spend(ctx, userID, bet.Amount()); err != nil {
			return errs.Wrap("s.wallets.Spend", err)
		}

		roll, err := s.roll(user.BetOdds)
		if err != nil {
			return errs.Wrap("s.roll", err)
		}

		wager.Roll, wager.Chance = int(roll), bet.Chance()
		wager.Won = bet.Wins(wager.Roll)

		if wager.Won {
			wager.Votes = bet.Prize()

			wallet, err := s.wallets.Credit(ctx, userID, wager.Votes, 0)
			if err != nil {
				return errs.Wrap("s.wallets.Credit", err)
			}

			wager.Balance = wallet.Votes
		}

		return errs.Wrap("s.casinos.CountBet", s.casinos.CountBet(ctx, bet.Amount(), wager.Votes))
	})
	if err != nil {
		return Wager{}, errs.Wrap("s.tx.WithinTx", err)
	}

	return wager, nil
}

func cryptoRoll(n int64) (int64, error) {
	roll, err := rand.Int(rand.Reader, big.NewInt(n))
	if err != nil {
		return 0, errs.Wrap("rand.Int", err)
	}

	return roll.Int64(), nil
}
