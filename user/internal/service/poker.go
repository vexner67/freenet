package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// pokerKept is how long the hands of a player are kept once over.
const pokerKept = 24 * time.Hour

// PokerRepository keeps the table of "Покер" and its hands.
type PokerRepository interface {
	// Poker is user.DefaultPoker until one is set.
	Poker(ctx context.Context) (user.Poker, error)
	SetPoker(ctx context.Context, p user.Poker) error
	Stats(ctx context.Context) (user.PokerStats, error)
	// Count adds hands dealt, votes bet and votes paid back.
	Count(ctx context.Context, hands, spent, won int64) error
	// AddHand keeps a hand dealt; user.ErrPokerOpen when its player has
	// one dealt already.
	AddHand(ctx context.Context, h *user.PokerHand) error
	// SetOver keeps the state and the payout of a hand over.
	SetOver(ctx context.Context, h *user.PokerHand) error
	// Hand and LockHand (FOR UPDATE, in a transaction) return
	// user.ErrPokerNotFound.
	Hand(ctx context.Context, id uuid.UUID) (*user.PokerHand, error)
	LockHand(ctx context.Context, id uuid.UUID) (*user.PokerHand, error)
	// Dealt is the hand of userID waiting for them; user.ErrPokerNotFound.
	Dealt(ctx context.Context, userID uuid.UUID) (*user.PokerHand, error)
	// DeleteOver deletes the hands of userID over and dealt before.
	DeleteOver(ctx context.Context, userID uuid.UUID, before time.Time) error
}

// PokerTable is the table as a player sees it: its limits and pays, and
// the hand dealt to them waiting, nil for none.
type PokerTable struct {
	Poker user.Poker
	Dealt *user.PokerHand
}

// PokerView is a hand after a deal, a fold or a call, with the balance of
// its player then.
type PokerView struct {
	Hand    *user.PokerHand
	Balance int64
}

// PokerSettings are the table and how it does: for the administrators.
type PokerSettings struct {
	Poker user.Poker
	Stats user.PokerStats
}

// PokerService is "Покер": Casino Hold'em against the dealer of the site.
type PokerService struct {
	userRepo UserRepository
	pokers   PokerRepository
	wallets  WalletRepository
	tx       TxManager
	admins   user.Admins
	roll     func(n int64) (int64, error)
	now      func() time.Time
}

func NewPokerService(users UserRepository, pokers PokerRepository, wallets WalletRepository, tx TxManager, admins user.Admins) *PokerService {
	return &PokerService{userRepo: users, pokers: pokers, wallets: wallets, tx: tx, admins: admins, roll: cryptoRoll, now: time.Now}
}

// WithRoll makes the deck shuffle with roll: for tests.
func (s *PokerService) WithRoll(roll func(n int64) (int64, error)) *PokerService {
	s.roll = roll

	return s
}

// Table is the table and, for userID (uuid.Nil — a guest), the hand dealt
// to them waiting.
func (s *PokerService) Table(ctx context.Context, userID uuid.UUID) (PokerTable, error) {
	p, err := s.pokers.Poker(ctx)
	if err != nil {
		return PokerTable{}, errs.Wrap("s.pokers.Poker", err)
	}

	table := PokerTable{Poker: p}
	if userID == uuid.Nil {
		return table, nil
	}

	table.Dealt, err = s.pokers.Dealt(ctx, userID)
	if errors.Is(err, user.ErrPokerNotFound) {
		return table, nil
	}

	return table, errs.Wrap("s.pokers.Dealt", err)
}

// shuffle draws the nine cards of a hand out of a full deck.
func (s *PokerService) shuffle() ([]user.Card, error) {
	deck := make([]user.Card, user.DeckSize)
	for i := range deck {
		deck[i] = user.Card(i)
	}

	for i := range 9 {
		j, err := s.roll(int64(user.DeckSize - i))
		if err != nil {
			return nil, errs.Wrap("s.roll", err)
		}
		deck[i], deck[i+int(j)] = deck[i+int(j)], deck[i]
	}

	return deck, nil
}

// Deal takes the ante from userID and deals them a hand, in one
// transaction; user.ErrPokerAnte, user.ErrPokerOpen, user.ErrNotEnoughVotes.
func (s *PokerService) Deal(ctx context.Context, userID uuid.UUID, ante int64) (PokerView, error) {
	var view PokerView

	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		p, err := s.pokers.Poker(ctx)
		if err != nil {
			return errs.Wrap("s.pokers.Poker", err)
		}

		deck, err := s.shuffle()
		if err != nil {
			return errs.Wrap("s.shuffle", err)
		}

		id, err := uuid.NewV7()
		if err != nil {
			return errs.Wrap("uuid.NewV7", err)
		}

		now := s.now()

		h, err := p.Deal(id, userID, ante, deck, now)
		if err != nil {
			return errs.Wrap("p.Deal", err)
		}

		// A hand waiting is played first; the unique index catches one
		// dealt meanwhile, this check spares the votes in memory too.
		if _, err = s.pokers.Dealt(ctx, userID); !errors.Is(err, user.ErrPokerNotFound) {
			if err == nil {
				err = user.ErrPokerOpen
			}

			return errs.Wrap("s.pokers.Dealt", err)
		}

		if view.Balance, err = s.wallets.Spend(ctx, userID, ante, user.Entry(user.VotePoker, uuid.Nil, "")); err != nil {
			return errs.Wrap("s.wallets.Spend", err)
		}

		if err = s.pokers.DeleteOver(ctx, userID, now.Add(-pokerKept)); err != nil {
			return errs.Wrap("s.pokers.DeleteOver", err)
		}

		if err = s.pokers.AddHand(ctx, h); err != nil {
			return errs.Wrap("s.pokers.AddHand", err)
		}

		view.Hand = h

		return errs.Wrap("s.pokers.Count", s.pokers.Count(ctx, 1, ante, 0))
	})
	if err != nil {
		return PokerView{}, errs.Wrap("s.tx.WithinTx", err)
	}

	return view, nil
}

