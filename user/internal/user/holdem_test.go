package user_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/user"
)

var holdemStart = time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)

// deckOf is a shuffle that lays first the cards of s, then the rest.
func deckOf(t *testing.T, s string) func() []user.Card {
	t.Helper()

	first := cards(t, s)

	return func() []user.Card {
		deck := append([]user.Card{}, first...)
		for c := range user.Card(user.DeckSize) {
			seen := false
			for _, f := range first {
				seen = seen || f == c
			}
			if !seen {
				deck = append(deck, c)
			}
		}

		return deck
	}
}

func newTable(t *testing.T, seats int, players ...uuid.UUID) *user.HoldemTable {
	t.Helper()

	table, err := user.NewHoldemTable(uuid.New(), players[0], "", false, seats, 10, holdemStart)
	if err != nil {
		t.Fatalf("NewHoldemTable: %v", err)
	}
	for _, id := range players {
		if err = table.Sit(id, -1, 1000, holdemStart); err != nil {
			t.Fatalf("Sit: %v", err)
		}
	}

	return table
}

func act(t *testing.T, table *user.HoldemTable, id uuid.UUID, action user.TableAction, amount int64) {
	t.Helper()

	if err := table.Act(id, action, amount, holdemStart.Add(5*time.Second)); err != nil {
		t.Fatalf("Act %s: %v", action, err)
	}
}

func TestHoldem_HeadsUpToShowdown(t *testing.T) {
	t.Parallel()

	a, b := uuid.New(), uuid.New()
	table := newTable(t, 2, a, b)

	// Seat 0 is the button and the small blind heads-up; seat 1 gets the
	// first card. B: Kh Kd, A: As Ad; the board gives A nothing better.
	payouts, changed := table.At(holdemStart.Add(4*time.Second), deckOf(t, "Kh As Kd Ad 2c 7s 9h 3d 4c"))
	if !changed || len(payouts) != 0 {
		t.Fatalf("At = %v, %v", payouts, changed)
	}
	if table.Street() != user.StreetPreflop || table.Turn() != 0 {
		t.Fatalf("street %s, turn %d", table.Street(), table.Turn())
	}
	if table.Pot() != 15 {
		t.Fatalf("pot = %d, want blinds 15", table.Pot())
	}

	act(t, table, a, user.ActCall, 0)
	if table.Street() != user.StreetPreflop || table.Turn() != 1 {
		t.Fatalf("the big blind has the option: %s, %d", table.Street(), table.Turn())
	}
	act(t, table, b, user.ActCheck, 0)

	if table.Street() != user.StreetFlop || len(table.Board()) != 3 || table.Turn() != 1 {
		t.Fatalf("flop: %s, %v, turn %d", table.Street(), table.Board(), table.Turn())
	}

	act(t, table, b, user.ActRaise, 50)
	act(t, table, a, user.ActCall, 0)
	act(t, table, b, user.ActCheck, 0)
	act(t, table, a, user.ActCheck, 0)
	act(t, table, b, user.ActCheck, 0)
	act(t, table, a, user.ActCheck, 0)

	if table.Street() != user.StreetShowdown {
		t.Fatalf("street = %s", table.Street())
	}

	seats := table.Seats()
	if seats[0].Stack != 1060 || seats[1].Stack != 940 || seats[0].Won != 120 || !seats[1].Shown {
		t.Fatalf("stacks %d, %d, won %d", seats[0].Stack, seats[1].Stack, seats[0].Won)
	}

	// The pause over, the next hand moves the button.
	table.At(holdemStart.Add(12*time.Second), deckOf(t, ""))
	if table.Hand() != 2 || table.Button() != 1 || table.Turn() != 1 {
		t.Fatalf("hand %d, button %d, turn %d", table.Hand(), table.Button(), table.Turn())
	}
}

func TestHoldem_FoldGivesThePot(t *testing.T) {
	t.Parallel()

	a, b, c := uuid.New(), uuid.New(), uuid.New()
	table := newTable(t, 6, a, b, c)
	table.At(holdemStart.Add(4*time.Second), deckOf(t, ""))

	// Button 0, small blind 1, big blind 2: the button acts first.
	if table.Turn() != 0 {
		t.Fatalf("turn = %d", table.Turn())
	}
	if err := table.Act(b, user.ActCall, 0, holdemStart); !errors.Is(err, user.ErrTableNotYourTurn) {
		t.Fatalf("out of turn: %v", err)
	}
	if err := table.Act(a, user.ActRaise, 15, holdemStart); !errors.Is(err, user.ErrTableRaise) {
		t.Fatalf("a short raise: %v", err)
	}

	act(t, table, a, user.ActRaise, 30)
	act(t, table, b, user.ActFold, 0)
	act(t, table, c, user.ActFold, 0)

	seats := table.Seats()
	if table.Street() != user.StreetShowdown || seats[0].Stack != 1015 || seats[0].Shown {
		t.Fatalf("street %s, stack %d", table.Street(), seats[0].Stack)
	}
}

