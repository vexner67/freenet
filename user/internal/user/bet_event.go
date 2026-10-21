package user

import (
	"errors"
	"math/big"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	// MaxBetQuestionLength is the longest question of an event, one line.
	MaxBetQuestionLength = 200
	// MaxBetOutcomeLength is the longest outcome of an event, one line.
	MaxBetOutcomeLength = 64
	// MinBetOutcomes and MaxBetOutcomes are how many outcomes an event has.
	MinBetOutcomes = 2
	MaxBetOutcomes = 10
	// MaxBetEventAhead is how far ahead the stakes of an event may close.
	MaxBetEventAhead = 366 * 24 * time.Hour
	// MaxStake is the most votes one stake puts on an outcome.
	MaxStake = 100_000_000
	// MinEventOdds and MaxEventOdds bound the odds of an outcome, in
	// hundredths: 101 is ×1.01, 100000 ×1000.
	MinEventOdds = 101
	MaxEventOdds = 100_000
	// MinEventMargin is the least sum of 1/odds of the outcomes of an
	// event, in percent: the treasury keeps 5% of what a fair book pays.
	MinEventMargin = 105
	// MaxPersonStakes is the most one person stakes on one event in all.
	MaxPersonStakes = 300
	// MaxOutcomePayout is the most the stakes on one outcome pay together.
	MaxOutcomePayout = 3_000
)

var (
	ErrBetQuestionEmpty   = errors.New("the question of the event is empty")
	ErrBetQuestionTooLong = errors.New("the question of the event is too long")
	ErrBetOutcomeEmpty    = errors.New("an outcome of the event is empty")
	ErrBetOutcomeTooLong  = errors.New("an outcome of the event is too long")
	ErrBetOutcomes        = errors.New("an event has from 2 to 10 outcomes")
	ErrBetOutcomeTwice    = errors.New("the outcomes of the event repeat")
	ErrBetCloseTime       = errors.New("the stakes must close in the future and within a year")
	ErrBetEventNotFound   = errors.New("event not found")
	ErrBetEventClosed     = errors.New("the event takes no more stakes")
	ErrBetEventFinished   = errors.New("the event is over already")
	ErrBetEventActive     = errors.New("the event is not over yet")
	ErrBetOutcome         = errors.New("unknown outcome")
	ErrStakeAmount        = errors.New("a stake must be from 1 to 100000000 votes")
	ErrEventOdds          = errors.New("the odds of an outcome must be from 1.01 to 1000")
	ErrEventOddsCount     = errors.New("every outcome needs its odds")
	ErrEventPooled        = errors.New("the event shares its stakes and has no odds")
	ErrEventMargin        = errors.New("the odds pay too much: the sum of 1/odds must be at least 1.05")
	ErrEventStakeLimit    = errors.New("one person stakes at most 300 votes on an event")
	ErrEventFull          = errors.New("the outcome takes no more stakes: its payouts reached 3000 votes")
)

// BetQuestion is what an event asks: "Will the Republicans lose the
// elections in November?".
type BetQuestion string

func NewBetQuestion(s string) (BetQuestion, error) {
	s, err := visibleText(strings.Join(strings.Fields(s), " "))
	if err != nil {
		return "", err
	}

	switch {
	case s == "":
		return "", ErrBetQuestionEmpty
	case utf8.RuneCountInString(s) > MaxBetQuestionLength:
		return "", ErrBetQuestionTooLong
	}

	return BetQuestion(s), nil
}

func (q BetQuestion) String() string {
	return string(q)
}

// BetOutcome is one answer to the question of an event: "Yes", "No".
type BetOutcome string

func NewBetOutcome(s string) (BetOutcome, error) {
	s, err := visibleText(strings.Join(strings.Fields(s), " "))
	if err != nil {
		return "", err
	}

	switch {
	case s == "":
		return "", ErrBetOutcomeEmpty
	case utf8.RuneCountInString(s) > MaxBetOutcomeLength:
		return "", ErrBetOutcomeTooLong
	}

	return BetOutcome(s), nil
}

func (o BetOutcome) String() string {
	return string(o)
}

// BetEventState is where an event is: taking stakes, closed to them by the
// administrators, resolved with a winning outcome or cancelled.
type BetEventState string

const (
	BetEventOpen      BetEventState = "open"
	BetEventClosed    BetEventState = "closed"
	BetEventResolved  BetEventState = "resolved"
	BetEventCancelled BetEventState = "cancelled"
)

// NewBetEventState checks a state read from the store.
func NewBetEventState(s string) (BetEventState, error) {
	switch st := BetEventState(s); st {
	case BetEventOpen, BetEventClosed, BetEventResolved, BetEventCancelled:
		return st, nil
	}

	return "", errors.New("unknown state of an event")
}

// Over tells that the event was resolved or cancelled: its stakes are paid.
func (s BetEventState) Over() bool {
	return s == BetEventResolved || s == BetEventCancelled
}

