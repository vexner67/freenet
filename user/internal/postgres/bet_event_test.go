package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/user/internal/postgres"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestBetEventRepository(t *testing.T) {
	repo, pool := newRepo(t)
	wallets := postgres.NewWalletRepository(pool)
	repoEvents := postgres.NewBetEventRepository(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	newUser := func(votes int64) uuid.UUID {
		t.Helper()

		u, err := user.New(user.Email(unique("bets") + "@example.com"))
		if err != nil {
			t.Fatal(err)
		}
		if err = repo.Save(ctx, u); err != nil {
			t.Fatal(err)
		}
		if _, err = wallets.Credit(ctx, u.ID(), votes, 0, user.Entry(user.VoteVoucher, uuid.Nil, "")); err != nil {
			t.Fatal(err)
		}

		return u.ID()
	}

	adminEmail := unique("betadmin") + "@example.com"
	admin, err := user.New(user.Email(adminEmail))
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.Save(ctx, admin); err != nil {
		t.Fatal(err)
	}
	admins, err := user.NewAdmins([]string{adminEmail})
	if err != nil {
		t.Fatal(err)
	}

	events := service.NewBetEventService(repo, repoEvents, wallets, pgpool.NewTxManager(pool), admins)

	q, err := user.NewBetQuestion("Проиграют ли республиканцы?")
	if err != nil {
		t.Fatal(err)
	}
	yes, _ := user.NewBetOutcome("Да")
	no, _ := user.NewBetOutcome("Нет")
	maybe, _ := user.NewBetOutcome("Ничья")

	quiet, err := events.Create(ctx, admin.ID(), q, []user.BetOutcome{yes, no}, []int{190, 190}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	e, err := events.Create(ctx, admin.ID(), q, []user.BetOutcome{yes, no, maybe}, []int{150, 300, 500}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	// An event made before the odds keeps none.
	pooled := user.RestoreBetEvent(uuid.Must(uuid.NewV7()), q, []user.BetOutcome{yes, no}, nil, user.BetEventOpen, time.Time{}, -1)
	if err := repoEvents.Create(ctx, pooled); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, id := range []uuid.UUID{quiet.ID(), e.ID(), pooled.ID()} {
			if err := repoEvents.Delete(context.Background(), id); err != nil && !errors.Is(err, user.ErrBetEventNotFound) {
				t.Error(err)
			}
		}
	})

	ann, bob, eve := newUser(100), newUser(100), newUser(100)

	// Many stakes at once all count: they share the row of the event.
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			if _, err := events.Stake(ctx, ann, e.ID(), 0, 3); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()

	if _, err := events.Stake(ctx, bob, e.ID(), 0, 70); err != nil {
		t.Fatal(err)
	}
	if _, err := events.Stake(ctx, eve, e.ID(), 1, 100); err != nil {
		t.Fatal(err)
	}
	if _, err := events.Stake(ctx, eve, e.ID(), 1, 1); !errors.Is(err, user.ErrNotEnoughVotes) {
		t.Fatalf("Stake above the balance: %v", err)
	}
	if _, err := events.Stake(ctx, ann, quiet.ID(), 1, 5); err != nil {
		t.Fatal(err)
	}

	find := func(views []service.BetEventView, id uuid.UUID) (service.BetEventView, int) {
		for i, v := range views {
			if v.Event.ID() == id {
				return v, i
			}
		}
		t.Fatalf("no event %s in %+v", id, views)

		return service.BetEventView{}, 0
	}

	views, err := events.Events(ctx, ann, service.BetEventsAll)
	if err != nil {
		t.Fatal(err)
	}
	v, _ := find(views, e.ID())
	if v.Staked[0] != 100 || v.Staked[1] != 100 || v.Staked[2] != 0 || v.Bettors[0] != 2 || v.People != 3 ||
		v.Mine[0] != 30 || v.Mine[1] != 0 || !v.Event.ClosesAt().After(time.Now()) || v.Event.Outcomes()[2] != maybe ||
		v.Potential[0] != 40 || v.Potential[1] != 0 || v.Event.Odds()[2] != 500 {
		t.Fatalf("view = %+v", v)
	}
	if v, _ := find(views, pooled.ID()); !v.Event.Pooled() {
		t.Fatalf("pooled view = %+v", v)
	}

	// New odds are kept; the stakes made keep theirs.
	if err := events.SetOdds(ctx, admin.ID(), quiet.ID(), []int{120, 400}); err != nil {
		t.Fatal(err)
	}
	if err := events.SetOdds(ctx, admin.ID(), pooled.ID(), []int{120, 400}); !errors.Is(err, user.ErrEventPooled) {
		t.Fatalf("SetOdds of a pooled event: %v", err)
	}

	views, err = events.Events(ctx, ann, service.BetEventsAll)
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := find(views, quiet.ID()); v.Event.Odds()[1] != 400 || v.Potential[1] != 9 {
		t.Fatalf("view after SetOdds = %+v", v)
	}

	popular, err := events.Events(ctx, uuid.Nil, service.BetEventsPopular)
	if err != nil {
		t.Fatal(err)
	}
	_, busy := find(popular, e.ID())
	if _, calm := find(popular, quiet.ID()); calm < busy {
		t.Fatal("the quiet event is more popular")
	}

	if err := events.Resolve(ctx, admin.ID(), e.ID(), 0); err != nil {
		t.Fatal(err)
	}
	// Each stake of 3 on ×1.5 pays 4, rounded down.
	for id, want := range map[uuid.UUID]int64{ann: 65 + 40, bob: 30 + 105, eve: 0} {
		u, err := repo.GetByID(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if u.Wallet().Votes != want {
			t.Fatalf("votes = %d; want %d", u.Wallet().Votes, want)
		}
	}

	views, err = events.Events(ctx, ann, service.BetEventsAll)
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := find(views, e.ID()); v.Payout != 40 || v.Event.State() != user.BetEventResolved || v.Event.Winner() != 0 {
		t.Fatalf("resolved view = %+v", v)
	}

	if err := events.Cancel(ctx, admin.ID(), quiet.ID()); err != nil {
		t.Fatal(err)
	}
	if err := events.Delete(ctx, admin.ID(), quiet.ID()); err != nil {
		t.Fatal(err)
	}
	if err := events.Delete(ctx, admin.ID(), quiet.ID()); !errors.Is(err, user.ErrBetEventNotFound) {
		t.Fatalf("second Delete: %v", err)
	}
	u, err := repo.GetByID(ctx, ann)
	if err != nil || u.Wallet().Votes != 110 {
		t.Fatalf("ann after Cancel = %+v, %v", u.Wallet(), err)
	}
}
