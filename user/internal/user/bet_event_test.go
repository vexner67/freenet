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
		closesAt time.Time
		err      error
	}{
		{name: "no deadline", outcomes: []string{"Да", "Нет"}},
		{name: "deadline", outcomes: []string{"Да", "Нет"}, closesAt: now.Add(time.Hour)},
		{name: "one outcome", outcomes: []string{"Да"}, err: user.ErrBetOutcomes},
		{name: "too many", outcomes: strings.Split("1 2 3 4 5 6 7 8 9 10 11", " "), err: user.ErrBetOutcomes},
		{name: "twice", outcomes: []string{"Да", "да"}, err: user.ErrBetOutcomeTwice},
		{name: "past", outcomes: []string{"Да", "Нет"}, closesAt: now, err: user.ErrBetCloseTime},
		{name: "too far", outcomes: []string{"Да", "Нет"}, closesAt: now.Add(user.MaxBetEventAhead + time.Second), err: user.ErrBetCloseTime},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e, err := user.NewBetEvent(q, outcomes(t, tt.outcomes...), tt.closesAt, now)
			if !errors.Is(err, tt.err) {
				t.Fatalf("NewBetEvent: %v; want %v", err, tt.err)
			}
			if err == nil && (e.State() != user.BetEventOpen || e.Winner() != -1 || !e.ClosesAt().Equal(tt.closesAt)) {
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
	e, err := user.NewBetEvent(q, outcomes(t, "Да", "Нет"), now.Add(time.Hour), now)
	if err != nil {
		t.Fatal(err)
	}

	if err := e.Stake(0, 10, now); err != nil {
		t.Fatal(err)
	}
	if err := e.Stake(2, 10, now); !errors.Is(err, user.ErrBetOutcome) {
		t.Fatalf("Stake on outcome 2: %v", err)
	}
	if err := e.Stake(1, 0, now); !errors.Is(err, user.ErrStakeAmount) {
		t.Fatalf("Stake of 0: %v", err)
	}
	if err := e.Stake(1, user.MaxStake+1, now); !errors.Is(err, user.ErrStakeAmount) {
		t.Fatalf("Stake too big: %v", err)
	}
	if err := e.Stake(0, 10, now.Add(time.Hour)); !errors.Is(err, user.ErrBetEventClosed) {
		t.Fatalf("Stake at the deadline: %v", err)
	}
	if err := e.CheckDelete(); !errors.Is(err, user.ErrBetEventActive) {
		t.Fatalf("CheckDelete of an open event: %v", err)
	}

	closed, err := e.Closed()
	if err != nil || closed.State() != user.BetEventClosed || e.State() != user.BetEventOpen {
		t.Fatalf("Closed = %+v, %v", closed, err)
	}
	if err := closed.Stake(0, 10, now); !errors.Is(err, user.ErrBetEventClosed) {
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