// BetEvent is an event the administrators ask about, as the bookmakers
// do: each outcome has its odds, people stake votes on the outcomes, and
// when the event is resolved, a stake on the winner pays its votes times
// the odds it was made at, however many staked on the same outcome. The
// administrators may change the odds while the event takes stakes; the
// stakes made keep theirs. Events made before the odds have none: those who
// guessed share all the votes staked (parimutuel). A cancelled event gives
// everyone their stakes back.
type BetEvent struct {
	id       uuid.UUID
	question BetQuestion
	outcomes []BetOutcome
	// odds are those of each outcome, in hundredths; nil for an event
	// sharing its stakes.
	odds     []int
	state    BetEventState
	closesAt time.Time
	// winner is the index of the winning outcome of a resolved event, -1
	// otherwise.
	winner int
}

// NewBetEvent is an open event with the odds of each outcome; closesAt is
// zero for stakes until the administrators close them, or within
// MaxBetEventAhead of now.
func NewBetEvent(question BetQuestion, outcomes []BetOutcome, odds []int, closesAt, now time.Time) (*BetEvent, error) {
	if len(outcomes) < MinBetOutcomes || len(outcomes) > MaxBetOutcomes {
		return nil, ErrBetOutcomes
	}
	if err := checkOdds(odds, len(outcomes)); err != nil {
		return nil, err
	}

	seen := make(map[string]bool, len(outcomes))
	for _, o := range outcomes {
		key := strings.ToLower(o.String())
		if seen[key] {
			return nil, ErrBetOutcomeTwice
		}
		seen[key] = true
	}

	if !closesAt.IsZero() && (!closesAt.After(now) || closesAt.After(now.Add(MaxBetEventAhead))) {
		return nil, ErrBetCloseTime
	}

	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	return &BetEvent{
		id:       id,
		question: question,
		outcomes: append([]BetOutcome(nil), outcomes...),
		odds:     append([]int(nil), odds...),
		state:    BetEventOpen,
		closesAt: closesAt.UTC(),
		winner:   -1,
	}, nil
}

// checkOdds checks the odds of n outcomes; ErrEventOddsCount, ErrEventOdds.
func checkOdds(odds []int, n int) error {
	if len(odds) != n {
		return ErrEventOddsCount
	}
	// The sum of 1/odds, as chances in percent.
	var sum float64
	for _, o := range odds {
		if o < MinEventOdds || o > MaxEventOdds {
			return ErrEventOdds
		}

		sum += 100 / float64(o)
	}

	// The tolerance forgives the floating point of a sum right at 1.05.
	if sum*100 < MinEventMargin-1e-9 {
		return ErrEventMargin
	}

	return nil
}

// CheckStake is ErrEventStakeLimit when userID would stake more than
// MaxPersonStakes on the event in all, and ErrEventFull when the stakes on
// the outcome would pay more than MaxOutcomePayout, with a stake of amount
// paying potential added to the stakes made.
func CheckStake(stakes []Stake, userID uuid.UUID, outcome int, amount, potential int64) error {
	mine, owed := amount, potential
	for _, s := range stakes {
		if s.UserID == userID {
			mine += s.Amount
		}
		if s.Outcome == outcome {
			owed += s.Potential
		}
	}

	switch {
	case mine > MaxPersonStakes:
		return ErrEventStakeLimit
	case owed > MaxOutcomePayout:
		return ErrEventFull
	}

	return nil
}

// RestoreBetEvent is an event read from the store; odds nil for one
// sharing its stakes.
func RestoreBetEvent(id uuid.UUID, question BetQuestion, outcomes []BetOutcome, odds []int, state BetEventState, closesAt time.Time, winner int) *BetEvent {
	return &BetEvent{id: id, question: question, outcomes: outcomes, odds: odds, state: state, closesAt: closesAt, winner: winner}
}

func (e *BetEvent) ID() uuid.UUID {
	return e.id
}

func (e *BetEvent) Question() BetQuestion {
	return e.question
}

func (e *BetEvent) Outcomes() []BetOutcome {
	return e.outcomes
}

// Odds are those of each outcome, in hundredths; nil for an event sharing
// its stakes.
func (e *BetEvent) Odds() []int {
	return e.odds
}

// Pooled tells that those who guessed share the stakes: the event has no
// odds.
func (e *BetEvent) Pooled() bool {
	return e.odds == nil
}

func (e *BetEvent) State() BetEventState {
	return e.state
}

// ClosesAt is when the stakes close by themselves; zero for never.
func (e *BetEvent) ClosesAt() time.Time {
	return e.closesAt
}

// Winner is the index of the winning outcome, -1 unless resolved.
func (e *BetEvent) Winner() int {
	return e.winner
}

// CreatedAt is when the event was made, the time of its UUIDv7.
func (e *BetEvent) CreatedAt() time.Time {
	sec, nsec := e.id.Time().UnixTime()

	return time.Unix(sec, nsec).UTC()
}

