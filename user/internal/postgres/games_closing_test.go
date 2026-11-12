package postgres_test

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/user/internal/postgres"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
	"github.com/vexner67/freenet/user/migrations"
)

func closingPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
	_, admin := newRepo(t)
	// A new database lets goose install its extensions in the same
	// namespace as the migration tables.
	name := "games_closing_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	ident := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+ident); err != nil { //nolint:gosec // quoted test database name
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), "DROP DATABASE "+ident); err != nil { //nolint:gosec // quoted test database name
			t.Error(err)
		}
	})
	dsn, err := url.Parse(os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	dsn.Path = "/" + name

	if _, err = pgpool.Migrate(ctx, dsn.String(), migrations.FS); err != nil {
		t.Fatal(err)
	}
	pool, err := pgpool.New(ctx, dsn.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestGamesClosePostgres(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool := closingPool(t, ctx)
	id := uuid.New()
	if _, err := pool.Exec(ctx, "INSERT INTO users (id, email, votes) VALUES ($1, 'closure@example.test', 15)", id); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO miners (user_id, energy, counted_at, backpack)
		VALUES ($1, 5, now(), '{"stone":150}')`, id); err != nil {
		t.Fatal(err)
	}
	event := uuid.New()
	if _, err := pool.Exec(ctx, "INSERT INTO bet_events (id, question, outcomes, state) VALUES ($1, 'Who wins?', ARRAY['A','B'], 'closed')", event); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO bet_stakes (event_id, user_id, outcome, amount) VALUES ($1, $2, 0, 11)", event, id); err != nil {
		t.Fatal(err)
	}

	store := postgres.NewGamesClosingRepository(pool)
	wallets := postgres.NewWalletRepository(pool)
	games := service.NewGamesService(store, store, wallets, pgpool.NewTxManager(pool))
	before, err := wallets.Treasury(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if report, err := games.Close(ctx, false, time.Now); err != nil || report.Total != 14 || len(report.Accounts) != 1 {
		t.Fatalf("preview = %+v, %v", report, err)
	}
	if err := store.WhileOpen(ctx, func(context.Context) error { return nil }); err != nil {
		t.Fatalf("preview must leave games open: %v", err)
	}
	// The first credit fits, but the second overflows. The entire wallet
	// transaction must roll back while the frozen plan remains resumable.
	if _, err := pool.Exec(ctx, "UPDATE users SET votes = 9223372036854775802 WHERE id = $1", id); err != nil {
		t.Fatal(err)
	}
	if _, err := games.Close(ctx, true, time.Now); err == nil {
		t.Fatal("overflowing account was paid")
	}
	var partial int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM vote_operations WHERE user_id = $1 AND kind = 'closing'", id).Scan(&partial); err != nil || partial != 0 {
		t.Fatalf("partial closing payments: %d, %v", partial, err)
	}
	if _, err := pool.Exec(ctx, "UPDATE users SET votes = 15 WHERE id = $1", id); err != nil {
		t.Fatal(err)
	}

	type outcome struct {
		report service.ClosingReport
		err    error
	}
	results := make(chan outcome, 2)
	for range cap(results) {
		go func() {
			report, err := games.Close(ctx, true, time.Now)
			results <- outcome{report, err}
		}()
	}
	for range cap(results) {
		got := <-results
		if got.err != nil || got.report.Total != 14 {
			t.Fatalf("run = %+v, %v", got.report, got.err)
		}
	}

	if err := store.WhileOpen(ctx, func(context.Context) error { t.Fatal("game request ran"); return nil }); !errors.Is(err, user.ErrGamesClosed) {
		t.Fatalf("closed games: %v", err)
	}

	var balance, credited, lines int64
	err = pool.QueryRow(ctx, `SELECT votes, sum(amount), count(*)
		FROM users JOIN vote_operations ON users.id = vote_operations.user_id
		WHERE users.id = $1 AND kind = 'closing' GROUP BY votes`, id).Scan(&balance, &credited, &lines)
	if err != nil || balance != 29 || credited != 14 || lines != 2 {
		t.Fatalf("balance %d, credited %d in %d lines: %v", balance, credited, lines, err)
	}
	if after, err := wallets.Treasury(ctx); err != nil || after.Votes != before.Votes-14 {
		t.Fatalf("treasury = %+v, %v; was %+v", after, err, before)
	}
}
