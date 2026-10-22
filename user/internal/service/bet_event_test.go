package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/inmem"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

func newBetEvents(t *testing.T, w walletWorld, now func() time.Time) *service.BetEventService {
	t.Helper()

	events, _ := newBetEventsRepo(t, w, now)

	return events
}

// newBetEventsRepo is newBetEvents with its repository, to keep events the
// service no longer makes.
func newBetEventsRepo(t *testing.T, w walletWorld, now func() time.Time) (*service.BetEventService, *inmem.BetEventRepository) {
	t.Helper()

	admins, err := user.NewAdmins([]string{"admin@example.com"})
	if err != nil {
		t.Fatal(err)
	}

	repo := inmem.NewBetEventRepository().WithNow(now)

	return service.NewBetEventService(w.users, repo, w.users, inmem.TxManager{}, admins).WithNow(now), repo
}

// evens are the odds ×1.9 of each of the outcomes: ×2 would pay out all
// that is staked.
func evens(outcomes []user.BetOutcome) []int {
	odds := make([]int, len(outcomes))
	for i := range odds {
		odds[i] = 190
	}

	return odds
}

func betQuestion(t *testing.T) (user.BetQuestion, []user.BetOutcome) {
	t.Helper()

	q, err := user.NewBetQuestion("Проиграют ли республиканцы выборы?")
	if err != nil {
		t.Fatal(err)
	}

	yes, _ := user.NewBetOutcome("Да")
	no, _ := user.NewBetOutcome("Нет")

	return q, []user.BetOutcome{yes, no}
}

