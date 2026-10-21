package user_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/user"
)

func outcomes(t *testing.T, names ...string) []user.BetOutcome {
	t.Helper()

	out := make([]user.BetOutcome, 0, len(names))
	for _, n := range names {
		o, err := user.NewBetOutcome(n)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, o)
	}

	return out
}

func TestNewBetQuestion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, in, want string
		err            error
	}{
		{name: "fine", in: "  Проиграют ли\nреспубликанцы?  ", want: "Проиграют ли республиканцы?"},
		{name: "empty", in: " \n ", err: user.ErrBetQuestionEmpty},
		{name: "longest", in: strings.Repeat("я", user.MaxBetQuestionLength), want: strings.Repeat("я", user.MaxBetQuestionLength)},
		{name: "too long", in: strings.Repeat("я", user.MaxBetQuestionLength+1), err: user.ErrBetQuestionTooLong},
		{name: "invisible", in: "вопрос\u200b", err: user.ErrInvisibleText},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			q, err := user.NewBetQuestion(tt.in)
			if !errors.Is(err, tt.err) || q.String() != tt.want {
				t.Fatalf("NewBetQuestion = %q, %v; want %q, %v", q, err, tt.want, tt.err)
			}
		})
	}
}

func TestNewBetOutcome(t *testing.T) {
	t.Parallel()

	if o, err := user.NewBetOutcome(" Да "); err != nil || o != "Да" {
		t.Fatalf("NewBetOutcome = %q, %v", o, err)
	}
	if _, err := user.NewBetOutcome(""); !errors.Is(err, user.ErrBetOutcomeEmpty) {
		t.Fatalf("empty: %v", err)
	}
	if _, err := user.NewBetOutcome(strings.Repeat("я", user.MaxBetOutcomeLength+1)); !errors.Is(err, user.ErrBetOutcomeTooLong) {
		t.Fatalf("too long: %v", err)
	}
}

func TestNewBetEvent(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	q, err := user.NewBetQuestion("Кто победит?")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		outcomes []string
		odds     []int
		closesAt time.Time
		err      error
	}{
		{name: "no deadline", outcomes: []string{"Да", "Нет"}, odds: []int{150, 250}},
		{name: "deadline", outcomes: []string{"Да", "Нет"}, odds: []int{user.MinEventOdds, 1000}, closesAt: now.Add(time.Hour)},
		{name: "one outcome", outcomes: []string{"Да"}, odds: []int{200}, err: user.ErrBetOutcomes},
		{name: "too many", outcomes: strings.Split("1 2 3 4 5 6 7 8 9 10 11", " "), odds: make([]int, 11), err: user.ErrBetOutcomes},
		{name: "no odds", outcomes: []string{"Да", "Нет"}, err: user.ErrEventOddsCount},
		{name: "odds short", outcomes: []string{"Да", "Нет"}, odds: []int{200}, err: user.ErrEventOddsCount},
		{name: "odds ×1", outcomes: []string{"Да", "Нет"}, odds: []int{100, 200}, err: user.ErrEventOdds},
		{name: "odds too big", outcomes: []string{"Да", "Нет"}, odds: []int{200, user.MaxEventOdds + 1}, err: user.ErrEventOdds},
		{name: "twice", outcomes: []string{"Да", "да"}, odds: []int{190, 190}, err: user.ErrBetOutcomeTwice},
		{name: "no margin", outcomes: []string{"Да", "Нет"}, odds: []int{200, 200}, err: user.ErrEventMargin},
		{name: "a margin of 5%", outcomes: []string{"1", "2", "3", "4"}, odds: []int{380, 380, 380, 380}},
		{name: "past", outcomes: []string{"Да", "Нет"}, odds: []int{190, 190}, closesAt: now, err: user.ErrBetCloseTime},
		{name: "too far", outcomes: []string{"Да", "Нет"}, odds: []int{190, 190}, closesAt: now.Add(user.MaxBetEventAhead + time.Second), err: user.ErrBetCloseTime},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e, err := user.NewBetEvent(q, outcomes(t, tt.outcomes...), tt.odds, tt.closesAt, now)
			if !errors.Is(err, tt.err) {
				t.Fatalf("NewBetEvent: %v; want %v", err, tt.err)
			}
			if err == nil && (e.State() != user.BetEventOpen || e.Winner() != -1 || !e.ClosesAt().Equal(tt.closesAt) || e.Pooled()) {
				t.Fatalf("NewBetEvent = %+v", e)
			}
		})
	}
}

