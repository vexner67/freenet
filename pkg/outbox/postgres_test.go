package outbox_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/outbox"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/pkg/reqid"
)

// schema is the table of the Postgres doc comment; services create it in
// their migrations.
const schema = `
	CREATE TABLE outbox (
		id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
		stream text NOT NULL,
		kind text NOT NULL,
		payload bytea NOT NULL,
		request_id text NOT NULL DEFAULT '',
		attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
		due_at timestamptz NOT NULL DEFAULT now(),
		created_at timestamptz NOT NULL DEFAULT now()
	)
`

// newPool connects to TEST_DATABASE_URL, an empty database, and creates the
// outbox there; the test is skipped without it.
func newPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx := context.Background()
	pool, err := pgpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgpool.New: %v", err)
	}
	t.Cleanup(pool.Close)

	if _, err = pool.Exec(ctx, schema); err != nil {
		t.Fatalf("create outbox: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DROP TABLE outbox`) })

	return pool
}

func TestPostgres(t *testing.T) {
	pool := newPool(t)
	ctx := context.Background()
	tm := pgpool.NewTxManager(pool)

	first := outbox.NewPostgres(pool, "notification")
	second := outbox.NewPostgres(pool, "notification")
	t.Cleanup(first.Close)
	t.Cleanup(second.Close)

	// One relay leads: the first takes the lock and returns at once, the
	// second waits d and still sees nothing.
	if err := first.Wait(ctx, time.Second); err != nil {
		t.Fatalf("first Wait: %v", err)
	}

	// A message recorded in a transaction that rolls back is not there.
	rollback := errors.New("rollback")
	err := tm.WithinTx(ctx, func(ctx context.Context) error {
		if err := first.Add(ctx, "lost", []byte("x")); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("WithinTx = %v", err)
	}

	// The committed one wakes the leader up well before d.
	woke := make(chan error, 1)
	go func() { woke <- first.Wait(ctx, time.Minute) }()

	err = tm.WithinTx(reqid.ContextWithID(ctx, "req-7"), func(ctx context.Context) error {
		if err := first.Add(ctx, "like", []byte{1, 2}); err != nil {
			return err
		}
		return first.Add(ctx, "unlike", []byte{3})
	})
	if err != nil {
		t.Fatalf("WithinTx: %v", err)
	}

	select {
	case err = <-woke:
		if err != nil {
			t.Fatalf("Wait: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the leader was not woken by the commit")
	}

	if err = second.Wait(ctx, 100*time.Millisecond); err != nil {
		t.Fatalf("second Wait: %v", err)
	}
	if head, err := second.Head(ctx, 10); err != nil || len(head) != 0 {
		t.Fatalf("second Head = %d messages, %v; want none while the first leads", len(head), err)
	}

	head, err := first.Head(ctx, 10)
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if len(head) != 2 || head[0].Kind != "like" || head[1].Kind != "unlike" {
		t.Fatalf("Head = %+v, want like then unlike", head)
	}
	if string(head[0].Payload) != "\x01\x02" || head[0].RequestID != "req-7" || head[0].Attempts != 0 {
		t.Errorf("like = %+v, want its payload and request ID", head[0])
	}

	due := time.Now().Add(time.Hour).UTC().Truncate(time.Microsecond)
	if err = first.Postpone(ctx, head[0].ID, 3, due); err != nil {
		t.Fatalf("Postpone: %v", err)
	}
	if err = first.Delete(ctx, head[1].ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	head, err = first.Head(ctx, 10)
	if err != nil || len(head) != 1 || head[0].Attempts != 3 || !head[0].DueAt.Equal(due) {
		t.Fatalf("after Postpone and Delete: %+v, %v", head, err)
	}

	// Another stream of the table has its own order and its own relay: it
	// leads at once and sees only its own messages.
	media := outbox.NewPostgres(pool, "media")
	t.Cleanup(media.Close)
	if err = media.Add(ctx, "erase", []byte{9}); err != nil {
		t.Fatalf("Add to another stream: %v", err)
	}
	if err = media.Wait(ctx, time.Second); err != nil {
		t.Fatalf("Wait of another stream: %v", err)
	}
	if head, err := media.Head(ctx, 10); err != nil || len(head) != 1 || head[0].Kind != "erase" {
		t.Fatalf("Head of another stream = %+v, %v", head, err)
	}
	if head, err := first.Head(ctx, 10); err != nil || len(head) != 1 {
		t.Fatalf("the first stream sees %d messages, %v; want its own one", len(head), err)
	}

	// The lead passes on when the leader goes: its lock ends with its
	// session, which the server closes a moment after Close returns.
	first.Close()
	for range 100 {
		if err = second.Wait(ctx, 50*time.Millisecond); err != nil {
			t.Fatalf("second Wait: %v", err)
		}
		if head, err = second.Head(ctx, 10); err != nil || len(head) > 0 {
			break
		}
	}
	if err != nil || len(head) != 1 {
		t.Errorf("second Head after the first left = %d messages, %v; want 1", len(head), err)
	}
}
