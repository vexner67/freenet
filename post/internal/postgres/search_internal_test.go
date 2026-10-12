package postgres

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/vexner67/freenet/pkg/pgpool"
)

// The search of a wall is served by the trigram index: ask the planner.
func TestWallSearch_Indexed(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgpool.New: %v", err)
	}
	defer pool.Close()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// A small table is read whole whatever the indexes: ask what the
	// planner can do without that.
	if _, err = tx.Exec(ctx, "SET LOCAL enable_seqscan = off"); err != nil {
		t.Fatal(err)
	}

	rows, err := tx.Query(ctx, "EXPLAIN SELECT id FROM posts WHERE text ILIKE $1", "%привет%")
	if err != nil {
		t.Fatal(err)
	}

	var plan strings.Builder
	for rows.Next() {
		var line string
		if err = rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(line + "\n")
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(plan.String(), "posts_wall_search_idx") {
		t.Fatalf("the search does not use posts_wall_search_idx:\n%s", plan.String())
	}
}