func TestBetEvent_Lifecycle(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	q, err := user.NewBetQuestion("Кто победит?")
	if err != nil {
		t.Fatal(err)
	}
	e, err := user.NewBetEvent(q, outcomes(t, "Да", "Нет"), []int{150, 250}, now.Add(time.Hour), now)
	if err != nil {
		t.Fatal(err)
	}

	// A stake pays its votes times the odds, rounded down.
	if potential, err := e.Stake(0, 10, now); err != nil || potential != 15 {
		t.Fatalf("Stake on ×1.5 = %d, %v", potential, err)
	}
	if potential, err := e.Stake(1, 3, now); err != nil || potential != 7 {
		t.Fatalf("Stake on ×2.5 = %d, %v", potential, err)
	}
	if _, err := e.Stake(2, 10, now); !errors.Is(err, user.ErrBetOutcome) {
		t.Fatalf("Stake on outcome 2: %v", err)
	}
	if _, err := e.Stake(1, 0, now); !errors.Is(err, user.ErrStakeAmount) {
		t.Fatalf("Stake of 0: %v", err)
	}
	if _, err := e.Stake(1, user.MaxStake+1, now); !errors.Is(err, user.ErrStakeAmount) {
		t.Fatalf("Stake too big: %v", err)
	}
	if _, err := e.Stake(0, 10, now.Add(time.Hour)); !errors.Is(err, user.ErrBetEventClosed) {
		t.Fatalf("Stake at the deadline: %v", err)
	}

	// New odds are for the stakes made from now on.
	reodded, err := e.Reodded([]int{300, 120})
	if err != nil || reodded.Odds()[0] != 300 || e.Odds()[0] != 150 {
		t.Fatalf("Reodded = %+v, %v", reodded, err)
	}
	if potential, _ := reodded.Stake(0, 10, now); potential != 30 {
		t.Fatalf("Stake after Reodded = %d", potential)
	}
	if _, err := e.Reodded([]int{300}); !errors.Is(err, user.ErrEventOddsCount) {
		t.Fatalf("Reodded short: %v", err)
	}
	if err := e.CheckDelete(); !errors.Is(err, user.ErrBetEventActive) {
		t.Fatalf("CheckDelete of an open event: %v", err)
	}

	closed, err := e.Closed()
	if err != nil || closed.State() != user.BetEventClosed || e.State() != user.BetEventOpen {
		t.Fatalf("Closed = %+v, %v", closed, err)
	}
	if _, err := closed.Reodded([]int{190, 190}); err != nil {
		t.Fatalf("Reodded of a closed event: %v", err)
	}
	if _, err := closed.Stake(0, 10, now); !errors.Is(err, user.ErrBetEventClosed) {
		t.Fatalf("Stake on a closed event: %v", err)
	}

	if _, err := closed.Resolved(2); !errors.Is(err, user.ErrBetOutcome) {
		t.Fatalf("Resolved(2): %v", err)
	}
	resolved, err := closed.Resolved(1)
	if err != nil || resolved.State() != user.BetEventResolved || resolved.Winner() != 1 {
		t.Fatalf("Resolved = %+v, %v", resolved, err)
	}
	if _, err := resolved.Cancelled(); !errors.Is(err, user.ErrBetEventFinished) {
		t.Fatalf("Cancelled after resolved: %v", err)
	}
	if _, err := resolved.Reodded([]int{190, 190}); !errors.Is(err, user.ErrBetEventFinished) {
		t.Fatalf("Reodded after resolved: %v", err)
	}
	if _, err := resolved.Closed(); !errors.Is(err, user.ErrBetEventFinished) {
		t.Fatalf("Closed after resolved: %v", err)
	}
	if err := resolved.CheckDelete(); err != nil {
		t.Fatal(err)
	}

	cancelled, err := e.Cancelled()
	if err != nil || cancelled.State() != user.BetEventCancelled || cancelled.CheckDelete() != nil {
		t.Fatalf("Cancelled = %+v, %v", cancelled, err)
	}
}

