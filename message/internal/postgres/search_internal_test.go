package postgres

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/vexner67/freenet/pkg/pgpool"
)

// The search is served by the trigram index only while the query and the
// migration name the same expression: ask the planner.
func TestSearch_Indexed(t *testing.T) {
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

	for _, tt := range []struct {
		index, query string
		args         []any
	}{
		{"messages_search_idx", "SELECT id FROM messages WHERE " + messageWords + " ILIKE $1", []any{"%привет%"}},
		{"chat_messages_search_idx", "SELECT msg.id FROM chat_messages msg WHERE " + chatMessageWords + " ILIKE $1", []any{"%привет%"}},
	} {
		rows, err := tx.Query(ctx, "EXPLAIN "+tt.query, tt.args...)
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

		if !strings.Contains(plan.String(), tt.index) {
			t.Errorf("the search does not use %s:\n%s", tt.index, plan.String())
		}
	}
}