// Fold gives up the hand id of userID, the ante lost; user.ErrPokerNotFound,
// user.ErrPokerOver.
func (s *PokerService) Fold(ctx context.Context, userID, id uuid.UUID) (PokerView, error) {
	return s.finish(ctx, userID, id, func(_ user.Poker, h *user.PokerHand) (*user.PokerHand, error) {
		return h.Fold()
	})
}

// Call takes twice the ante from userID, plays the hand id to the end and
// pays what it gives back; user.ErrPokerNotFound, user.ErrPokerOver,
// user.ErrNotEnoughVotes.
func (s *PokerService) Call(ctx context.Context, userID, id uuid.UUID) (PokerView, error) {
	return s.finish(ctx, userID, id, func(p user.Poker, h *user.PokerHand) (*user.PokerHand, error) {
		return h.Call(p)
	})
}

func (s *PokerService) finish(ctx context.Context, userID, id uuid.UUID, play func(p user.Poker, h *user.PokerHand) (*user.PokerHand, error)) (PokerView, error) {
	var view PokerView

	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		h, err := s.pokers.LockHand(ctx, id)
		if err != nil {
			return errs.Wrap("s.pokers.LockHand", err)
		}
		if !h.Owned(userID) {
			return errs.Wrap("h.Owned", user.ErrPokerNotFound)
		}

		p, err := s.pokers.Poker(ctx)
		if err != nil {
			return errs.Wrap("s.pokers.Poker", err)
		}

		if h, err = play(p, h); err != nil {
			return errs.Wrap("play", err)
		}

		if err = s.pokers.SetOver(ctx, h); err != nil {
			return errs.Wrap("s.pokers.SetOver", err)
		}

		view.Hand = h
		if view.Balance, err = s.settle(ctx, userID, h); err != nil {
			return errs.Wrap("s.settle", err)
		}

		if !h.Called() {
			return nil
		}

		return errs.Wrap("s.pokers.Count", s.pokers.Count(ctx, 0, 2*h.Ante(), h.Payout()))
	})
	if err != nil {
		return PokerView{}, errs.Wrap("s.tx.WithinTx", err)
	}

	return view, nil
}

// settle takes the call of a hand called and pays it, and is the balance
// of userID after it.
func (s *PokerService) settle(ctx context.Context, userID uuid.UUID, h *user.PokerHand) (int64, error) {
	if !h.Called() {
		u, err := s.userRepo.GetByID(ctx, userID)
		if err != nil {
			return 0, errs.Wrap("s.userRepo.GetByID", err)
		}

		return u.Wallet().Votes, nil
	}

	balance, err := s.wallets.Spend(ctx, userID, 2*h.Ante(), user.Entry(user.VotePoker, uuid.Nil, ""))
	if err != nil {
		return 0, errs.Wrap("s.wallets.Spend", err)
	}
	if h.Payout() == 0 {
		return balance, nil
	}

	wallet, err := s.wallets.Credit(ctx, userID, h.Payout(), 0, user.Entry(user.VotePoker, uuid.Nil, ""))

	return wallet.Votes, errs.Wrap("s.wallets.Credit", err)
}

// Settings are the table and its stats; user.ErrNotAdmin.
func (s *PokerService) Settings(ctx context.Context, adminID uuid.UUID) (PokerSettings, error) {
	if err := checkAdmin(ctx, s.userRepo, s.admins, adminID); err != nil {
		return PokerSettings{}, errs.Wrap("checkAdmin", err)
	}

	p, err := s.pokers.Poker(ctx)
	if err != nil {
		return PokerSettings{}, errs.Wrap("s.pokers.Poker", err)
	}

	stats, err := s.pokers.Stats(ctx)
	if err != nil {
		return PokerSettings{}, errs.Wrap("s.pokers.Stats", err)
	}

	return PokerSettings{Poker: p, Stats: stats}, nil
}

// SetPoker changes the limits and the pays of the table; user.ErrNotAdmin.
func (s *PokerService) SetPoker(ctx context.Context, adminID uuid.UUID, p user.Poker) error {
	if err := checkAdmin(ctx, s.userRepo, s.admins, adminID); err != nil {
		return errs.Wrap("checkAdmin", err)
	}

	return errs.Wrap("s.pokers.SetPoker", s.pokers.SetPoker(ctx, p))
}
