package service

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// MaxListedBetEvents is how many events a list shows.
const MaxListedBetEvents = 100

// BetEventRepository keeps the events people stake votes on and the
// stakes.
type BetEventRepository interface {
	Create(ctx context.Context, e *user.BetEvent) error
	// Update writes the state, the winner and the odds of the event.
	Update(ctx context.Context, e *user.BetEvent) error
	// Lock reads the event FOR UPDATE: its stakes and the changes of its
	// state go one by one, so the limits of the stakes hold;
	// user.ErrBetEventNotFound.
	Lock(ctx context.Context, id uuid.UUID) (*user.BetEvent, error)
	// List is the events with their stakes and the stakes of viewerID
	// (uuid.Nil for nobody), at most MaxListedBetEvents.
	List(ctx context.Context, list BetEventList, viewerID uuid.UUID) ([]BetEventView, error)
	// AddStake adds amount votes of userID on the outcome, paying potential
	// if it wins, to what they staked on it before.
	AddStake(ctx context.Context, eventID, userID uuid.UUID, outcome int, amount, potential int64) error
	// Stakes are all the stakes of the event, one per person and outcome.
	Stakes(ctx context.Context, eventID uuid.UUID) ([]user.Stake, error)
	// SetPayouts writes what the stakes paid back.
	SetPayouts(ctx context.Context, eventID uuid.UUID, stakes []user.Stake) error
	// Delete removes the event with its stakes; user.ErrBetEventNotFound.
	Delete(ctx context.Context, id uuid.UUID) error
}

// BetEventList is which events a list shows.
type BetEventList int

const (
	// BetEventsAll are all the events, those taking stakes or waiting to be
	// resolved first, then the newest.
	BetEventsAll BetEventList = iota
	// BetEventsPopular are the events taking stakes, the most staked on
	// first.
	BetEventsPopular
)

// BetEventView is an event with its stakes, as a list shows it.
type BetEventView struct {
	Event *user.BetEvent
	// Staked and Bettors are the votes and the people on each outcome.
	Staked  []int64
	Bettors []int
	// People are those who staked on any outcome.
	People int
	// Mine are the votes of the viewer on each outcome, Potential what they
	// pay if it wins, Payout what they paid back once the event is over.
	Mine      []int64
	Potential []int64
	Payout    int64
	// Open tells that the event takes stakes now.
	Open bool
}

// BetEventService is the bets on events: the administrators ask and set
// the odds, people stake votes on the outcomes, a stake on the winner pays
// its votes times its odds.
type BetEventService struct {
	userRepo UserRepository
	events   BetEventRepository
	wallets  WalletRepository
	tx       TxManager
	admins   user.Admins
	now      func() time.Time
}

func NewBetEventService(users UserRepository, events BetEventRepository, wallets WalletRepository, tx TxManager, admins user.Admins) *BetEventService {
	return &BetEventService{userRepo: users, events: events, wallets: wallets, tx: tx, admins: admins, now: time.Now}
}

// WithNow makes the service tell the time by now: for tests.
func (s *BetEventService) WithNow(now func() time.Time) *BetEventService {
	s.now = now

	return s
}

// Events are the events of the list with the stakes of viewerID (uuid.Nil
// for a guest); anyone may ask.
func (s *BetEventService) Events(ctx context.Context, viewerID uuid.UUID, list BetEventList) ([]BetEventView, error) {
	views, err := s.events.List(ctx, list, viewerID)
	if err != nil {
		return nil, errs.Wrap("s.events.List", err)
	}

	now := s.now()
	for i := range views {
		views[i].Open = views[i].Event.OpenAt(now)
	}

	return views, nil
}

// Create asks a new question with the odds of each outcome; user.ErrNotAdmin.
func (s *BetEventService) Create(ctx context.Context, adminID uuid.UUID, question user.BetQuestion, outcomes []user.BetOutcome, odds []int, closesAt time.Time) (*user.BetEvent, error) {
	if err := checkAdmin(ctx, s.userRepo, s.admins, adminID); err != nil {
		return nil, errs.Wrap("checkAdmin", err)
	}

	e, err := user.NewBetEvent(question, outcomes, odds, closesAt, s.now())
	if err != nil {
		return nil, errs.Wrap("user.NewBetEvent", err)
	}

	if err = s.events.Create(ctx, e); err != nil {
		return nil, errs.Wrap("s.events.Create", err)
	}

	return e, nil
}

// Close stops the stakes of the event; user.ErrNotAdmin,
// user.ErrBetEventNotFound, user.ErrBetEventFinished.
func (s *BetEventService) Close(ctx context.Context, adminID, id uuid.UUID) error {
	return s.change(ctx, adminID, id, func(e *user.BetEvent) (*user.BetEvent, error) {
		closed, err := e.Closed()

		return closed, errs.Wrap("e.Closed", err)
	})
}

// SetOdds changes the odds of the outcomes for the stakes made from now
// on; user.ErrBetEventFinished, user.ErrEventPooled, user.ErrEventOdds,
// user.ErrEventOddsCount as well.
func (s *BetEventService) SetOdds(ctx context.Context, adminID, id uuid.UUID, odds []int) error {
	return s.change(ctx, adminID, id, func(e *user.BetEvent) (*user.BetEvent, error) {
		changed, err := e.Reodded(odds)

		return changed, errs.Wrap("e.Reodded", err)
	})
}

