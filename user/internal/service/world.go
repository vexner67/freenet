package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// WorldVoteRepository remembers the operations of votes the world service
// asked for.
type WorldVoteRepository interface {
	// AddWorldVote writes opID down: amount of userID, below zero for
	// votes taken; false when opID was written down already.
	AddWorldVote(ctx context.Context, opID, userID uuid.UUID, amount int64) (bool, error)
}

var ErrWorldVotes = errors.New("invalid votes of the world")

// WorldVotes is an operation of votes of the world service. OpID makes it
// happen once, however many times the world asks.
type WorldVotes struct {
	OpID   uuid.UUID
	UserID uuid.UUID
	Votes  int64
	Note   string
	// Settled asks that the page may run enterprises (User.CheckFirmsAt):
	// the founder of a state.
	Settled bool
}

// WorldService takes votes for the states of «Мир» and pays them out of
// their treasuries. The votes taken go to the treasury of the site, as the
// price of an enterprise; the votes paid come out of it.
type WorldService struct {
	users   UserRepository
	wallets WalletRepository
	ops     WorldVoteRepository
	tx      TxManager
	now     func() time.Time
}

func NewWorldService(users UserRepository, wallets WalletRepository, ops WorldVoteRepository, tx TxManager) *WorldService {
	return &WorldService{users: users, wallets: wallets, ops: ops, tx: tx, now: time.Now}
}

// ChargeVotes takes v.Votes of v.UserID and returns the votes left; done
// before, it takes nothing. user.ErrNotEnoughVotes, user.ErrDeactivated
// for a page deleted or blocked, user.ErrFirmNotYet for a page not
// settled when v.Settled, ErrWorldVotes.
func (s *WorldService) ChargeVotes(ctx context.Context, v WorldVotes) (int64, error) {
	if v.Votes <= 0 || v.OpID == uuid.Nil {
		return 0, ErrWorldVotes
	}

	u, err := s.users.GetByID(ctx, v.UserID)
	if err != nil {
		return 0, errs.Wrap("s.users.GetByID", err)
	}

	now := s.now()
	if u.IsDeactivated() || u.IsBannedAt(now) {
		return 0, user.ErrDeactivated
	}

	if v.Settled {
		if err = u.CheckFirmsAt(now); err != nil {
			return 0, errs.Wrap("u.CheckFirmsAt", err)
		}
	}

	left := u.Wallet().Votes
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		added, err := s.ops.AddWorldVote(ctx, v.OpID, v.UserID, -v.Votes)
		if err != nil || !added {
			return errs.Wrap("s.ops.AddWorldVote", err)
		}

		left, err = s.wallets.Spend(ctx, v.UserID, v.Votes, user.Entry(user.VoteWorld, uuid.Nil, v.Note))

		return errs.Wrap("s.wallets.Spend", err)
	})

	return left, errs.Wrap("s.tx.WithinTx", err)
}

// PayVotes pays v.Votes to v.UserID and returns their votes; done before,
// it pays nothing. user.ErrNotFound, ErrWorldVotes.
func (s *WorldService) PayVotes(ctx context.Context, v WorldVotes) (int64, error) {
	if v.Votes <= 0 || v.OpID == uuid.Nil {
		return 0, ErrWorldVotes
	}

	u, err := s.users.GetByID(ctx, v.UserID)
	if err != nil {
		return 0, errs.Wrap("s.users.GetByID", err)
	}

	votes := u.Wallet().Votes
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		added, err := s.ops.AddWorldVote(ctx, v.OpID, v.UserID, v.Votes)
		if err != nil || !added {
			return errs.Wrap("s.ops.AddWorldVote", err)
		}

		wallet, err := s.wallets.Credit(ctx, v.UserID, v.Votes, 0, user.Entry(user.VoteWorld, uuid.Nil, v.Note))
		votes = wallet.Votes

		return errs.Wrap("s.wallets.Credit", err)
	})

	return votes, errs.Wrap("s.tx.WithinTx", err)
}