// OpenAt tells whether the event takes stakes at now.
func (e *BetEvent) OpenAt(now time.Time) bool {
	return e.state == BetEventOpen && (e.closesAt.IsZero() || now.Before(e.closesAt))
}

// Stake checks a stake of amount votes on the outcome at now and tells what
// it pays if the outcome wins: the votes times its odds now, rounded down;
// 0 for an event sharing its stakes. ErrBetEventClosed, ErrBetOutcome,
// ErrStakeAmount.
func (e *BetEvent) Stake(outcome int, amount int64, now time.Time) (int64, error) {
	switch {
	case !e.OpenAt(now):
		return 0, ErrBetEventClosed
	case outcome < 0 || outcome >= len(e.outcomes):
		return 0, ErrBetOutcome
	case amount < 1 || amount > MaxStake:
		return 0, ErrStakeAmount
	}

	if e.Pooled() {
		return 0, nil
	}

	return amount * int64(e.odds[outcome]) / 100, nil
}

// Reodded is the event with new odds, for the stakes made from now on;
// ErrBetEventFinished, ErrEventPooled, ErrEventOddsCount, ErrEventOdds.
func (e *BetEvent) Reodded(odds []int) (*BetEvent, error) {
	switch {
	case e.state.Over():
		return nil, ErrBetEventFinished
	case e.Pooled():
		return nil, ErrEventPooled
	}
	if err := checkOdds(odds, len(e.outcomes)); err != nil {
		return nil, err
	}

	changed := *e
	changed.odds = append([]int(nil), odds...)

	return &changed, nil
}

// Closed is the event closed to stakes; ErrBetEventFinished for one
// resolved or cancelled. Closing a closed event changes nothing.
func (e *BetEvent) Closed() (*BetEvent, error) {
	if e.state.Over() {
		return nil, ErrBetEventFinished
	}

	closed := *e
	closed.state = BetEventClosed

	return &closed, nil
}

// Resolved is the event with its winning outcome; ErrBetEventFinished,
// ErrBetOutcome.
func (e *BetEvent) Resolved(winner int) (*BetEvent, error) {
	if e.state.Over() {
		return nil, ErrBetEventFinished
	}
	if winner < 0 || winner >= len(e.outcomes) {
		return nil, ErrBetOutcome
	}

	resolved := *e
	resolved.state, resolved.winner = BetEventResolved, winner

	return &resolved, nil
}

// Cancelled is the event called off; ErrBetEventFinished.
func (e *BetEvent) Cancelled() (*BetEvent, error) {
	if e.state.Over() {
		return nil, ErrBetEventFinished
	}

	cancelled := *e
	cancelled.state = BetEventCancelled

	return &cancelled, nil
}

// CheckDelete lets only an event that is over go: the stakes of the others
// are not paid yet; ErrBetEventActive.
func (e *BetEvent) CheckDelete() error {
	if !e.state.Over() {
		return ErrBetEventActive
	}

	return nil
}

// Stake is the votes one person put on one outcome of an event, what they
// pay if it wins — the votes times the odds of each stake, summed — and
// what they paid back once the event was over.
type Stake struct {
	UserID    uuid.UUID
	Outcome   int
	Amount    int64
	Potential int64
	Payout    int64
}

// Payouts are what the stakes of the event pay when it is resolved with
// winner, or cancelled with -1: everyone gets their stakes back. With odds
// a stake on the winner pays its Potential and the rest nothing; without
// them those who guessed share the stakes (Payouts).
func (e *BetEvent) Payouts(stakes []Stake, winner int) []Stake {
	if e.Pooled() || winner < 0 {
		return Payouts(stakes, winner)
	}

	paid := make([]Stake, len(stakes))
	for i, s := range stakes {
		s.Payout = 0
		if s.Outcome == winner {
			s.Payout = s.Potential
		}
		paid[i] = s
	}

	return paid
}

// Payouts are what the stakes of an event sharing them pay when it is
// resolved with winner: those on it share all the votes staked, each by
// their stake, rounded down; when nobody guessed, everyone gets their
// stakes back, as when the event is cancelled (winner -1).
func Payouts(stakes []Stake, winner int) []Stake {
	var pool, won int64
	for _, s := range stakes {
		pool += s.Amount
		if s.Outcome == winner {
			won += s.Amount
		}
	}

	paid := make([]Stake, len(stakes))
	for i, s := range stakes {
		s.Payout = 0
		switch {
		case won == 0:
			s.Payout = s.Amount
		case s.Outcome == winner:
			s.Payout = share(s.Amount, pool, won)
		}
		paid[i] = s
	}

	return paid
}

// share is amount * pool / won rounded down, without overflowing: the
// stakes may be big.
func share(amount, pool, won int64) int64 {
	n := new(big.Int).Mul(big.NewInt(amount), big.NewInt(pool))

	return n.Quo(n, big.NewInt(won)).Int64()
}