// Resolve names the winning outcome and pays the winners, in one
// transaction; user.ErrBetOutcome as well.
func (s *BetEventService) Resolve(ctx context.Context, adminID, id uuid.UUID, winner int) error {
	return s.change(ctx, adminID, id, func(e *user.BetEvent) (*user.BetEvent, error) {
		resolved, err := e.Resolved(winner)
		if err != nil {
			return nil, errs.Wrap("e.Resolved", err)
		}

		return resolved, errs.Wrap("s.pay", s.pay(ctx, e, winner))
	})
}

// Cancel calls the event off and gives everyone their stakes back.
func (s *BetEventService) Cancel(ctx context.Context, adminID, id uuid.UUID) error {
	return s.change(ctx, adminID, id, func(e *user.BetEvent) (*user.BetEvent, error) {
		cancelled, err := e.Cancelled()
		if err != nil {
			return nil, errs.Wrap("e.Cancelled", err)
		}

		return cancelled, errs.Wrap("s.pay", s.pay(ctx, e, -1))
	})
}

// Delete removes an event that is over; user.ErrBetEventActive as well.
func (s *BetEventService) Delete(ctx context.Context, adminID, id uuid.UUID) error {
	if err := checkAdmin(ctx, s.userRepo, s.admins, adminID); err != nil {
		return errs.Wrap("checkAdmin", err)
	}

	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		e, err := s.events.Lock(ctx, id)
		if err != nil {
			return errs.Wrap("s.events.Lock", err)
		}

		if err = e.CheckDelete(); err != nil {
			return errs.Wrap("e.CheckDelete", err)
		}

		return errs.Wrap("s.events.Delete", s.events.Delete(ctx, id))
	})

	return errs.Wrap("s.tx.WithinTx", err)
}

// Stake takes amount votes of userID and puts them on the outcome, in one
// transaction, and returns what is left; user.ErrBetEventNotFound,
// user.ErrBetEventClosed, user.ErrBetOutcome, user.ErrStakeAmount,
// user.ErrNotEnoughVotes.
func (s *BetEventService) Stake(ctx context.Context, userID, id uuid.UUID, outcome int, amount int64) (int64, error) {
	var balance int64

	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		e, err := s.events.Lock(ctx, id)
		if err != nil {
			return errs.Wrap("s.events.Lock", err)
		}

		potential, err := e.Stake(outcome, amount, s.now())
		if err != nil {
			return errs.Wrap("e.Stake", err)
		}

		if balance, err = s.wallets.Spend(ctx, userID, amount, user.Entry(user.VoteStake, uuid.Nil, e.Question().String())); err != nil {
			return errs.Wrap("s.wallets.Spend", err)
		}

		return errs.Wrap("s.events.AddStake", s.events.AddStake(ctx, id, userID, outcome, amount, potential))
	})
	if err != nil {
		return 0, errs.Wrap("s.tx.WithinTx", err)
	}

	return balance, nil
}

// change makes a change of the state of an event by an administrator, in
// one transaction with the event locked.
func (s *BetEventService) change(ctx context.Context, adminID, id uuid.UUID, change func(*user.BetEvent) (*user.BetEvent, error)) error {
	if err := checkAdmin(ctx, s.userRepo, s.admins, adminID); err != nil {
		return errs.Wrap("checkAdmin", err)
	}

	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		e, err := s.events.Lock(ctx, id)
		if err != nil {
			return errs.Wrap("s.events.Lock", err)
		}

		changed, err := change(e)
		if err != nil {
			return errs.Wrap("change", err)
		}

		return errs.Wrap("s.events.Update", s.events.Update(ctx, changed))
	})

	return errs.Wrap("s.tx.WithinTx", err)
}

// pay pays the stakes of the event resolved with winner (-1 for
// cancelled) back to the people.
func (s *BetEventService) pay(ctx context.Context, e *user.BetEvent, winner int) error {
	id := e.ID()

	stakes, err := s.events.Stakes(ctx, id)
	if err != nil {
		return errs.Wrap("s.events.Stakes", err)
	}

	paid := e.Payouts(stakes, winner)

	owed := make(map[uuid.UUID]int64)
	var order []uuid.UUID
	for _, st := range paid {
		if st.Payout == 0 {
			continue
		}
		if _, ok := owed[st.UserID]; !ok {
			order = append(order, st.UserID)
		}
		owed[st.UserID] += st.Payout
	}

	// In the order of the IDs, as Move locks the rows: two payouts at once
	// never wait for each other in a circle.
	slices.SortFunc(order, func(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) })

	for _, userID := range order {
		// A page erased meanwhile takes its stake with it.
		if _, err := s.wallets.Credit(ctx, userID, owed[userID], 0, user.Entry(user.VoteStake, uuid.Nil, e.Question().String())); err != nil && !errors.Is(err, user.ErrNotFound) {
			return errs.Wrap("s.wallets.Credit", err)
		}
	}

	return errs.Wrap("s.events.SetPayouts", s.events.SetPayouts(ctx, id, paid))
}
