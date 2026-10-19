package postgres

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestSearchUsersQuery(t *testing.T) {
	t.Parallel()

	q, err := user.NewSearchQuery("ann lee", user.SearchFilters{City: "Москва", Film: "50%"})
	if err != nil {
		t.Fatal(err)
	}

	conds := searchConditions(q)
	sql := searchUsersQuery(conds)

	for _, want := range []string{
		"AND id < $1", searchHaystack + " LIKE $3", searchHaystack + " LIKE $4",
		"info_read = $5", "lower(city) = $6", "lower(fav_films) LIKE $7", "LIMIT $2",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("query lacks %q:\n%s", want, sql)
		}
	}
	if strings.Contains(sql, "$8") || strings.Contains(sql, "LIKE ALL") {
		t.Fatalf("one condition per word and filter:\n%s", sql)
	}
	if conds[2].param != "everyone" || conds[3].param != "москва" || conds[4].param != `%50\%%` {
		t.Fatalf("params = %+v", conds)
	}
	// The migration indexes exactly this expression.
	if searchHaystack != "lower(nickname || ' ' || name || ' ' || surname)" {
		t.Fatalf("haystack drifted from users_search_idx: %s", searchHaystack)
	}
}

// Every filter of the search is served by an index of its own: the
// expressions here and in the migration must not drift apart.
func TestSearchFilters_Indexed(t *testing.T) {
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

	tests := []struct {
		filters user.SearchFilters
		index   string
	}{
		{user.SearchFilters{Hometown: "Тверь"}, "users_hometown_idx"},
		{user.SearchFilters{City: "Москва"}, "users_city_idx"},
		{user.SearchFilters{Film: "Матрица"}, "users_fav_films_idx"},
		{user.SearchFilters{Show: "Друзья"}, "users_fav_shows_idx"},
		{user.SearchFilters{Book: "Идиот"}, "users_fav_books_idx"},
		{user.SearchFilters{Verified: true}, "users_verified_idx"},
	}

	for _, tt := range tests {
		t.Run(tt.index, func(t *testing.T) {
			q, err := user.NewSearchQuery("", tt.filters)
			if err != nil {
				t.Fatal(err)
			}

			// The last condition is the filter; before it, who tells.
			conds := searchConditions(q)
			filter := conds[len(conds)-1]

			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = tx.Rollback(ctx) }()

			// A small table is read whole whatever the indexes: ask what
			// the planner can do without that.
			if _, err = tx.Exec(ctx, "SET LOCAL enable_seqscan = off"); err != nil {
				t.Fatal(err)
			}

			rows, err := tx.Query(ctx, "EXPLAIN SELECT id FROM users WHERE "+fmt.Sprintf(filter.sql, 1), filter.param)
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
				t.Fatalf("the plan does not use %s:\n%s", tt.index, plan.String())
			}
		})
	}
}
