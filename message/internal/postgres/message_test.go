package postgres_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/message/internal/message"
	"github.com/vexner67/freenet/message/internal/postgres"
	"github.com/vexner67/freenet/pkg/pgpool"
)

// Integration test: needs a database with migrations applied, e.g.
// TEST_DATABASE_URL=postgres://... go test ./internal/postgres
func TestRepositories(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	messages := postgres.NewMessageRepository(pool)
	dialogs := postgres.NewDialogRepository(pool)
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()

	save := func(from, to uuid.UUID, text string) *message.Message {
		t.Helper()
		m, err := message.New(from, to, message.Text(text), time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err = messages.Save(ctx, m); err != nil {
			t.Fatalf("Save: %v", err)
		}
		return m
	}

	m1 := save(bob, ann, "раз")
	m2 := save(ann, bob, "два")
	m3 := save(bob, ann, "три")
	toCat := save(ann, cat, "кату")

	t.Run("who has written to whom", func(t *testing.T) {
		for _, tt := range []struct {
			user, peer uuid.UUID
			want       bool
		}{
			{ann, bob, true},  // bob wrote to ann
			{cat, ann, true},  // ann wrote to cat
			{ann, cat, false}, // cat never answered
			{bob, cat, false}, // no dialog at all
		} {
			if got, err := dialogs.HasIncoming(ctx, tt.user, tt.peer); err != nil || got != tt.want {
				t.Fatalf("HasIncoming(%v, %v) = %v, %v; want %v", tt.user, tt.peer, got, err, tt.want)
			}
		}
	})

	t.Run("history in both directions", func(t *testing.T) {
		got, err := messages.Get(ctx, m2.ID())
		if err != nil || *got != *m2 {
			t.Fatalf("Get = %+v, %v; want %+v", got, err, m2)
		}

		newest, err := messages.ListBefore(ctx, ann, bob, uuid.Nil, 2)
		if err != nil || len(newest) != 2 || newest[0].ID() != m3.ID() || newest[1].ID() != m2.ID() {
			t.Fatalf("ListBefore = %v, %v", newest, err)
		}

		// The pair in the other order is the same dialog.
		older, err := messages.ListBefore(ctx, bob, ann, m2.ID(), 10)
		if err != nil || len(older) != 1 || older[0].ID() != m1.ID() {
			t.Fatalf("ListBefore after cursor = %v, %v", older, err)
		}

		after, err := messages.ListAfter(ctx, ann, bob, m1.ID(), 10)
		if err != nil || len(after) != 2 || after[0].ID() != m2.ID() || after[1].ID() != m3.ID() {
			t.Fatalf("ListAfter = %v, %v", after, err)
		}

		if _, err = messages.Get(ctx, uuid.New()); !errors.Is(err, message.ErrNotFound) {
			t.Fatalf("Get missing: %v", err)
		}
	})

	t.Run("dialogs and read marks", func(t *testing.T) {
		list, err := dialogs.List(ctx, ann, uuid.Nil, 10)
		if err != nil || len(list) != 2 {
			t.Fatalf("List = %v, %v", list, err)
		}
		if list[0].PeerID != cat || list[1].PeerID != bob || list[1].LastMessage.ID() != m3.ID() || list[1].LastIncomingID != m3.ID() {
			t.Fatalf("dialogs = %+v", list)
		}
		if !list[1].HasUnread() || list[0].HasUnread() {
			t.Fatal("bob's dialog is unread, cat's has nothing incoming")
		}

		if n, err := dialogs.CountUnread(ctx, ann, 10); err != nil || n != 1 {
			t.Fatalf("CountUnread = %d, %v", n, err)
		}

		if err = dialogs.MarkRead(ctx, ann, bob, m3.ID()); err != nil {
			t.Fatal(err)
		}
		if err = dialogs.MarkRead(ctx, ann, bob, m1.ID()); err != nil {
			t.Fatal(err)
		}

		marks, err := dialogs.ReadMarks(ctx, ann, bob)
		if err != nil || marks.Mine != m3.ID() || marks.Theirs != uuid.Nil {
			t.Fatalf("ReadMarks = %+v, %v; the newer mark must stay", marks, err)
		}

		theirs, err := dialogs.ReadMarks(ctx, bob, ann)
		if err != nil || theirs.Theirs != m3.ID() {
			t.Fatalf("bob's marks = %+v, %v", theirs, err)
		}

		if n, _ := dialogs.CountUnread(ctx, ann, 10); n != 0 {
			t.Fatalf("unread after reading = %d", n)
		}

		page, err := dialogs.List(ctx, ann, toCat.ID(), 10)
		if err != nil || len(page) != 1 || page[0].PeerID != bob {
			t.Fatalf("List after cursor = %v, %v", page, err)
		}
	})

	// Both sides' dialogs change in the statement that saves the message,
	// so concurrent messages leave them pointing at the newest.
	t.Run("concurrent messages", func(t *testing.T) {
		dan, eve := uuid.New(), uuid.New()

		var wg sync.WaitGroup
		for i := range 20 {
			from, to := dan, eve
			if i%2 == 0 {
				from, to = eve, dan
			}
			wg.Go(func() {
				m, err := message.New(from, to, "x", time.Now())
				if err == nil {
					err = messages.Save(ctx, m)
				}
				if err != nil {
					t.Error(err)
				}
			})
		}
		wg.Wait()

		newest, err := messages.ListBefore(ctx, dan, eve, uuid.Nil, 1)
		if err != nil || len(newest) != 1 {
			t.Fatal(err)
		}

		for _, user := range []uuid.UUID{dan, eve} {
			list, err := dialogs.List(ctx, user, uuid.Nil, 10)
			if err != nil || len(list) != 1 || list[0].LastMessage.ID() != newest[0].ID() {
				t.Fatalf("%v's dialog = %+v, %v; want the newest message", user, list, err)
			}
		}
	})
}
