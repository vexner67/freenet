package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// rocketKept is how long the rounds of a player are kept: the longest
// flight, to ×10000, takes under 80 s.
const rocketKept = time.Hour

// RocketRepository keeps the rounds of "Ракетка".
type RocketRepository interface {
	Save(ctx context.Context, round *user.RocketRound) error
	// Get and Lock (FOR UPDATE, in a transaction) return
	// user.ErrRocketNotFound.
	Get(ctx context.Context, id uuid.UUID) (*user.RocketRound, error)
	Lock(ctx context.Context, id uuid.UUID) (*user.RocketRound, error)
	// SetCashed keeps the multiplier and the payout of a round cashed.
	SetCashed(ctx context.Context, round *user.RocketRound) error
	// DeleteBefore deletes the rounds of userID launched before.
	DeleteBefore(ctx context.Context, userID uuid.UUID, before time.Time) error
}

// RocketView is a round as its player sees it at a moment: the crash
// point only once the round is over.
type RocketView struct {
	Round *user.RocketRound
	State user.RocketState
	// Balance is the votes of the player after the launch or the cash.
	Balance int64
}

// Crash is the crash point of a round over, 0 while it flies.
func (v RocketView) Crash() int {
	if v.State == user.RocketFlying {
		return 0
	}

	return v.Round.Crash()
}

// RocketService is "Ракетка": a bet launches a rocket, and the player
// cashes it before it explodes.
type RocketService struct {
	userRepo UserRepository
	casinos  CasinoRepository
	rockets  RocketRepository
	wallets  WalletRepository
	tx       TxManager
	admins   user.Admins
	roll     func(n int64) (int64, error)
	now      func() time.Time
}

func NewRocketService(users UserRepository, casinos CasinoRepository, rockets RocketRepository, wallets WalletRepository, tx TxManager, admins user.Admins) *RocketService {
	return &RocketService{userRepo: users, casinos: casinos, rockets: rockets, wallets: wallets, tx: tx, admins: admins, roll: cryptoRoll, now: time.Now}
}

// WithRoll makes the crash points roll with roll: for tests.
func (s *RocketService) WithRoll(roll func(n int64) (int64, error)) *RocketService {
	s.roll = roll

	return s
}

// WithNow makes the service tell the time by now: for tests.
func (s *RocketService) WithNow(now func() time.Time) *RocketService {
	s.now = now

	return s
}

// SetRocket changes the limits, the return and the top of the rocket;
// user.ErrNotAdmin.
func (s *RocketService) SetRocket(ctx context.Context, adminID uuid.UUID, rocket user.CasinoRocket) error {
	if err := checkAdmin(ctx, s.userRepo, s.admins, adminID); err != nil {
		return errs.Wrap("checkAdmin", err)
	}

	return errs.Wrap("s.casinos.SetRocket", s.casinos.SetRocket(ctx, rocket))
}

// Launch takes amount votes from userID and launches a rocket cashing by
// itself at auto (0 — by hand), paying a round over at once, in one
// transaction; user.ErrRocketAmount, user.ErrRocketAuto,
// user.ErrNotEnoughVotes.
func (s *RocketService) Launch(ctx context.Context, userID uuid.UUID, amount int64, auto int) (RocketView, error) {
	var view RocketView

	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		rocket, err := s.casinos.Rocket(ctx)
		if err != nil {
			return errs.Wrap("s.casinos.Rocket", err)
		}

		roll, err := s.roll(user.RocketRolls)
		if err != nil {
			return errs.Wrap("s.roll", err)
		}

		id, err := uuid.NewV7()
		if err != nil {
			return errs.Wrap("uuid.NewV7", err)
		}

		now := s.now()

		round, err := rocket.Launch(id, userID, amount, auto, rocket.Crash(roll), now)
		if err != nil {
			return errs.Wrap("rocket.Launch", err)
		}

		if view.Balance, err = s.wallets.Spend(ctx, userID, amount, user.Entry(user.VoteRocket, uuid.Nil, "")); err != nil {
			return errs.Wrap("s.wallets.Spend", err)
		}

		if round.Payout() > 0 {
			if view.Balance, err = s.credit(ctx, userID, round.Payout()); err != nil {
				return errs.Wrap("s.credit", err)
			}
		}

		if err = s.rockets.DeleteBefore(ctx, userID, now.Add(-rocketKept)); err != nil {
			return errs.Wrap("s.rockets.DeleteBefore", err)
		}

		if err = s.rockets.Save(ctx, round); err != nil {
			return errs.Wrap("s.rockets.Save", err)
		}

		view.Round, view.State = round, round.StateAt(now)

		return errs.Wrap("s.casinos.CountRocket", s.casinos.CountRocket(ctx, 1, amount, round.Payout()))
	})
	if err != nil {
		return RocketView{}, errs.Wrap("s.tx.WithinTx", err)
	}

	return view, nil
}

func (s *RocketService) credit(ctx context.Context, userID uuid.UUID, votes int64) (int64, error) {
	wallet, err := s.wallets.Credit(ctx, userID, votes, 0, user.Entry(user.VoteRocket, uuid.Nil, ""))

	return wallet.Votes, errs.Wrap("s.wallets.Credit", err)
}

// Round is the round id of userID as it is now, without the balance;
// user.ErrRocketNotFound, also for a round of someone else.
func (s *RocketService) Round(ctx context.Context, userID, id uuid.UUID) (RocketView, error) {
	round, err := s.rockets.Get(ctx, id)
	if err != nil {
		return RocketView{}, errs.Wrap("s.rockets.Get", err)
	}
	if !round.Owned(userID) {
		return RocketView{}, errs.Wrap("round.Owned", user.ErrRocketNotFound)
	}

	return RocketView{Round: round, State: round.StateAt(s.now())}, nil
}

// Cash takes the bet of the round id back at the multiplier of now, if the
// rocket has not exploded, and pays it, in one transaction;
// user.ErrRocketNotFound, user.ErrRocketOver.
func (s *RocketService) Cash(ctx context.Context, userID, id uuid.UUID) (RocketView, error) {
	var view RocketView

	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		round, err := s.rockets.Lock(ctx, id)
		if err != nil {
			return errs.Wrap("s.rockets.Lock", err)
		}
		if !round.Owned(userID) {
			return errs.Wrap("round.Owned", user.ErrRocketNotFound)
		}

		now := s.now()

		if round, err = round.Cash(now); err != nil {
			return errs.Wrap("round.Cash", err)
		}

		view.Round, view.State = round, round.StateAt(now)
		if round.Payout() == 0 {
			u, err := s.userRepo.GetByID(ctx, userID)
			if err != nil {
				return errs.Wrap("s.userRepo.GetByID", err)
			}

			view.Balance = u.Wallet().Votes

			return nil
		}

		if err = s.rockets.SetCashed(ctx, round); err != nil {
			return errs.Wrap("s.rockets.SetCashed", err)
		}

		if view.Balance, err = s.credit(ctx, userID, round.Payout()); err != nil {
			return errs.Wrap("s.credit", err)
		}

		return errs.Wrap("s.casinos.CountRocket", s.casinos.CountRocket(ctx, 0, 0, round.Payout()))
	})
	if err != nil {
		return RocketView{}, errs.Wrap("s.tx.WithinTx", err)
	}

	return view, nil
}