func TestBetEvent_Payouts(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	q, err := user.NewBetQuestion("Кто победит?")
	if err != nil {
		t.Fatal(err)
	}
	e, err := user.NewBetEvent(q, outcomes(t, "Да", "Нет"), []int{120, 400}, time.Time{}, now)
	if err != nil {
		t.Fatal(err)
	}

	// Everyone on one outcome still wins: each stake pays its potential.
	ann, bob, eve := uuid.New(), uuid.New(), uuid.New()
	stakes := []user.Stake{
		{UserID: ann, Outcome: 0, Amount: 100, Potential: 120},
		{UserID: bob, Outcome: 0, Amount: 50, Potential: 60},
		{UserID: eve, Outcome: 1, Amount: 10, Potential: 40},
	}
	for i, want := range []int64{120, 60, 0} {
		if got := e.Payouts(stakes, 0)[i].Payout; got != want {
			t.Fatalf("Payouts(0)[%d] = %d; want %d", i, got, want)
		}
	}
	for i, want := range []int64{0, 0, 40} {
		if got := e.Payouts(stakes, 1)[i].Payout; got != want {
			t.Fatalf("Payouts(1)[%d] = %d; want %d", i, got, want)
		}
	}
	// Cancelled: the stakes come back.
	for i, s := range e.Payouts(stakes, -1) {
		if s.Payout != stakes[i].Amount {
			t.Fatalf("Payouts(-1)[%d] = %d", i, s.Payout)
		}
	}

	// An event made before the odds shares the stakes.
	pooled := user.RestoreBetEvent(uuid.New(), q, outcomes(t, "Да", "Нет"), nil, user.BetEventOpen, time.Time{}, -1)
	if !pooled.Pooled() {
		t.Fatal("RestoreBetEvent without odds is not pooled")
	}
	if got := pooled.Payouts(stakes, 1)[2].Payout; got != 160 {
		t.Fatalf("pooled Payouts(1) = %d; want 160", got)
	}
	if potential, err := pooled.Stake(0, 10, now); err != nil || potential != 0 {
		t.Fatalf("pooled Stake = %d, %v", potential, err)
	}
	if _, err := pooled.Reodded([]int{190, 190}); !errors.Is(err, user.ErrEventPooled) {
		t.Fatalf("pooled Reodded: %v", err)
	}
}

func TestPayouts(t *testing.T) {
	t.Parallel()

	ann, bob, eve := uuid.New(), uuid.New(), uuid.New()
	stakes := []user.Stake{
		{UserID: ann, Outcome: 0, Amount: 30},
		{UserID: bob, Outcome: 0, Amount: 70},
		{UserID: eve, Outcome: 1, Amount: 101},
	}

	// The pool of 201 goes to those on outcome 0 by their stakes.
	got := user.Payouts(stakes, 0)
	want := []int64{60, 140, 0}
	for i, s := range got {
		if s.Payout != want[i] {
			t.Fatalf("Payouts(0)[%d] = %d; want %d", i, s.Payout, want[i])
		}
	}

	// Nobody on outcome 2 of three: everyone gets their stake back, as on
	// a cancelled event.
	for _, winner := range []int{2, -1} {
		for i, s := range user.Payouts(stakes, winner) {
			if s.Payout != stakes[i].Amount {
				t.Fatalf("Payouts(%d)[%d] = %d", winner, i, s.Payout)
			}
		}
	}

	// Big stakes do not overflow.
	big := []user.Stake{{Outcome: 0, Amount: 3_000_000_000}, {Outcome: 1, Amount: 9_000_000_000_000}}
	if got := user.Payouts(big, 0); got[0].Payout != 9_003_000_000_000 {
		t.Fatalf("big Payouts = %d", got[0].Payout)
	}
}

func TestCheckStake(t *testing.T) {
	t.Parallel()

	ann, bob := uuid.New(), uuid.New()
	stakes := []user.Stake{
		{UserID: ann, Outcome: 0, Amount: 200, Potential: 1000},
		{UserID: bob, Outcome: 0, Amount: 100, Potential: 1900},
		{UserID: bob, Outcome: 1, Amount: 50, Potential: 75},
	}

	tests := []struct {
		name      string
		userID    uuid.UUID
		outcome   int
		amount    int64
		potential int64
		want      error
	}{
		{name: "fine", userID: ann, outcome: 1, amount: 100, potential: 150},
		{name: "the most of a person", userID: ann, outcome: 1, amount: 101, potential: 150, want: user.ErrEventStakeLimit},
		{name: "the most of an outcome", userID: uuid.New(), outcome: 0, amount: 50, potential: 100},
		{name: "the outcome is full", userID: uuid.New(), outcome: 0, amount: 50, potential: 101, want: user.ErrEventFull},
	}

	for _, tt := range tests {
		if err := user.CheckStake(stakes, tt.userID, tt.outcome, tt.amount, tt.potential); !errors.Is(err, tt.want) {
			t.Errorf("%s: CheckStake = %v, want %v", tt.name, err, tt.want)
		}
	}
}