func TestHoldem_SidePots(t *testing.T) {
	t.Parallel()

	a, b, c := uuid.New(), uuid.New(), uuid.New()
	table, err := user.NewHoldemTable(uuid.New(), a, "Пятница", false, 3, 10, holdemStart)
	if err != nil {
		t.Fatal(err)
	}
	for id, buyIn := range map[uuid.UUID]int64{a: 1000, b: 200, c: 600} {
		seat := map[uuid.UUID]int{a: 0, b: 1, c: 2}[id]
		if err = table.Sit(id, seat, buyIn, holdemStart); err != nil {
			t.Fatal(err)
		}
	}

	// Deal from seat 1: B Ah Ac, C Kh Kc, A Qh Qc; B has the best hand,
	// C the second, A the worst.
	table.At(holdemStart.Add(4*time.Second), deckOf(t, "Ah Kh Qh Ac Kc Qc 2d 7s 9d 3s 4c"))

	act(t, table, a, user.ActAllIn, 0)
	act(t, table, b, user.ActAllIn, 0)
	act(t, table, c, user.ActAllIn, 0)

	seats := table.Seats()
	if table.Street() != user.StreetShowdown {
		t.Fatalf("street = %s", table.Street())
	}
	// The main pot 600 to B, the side pot of C's level 800 to C, A's
	// 400 uncalled back to A.
	if seats[1].Stack != 600 || seats[2].Stack != 800 || seats[0].Stack != 400 {
		t.Fatalf("stacks A %d, B %d, C %d", seats[0].Stack, seats[1].Stack, seats[2].Stack)
	}

}

func TestHoldem_TimeoutsSitOut(t *testing.T) {
	t.Parallel()

	a, b := uuid.New(), uuid.New()
	table := newTable(t, 2, a, b)
	table.At(holdemStart.Add(4*time.Second), deckOf(t, ""))

	// Nobody acts: the small blind folds, then the next hand again.
	table.At(holdemStart.Add(4*time.Second+user.TableTurn), deckOf(t, ""))
	if table.Street() != user.StreetShowdown || table.Seats()[0].Missed != 1 {
		t.Fatalf("street %s, missed %d", table.Street(), table.Seats()[0].Missed)
	}

	// The small blinds pass by turns: A misses twice first, sits out, the
	// table waits, and A stands up with their chips.
	payouts, _ := table.At(holdemStart.Add(time.Hour), deckOf(t, ""))
	if len(payouts) != 1 || payouts[0].UserID != a || table.Players() != 1 || table.Street() != user.StreetWaiting {
		t.Fatalf("payouts %v, players %d, street %s", payouts, table.Players(), table.Street())
	}
	if payouts[0].Chips+table.Seats()[1].Stack != 2000 {
		t.Fatalf("chips %d and %d", payouts[0].Chips, table.Seats()[1].Stack)
	}
}

func TestHoldem_LeaveMidHand(t *testing.T) {
	t.Parallel()

	a, b, c := uuid.New(), uuid.New(), uuid.New()
	table := newTable(t, 3, a, b, c)
	table.At(holdemStart.Add(4*time.Second), deckOf(t, ""))

	// The big blind leaves out of turn: their blind stays in the pot.
	payout, err := table.Leave(c, holdemStart.Add(5*time.Second))
	if err != nil || payout.Chips != 990 {
		t.Fatalf("Leave = %v, %v", payout, err)
	}

	act(t, table, a, user.ActFold, 0)

	seats := table.Seats()
	if table.Street() != user.StreetShowdown || seats[1].Stack != 1010 {
		t.Fatalf("street %s, stack %d", table.Street(), seats[1].Stack)
	}

	table.At(holdemStart.Add(time.Minute), deckOf(t, ""))
	if table.Players() != 2 {
		t.Fatalf("players %d", table.Players())
	}
}

func TestHoldem_Sit(t *testing.T) {
	t.Parallel()

	owner, guest, friend := uuid.New(), uuid.New(), uuid.New()
	table, err := user.NewHoldemTable(uuid.New(), owner, "Свои", true, 2, 10, holdemStart)
	if err != nil {
		t.Fatal(err)
	}

	if err = table.Sit(owner, -1, 199, holdemStart); !errors.Is(err, user.ErrTableBuyIn) {
		t.Fatalf("a short buy-in: %v", err)
	}
	if err = table.Sit(owner, -1, 200, holdemStart); err != nil {
		t.Fatal(err)
	}
	if err = table.Sit(owner, -1, 200, holdemStart); !errors.Is(err, user.ErrTableSeated) {
		t.Fatalf("twice: %v", err)
	}
	if err = table.Sit(guest, -1, 200, holdemStart); !errors.Is(err, user.ErrTableNotInvited) {
		t.Fatalf("not invited: %v", err)
	}
	if err = table.Invite(guest, []uuid.UUID{friend}); !errors.Is(err, user.ErrTableNotSeated) {
		t.Fatalf("invited by a stranger: %v", err)
	}
	if err = table.Invite(owner, []uuid.UUID{friend}); err != nil {
		t.Fatal(err)
	}
	if err = table.Sit(friend, 0, 200, holdemStart); !errors.Is(err, user.ErrTableSeatTaken) {
		t.Fatalf("a taken seat: %v", err)
	}
	if err = table.Sit(friend, -1, 200, holdemStart); err != nil {
		t.Fatal(err)
	}
	if table.Deadline().IsZero() {
		t.Fatal("two players wait for no hand")
	}

	for _, bad := range []struct {
		seats int
		blind int64
		name  string
	}{{1, 10, ""}, {10, 10, ""}, {6, 1, ""}, {6, 10, "a\nb"}, {6, 10, strings.Repeat("я", 41)}} {
		if _, err = user.NewHoldemTable(uuid.New(), owner, bad.name, false, bad.seats, bad.blind, holdemStart); err == nil {
			t.Fatalf("%+v is a table", bad)
		}
	}
}
