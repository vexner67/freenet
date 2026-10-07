package pgpool_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/pgpool"
)

// newPool connects to TEST_DATABASE_URL, an empty database; the test is
// skipped without it.
func newPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	pool, err := pgpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("pgpool.New: %v", err)
	}
	t.Cleanup(pool.Close)

	return pool
}

func TestTxManager(t *testing.T) {
	pool := newPool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS tx_test (n int NOT NULL)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DROP TABLE tx_test`) })

	insert := func(ctx context.Context, n int) error {
		_, err := pgpool.Conn(ctx, pool).Exec(ctx, `INSERT INTO tx_test (n) VALUES ($1)`, n)
		return err
	}
	count := func(n int) int {
		var c int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM tx_test WHERE n = $1`, n).Scan(&c); err != nil {
			t.Fatalf("count: %v", err)
		}
		return c
	}

	tm := pgpool.NewTxManager(pool)
	failure := errors.New("fail")

	// Committed, with a nested WithinTx that joins the transaction.
	err := tm.WithinTx(ctx, func(ctx context.Context) error {
		if err := insert(ctx, 1); err != nil {
			return err
		}
		return tm.WithinTx(ctx, func(ctx context.Context) error { return insert(ctx, 1) })
	})
	if err != nil {
		t.Fatalf("WithinTx: %v", err)
	}
	if got := count(1); got != 2 {
		t.Errorf("committed rows = %d, want 2", got)
	}

	// Rolled back: the insert before the failure is gone too, the nested
	// one included.
	err = tm.WithinTx(ctx, func(ctx context.Context) error {
		if err := tm.WithinTx(ctx, func(ctx context.Context) error { return insert(ctx, 2) }); err != nil {
			return err
		}
		return failure
	})
	if !errors.Is(err, failure) {
		t.Fatalf("WithinTx = %v, want the failure", err)
	}
	if got := count(2); got != 0 {
		t.Errorf("rolled back rows = %d, want 0", got)
	}

	// Outside a transaction Conn is the pool.
	if err = insert(ctx, 3); err != nil || count(3) != 1 {
		t.Errorf("insert outside a transaction: %v, rows %d", err, count(3))
	}
}