func TestBetEventService_Resolve(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	w := newWalletWorld(t)
	admin, ann, bob, eve := w.person(t, "admin"), w.person(t, "ann"), w.person(t, "bob"), w.person(t, "eve")
	events := newBetEvents(t, w, func() time.Time { return now })

	q, outcomes := betQuestion(t)
	if _, err := events.Create(ctx, ann, q, outcomes, []int{150, 250}, time.Time{}); !errors.Is(err, user.ErrNotAdmin) {
		t.Fatalf("Create by ann: %v", err)
	}
	if _, err := events.Create(ctx, admin, q, outcomes, []int{150}, time.Time{}); !errors.Is(err, user.ErrEventOddsCount) {
		t.Fatalf("Create without all the odds: %v", err)
	}
	e, err := events.Create(ctx, admin, q, outcomes, []int{150, 250}, now.Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}

	for _, id := range []uuid.UUID{ann, bob, eve} {
		if _, err := w.users.Credit(ctx, id, 100, 0, user.VoteEntry{}); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := events.Stake(ctx, ann, e.ID(), 0, 101); !errors.Is(err, user.ErrNotEnoughVotes) {
		t.Fatalf("Stake above the balance: %v", err)
	}
	if _, err := events.Stake(ctx, ann, uuid.New(), 0, 1); !errors.Is(err, user.ErrBetEventNotFound) {
		t.Fatalf("Stake on nothing: %v", err)
	}
	if _, err := events.Stake(ctx, ann, e.ID(), 5, 1); !errors.Is(err, user.ErrBetOutcome) {
		t.Fatalf("Stake on outcome 5: %v", err)
	}

	stakes := []struct {
		who     uuid.UUID
		outcome int
		amount  int64
	}{{ann, 0, 10}, {ann, 0, 20}, {bob, 0, 70}, {eve, 1, 100}}
	for _, s := range stakes {
		if _, err := events.Stake(ctx, s.who, e.ID(), s.outcome, s.amount); err != nil {
			t.Fatal(err)
		}
	}
	if got := w.votes(t, ann); got != 70 {
		t.Fatalf("ann has %d; want 70", got)
	}

	views, err := events.Events(ctx, ann, service.BetEventsAll)
	if err != nil || len(views) != 1 {
		t.Fatalf("Events = %+v, %v", views, err)
	}
	v := views[0]
	if v.Staked[0] != 100 || v.Staked[1] != 100 || v.Bettors[0] != 2 || v.People != 3 || v.Mine[0] != 30 || v.Mine[1] != 0 ||
		v.Potential[0] != 45 || v.Potential[1] != 0 {
		t.Fatalf("view = %+v", v)
	}

	if err := events.Resolve(ctx, ann, e.ID(), 0); !errors.Is(err, user.ErrNotAdmin) {
		t.Fatalf("Resolve by ann: %v", err)
	}
	if err := events.Resolve(ctx, admin, e.ID(), 0); err != nil {
		t.Fatal(err)
	}

	// Everyone on the winner gets their stakes times ×1.5, however many
	// of them there are.
	for id, want := range map[uuid.UUID]int64{ann: 115, bob: 135, eve: 0} {
		if got := w.votes(t, id); got != want {
			t.Fatalf("votes = %d; want %d", got, want)
		}
	}

	if err := events.Resolve(ctx, admin, e.ID(), 1); !errors.Is(err, user.ErrBetEventFinished) {
		t.Fatalf("second Resolve: %v", err)
	}
	if _, err := events.Stake(ctx, eve, e.ID(), 1, 1); !errors.Is(err, user.ErrBetEventClosed) {
		t.Fatalf("Stake after Resolve: %v", err)
	}

	views, err = events.Events(ctx, ann, service.BetEventsAll)
	if err != nil || views[0].Payout != 45 || views[0].Event.Winner() != 0 {
		t.Fatalf("Events after Resolve = %+v, %v", views, err)
	}

	if err := events.Delete(ctx, admin, e.ID()); err != nil {
		t.Fatal(err)
	}
	if views, err := events.Events(ctx, ann, service.BetEventsAll); err != nil || len(views) != 0 {
		t.Fatalf("Events after Delete = %+v, %v", views, err)
	}
}

func TestBetEventService_Cancel(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	w := newWalletWorld(t)
	admin, ann := w.person(t, "admin"), w.person(t, "ann")
	events := newBetEvents(t, w, func() time.Time { return now })

	q, outcomes := betQuestion(t)
	e, err := events.Create(ctx, admin, q, outcomes, evens(outcomes), time.Time{})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := w.users.Credit(ctx, ann, 50, 0, user.VoteEntry{}); err != nil {
		t.Fatal(err)
	}
	if left, err := events.Stake(ctx, ann, e.ID(), 1, 40); err != nil || left != 10 {
		t.Fatalf("Stake = %d, %v", left, err)
	}

	if err := events.Delete(ctx, admin, e.ID()); !errors.Is(err, user.ErrBetEventActive) {
		t.Fatalf("Delete of an open event: %v", err)
	}

	if err := events.Close(ctx, admin, e.ID()); err != nil {
		t.Fatal(err)
	}
	if _, err := events.Stake(ctx, ann, e.ID(), 1, 1); !errors.Is(err, user.ErrBetEventClosed) {
		t.Fatalf("Stake after Close: %v", err)
	}
	if views, err := events.Events(ctx, ann, service.BetEventsPopular); err != nil || len(views) != 0 {
		t.Fatalf("popular after Close = %+v, %v", views, err)
	}

	if err := events.Cancel(ctx, admin, e.ID()); err != nil {
		t.Fatal(err)
	}
	if got := w.votes(t, ann); got != 50 {
		t.Fatalf("votes after Cancel = %d; want 50", got)
	}
}

func TestBetEventService_Popular(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	w := newWalletWorld(t)
	admin, ann := w.person(t, "admin"), w.person(t, "ann")
	events := newBetEvents(t, w, func() time.Time { return now })

	q, outcomes := betQuestion(t)
	quiet, err := events.Create(ctx, admin, q, outcomes, evens(outcomes), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	busy, err := events.Create(ctx, admin, q, outcomes, evens(outcomes), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := events.Create(ctx, admin, q, outcomes, evens(outcomes), now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}

	if _, err := w.users.Credit(ctx, ann, 100, 0, user.VoteEntry{}); err != nil {
		t.Fatal(err)
	}
	if _, err := events.Stake(ctx, ann, quiet.ID(), 0, 10); err != nil {
		t.Fatal(err)
	}
	if _, err := events.Stake(ctx, ann, busy.ID(), 1, 50); err != nil {
		t.Fatal(err)
	}

	// An hour later the third takes no stakes: popular are the open ones,
	// the most staked on first.
	now = now.Add(time.Hour)
	views, err := events.Events(ctx, uuid.Nil, service.BetEventsPopular)
	if err != nil || len(views) != 2 || views[0].Event.ID() != busy.ID() || views[1].Event.ID() != quiet.ID() {
		t.Fatalf("popular = %+v, %v", views, err)
	}
	if views[0].Mine[1] != 0 {
		t.Fatalf("a guest sees stakes of their own: %+v", views[0])
	}
}

func TestBetEventService_SetOdds(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	w := newWalletWorld(t)
	admin, ann := w.person(t, "admin"), w.person(t, "ann")
	events := newBetEvents(t, w, func() time.Time { return now })

	q, outcomes := betQuestion(t)
	e, err := events.Create(ctx, admin, q, outcomes, evens(outcomes), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.users.Credit(ctx, ann, 100, 0, user.VoteEntry{}); err != nil {
		t.Fatal(err)
	}

	if _, err := events.Stake(ctx, ann, e.ID(), 0, 10); err != nil {
		t.Fatal(err)
	}
	if err := events.SetOdds(ctx, ann, e.ID(), []int{300, 150}); !errors.Is(err, user.ErrNotAdmin) {
		t.Fatalf("SetOdds by ann: %v", err)
	}
	if err := events.SetOdds(ctx, admin, e.ID(), []int{300}); !errors.Is(err, user.ErrEventOddsCount) {
		t.Fatalf("SetOdds short: %v", err)
	}
	if err := events.SetOdds(ctx, admin, e.ID(), []int{300, 130}); err != nil {
		t.Fatal(err)
	}
	// The stake made before keeps its ×1.9, the new one takes ×3.
	if _, err := events.Stake(ctx, ann, e.ID(), 0, 10); err != nil {
		t.Fatal(err)
	}

	views, err := events.Events(ctx, ann, service.BetEventsAll)
	if err != nil || views[0].Potential[0] != 49 || views[0].Event.Odds()[0] != 300 {
		t.Fatalf("Events after SetOdds = %+v, %v", views, err)
	}

	if err := events.Resolve(ctx, admin, e.ID(), 0); err != nil {
		t.Fatal(err)
	}
	if got := w.votes(t, ann); got != 129 {
		t.Fatalf("votes = %d; want 129", got)
	}
	if err := events.SetOdds(ctx, admin, e.ID(), []int{300, 130}); !errors.Is(err, user.ErrBetEventFinished) {
		t.Fatalf("SetOdds after Resolve: %v", err)
	}
}

// An event made before the odds shares the stakes among those who guessed.
func TestBetEventService_Pooled(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	w := newWalletWorld(t)
	admin, ann, bob := w.person(t, "admin"), w.person(t, "ann"), w.person(t, "bob")
	events, repo := newBetEventsRepo(t, w, func() time.Time { return now })

	q, outcomes := betQuestion(t)
	e := user.RestoreBetEvent(uuid.Must(uuid.NewV7()), q, outcomes, nil, user.BetEventOpen, time.Time{}, -1)
	if err := repo.Create(ctx, e); err != nil {
		t.Fatal(err)
	}

	for _, id := range []uuid.UUID{ann, bob} {
		if _, err := w.users.Credit(ctx, id, 100, 0, user.VoteEntry{}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := events.Stake(ctx, ann, e.ID(), 0, 30); err != nil {
		t.Fatal(err)
	}
	if _, err := events.Stake(ctx, bob, e.ID(), 1, 70); err != nil {
		t.Fatal(err)
	}
	if err := events.SetOdds(ctx, admin, e.ID(), []int{200, 200}); !errors.Is(err, user.ErrEventPooled) {
		t.Fatalf("SetOdds of a pooled event: %v", err)
	}

	if err := events.Resolve(ctx, admin, e.ID(), 0); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[uuid.UUID]int64{ann: 170, bob: 30} {
		if got := w.votes(t, id); got != want {
			t.Fatalf("votes = %d; want %d", got, want)
		}
	}
}
