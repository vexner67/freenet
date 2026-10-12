package service_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/club/internal/club"
	"github.com/vexner67/freenet/club/internal/inmem"
	"github.com/vexner67/freenet/club/internal/service"
)

// reminded remembers who was reminded of which event, in order.
type reminded struct {
	told [][2]uuid.UUID
	err  error
}

func (r *reminded) Reminded(_ context.Context, clubID, userID uuid.UUID) error {
	if r.err != nil {
		return r.err
	}

	r.told = append(r.told, [2]uuid.UUID{clubID, userID})

	return nil
}

func TestReminderService_RemindDue(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := inmem.NewStore()
	svc := service.NewClubService(service.ClubDeps{
		Clubs: store.Clubs(), Members: store.Members(), Tx: inmem.TxManager{}, Notifier: &notifier{appointed: make(map[[2]uuid.UUID]bool)}, Readers: &notifier{},
	})
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()

	now := time.Now()
	clock := now
	told := &reminded{}
	reminder := service.NewReminderService(store.Reminders(), inmem.TxManager{}, told, 24*time.Hour, func() time.Time { return clock })

	event := func(name club.Name, start time.Time) *club.Club {
		t.Helper()

		schedule, err := club.NewSchedule(start, time.Time{})
		if err != nil {
			t.Fatal(err)
		}

		v, err := svc.CreateEvent(ctx, ann, name, "", schedule)
		if err != nil {
			t.Fatal(err)
		}

		return v.Club
	}

	create(t, svc, ann) // a group is never reminded of
	later := event("Через неделю", now.Add(7*24*time.Hour))
	soon := event("Завтра", now.Add(20*time.Hour))

	if _, err := svc.Join(ctx, bob, soon.ID(), true); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Join(ctx, cat, later.ID(), false); err != nil {
		t.Fatal(err)
	}

	// A failed round reminds of nothing and leaves nothing behind... in
	// PostgreSQL; in memory the mark stays, so the failure comes first
	// with nothing due.
	told.err = errors.New("outbox is down")
	clock = now.Add(-30 * 24 * time.Hour)
	if n, err := reminder.RemindDue(ctx); err != nil || n != 0 {
		t.Fatalf("with nothing due = %d, %v", n, err)
	}
	told.err = nil
	clock = now

	// Only the event that starts within a day, to everyone who comes or
	// might, the newest member first.
	n, err := reminder.RemindDue(ctx)
	if err != nil || n != 1 {
		t.Fatalf("RemindDue = %d, %v; want 1", n, err)
	}
	if want := [][2]uuid.UUID{{soon.ID(), bob}, {soon.ID(), ann}}; !slices.Equal(told.told, want) {
		t.Fatalf("reminded = %v, want %v", told.told, want)
	}

	// Once.
	if n, err = reminder.RemindDue(ctx); err != nil || n != 0 || len(told.told) != 2 {
		t.Fatalf("again = %d, %v, reminded %d", n, err, len(told.told))
	}

	// Moved to another day, it is reminded of again when that day is near.
	moved, _ := club.NewSchedule(now.Add(3*24*time.Hour), time.Time{})
	if _, err = svc.Update(ctx, ann, soon.ID(), club.Changes{Name: "Завтра", Wall: club.WallOpen, AdminsDisplay: club.AdminsDisplayCreator, Schedule: moved}); err != nil {
		t.Fatal(err)
	}
	if n, err = reminder.RemindDue(ctx); err != nil || n != 0 {
		t.Fatalf("moved away = %d, %v", n, err)
	}

	clock = now.Add(2*24*time.Hour + time.Hour)
	if n, err = reminder.RemindDue(ctx); err != nil || n != 1 || len(told.told) != 4 {
		t.Fatalf("moved and near = %d, %v, reminded %d", n, err, len(told.told))
	}

	// An event that has started is not reminded of.
	clock = now.Add(8 * 24 * time.Hour)
	if n, err = reminder.RemindDue(ctx); err != nil || n != 0 {
		t.Fatalf("after the start = %d, %v", n, err)
	}

	// A failure to record a reminder fails the round.
	clock = now.Add(6*24*time.Hour + time.Hour)
	told.err = errors.New("outbox is down")
	if _, err = reminder.RemindDue(ctx); err == nil {
		t.Fatal("a failed reminder went unnoticed")
	}
}
