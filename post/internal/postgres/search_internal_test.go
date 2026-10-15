package postgres

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/vexner67/freenet/pkg/pgpool"
)

// The searches of a wall and of all posts are served by their indexes:
// ask the planner.
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

	tests := []struct {
		name, query, arg, index string
	}{
		{name: "wall", query: "SELECT id FROM posts WHERE text ILIKE $1", arg: "%привет%", index: "posts_wall_search_idx"},
		{name: "text", query: countPostsByTextQuery, arg: "%привет%", index: "posts_wall_search_idx"},
		{name: "tag", query: countPostsByTagQuery, arg: "поход", index: "posts_hashtags_idx"},
	}

	for _, tt := range tests {
		args := []any{tt.arg}
		if tt.name != "wall" {
			args = []any{nil, []string{}, tt.arg}
		}

		if plan := explain(ctx, t, tx, tt.query, args...); !strings.Contains(plan, tt.index) {
			t.Errorf("%s: the search does not use %s:\n%s", tt.name, tt.index, plan)
		}
	}
}

func explain(ctx context.Context, t *testing.T, tx pgx.Tx, query string, args ...any) string {
	t.Helper()

	rows, err := tx.Query(ctx, "EXPLAIN "+query, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

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

	return plan.String()
}
