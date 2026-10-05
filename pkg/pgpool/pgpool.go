// Package pgpool opens PostgreSQL connection pools and applies migrations.
package pgpool

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // database/sql driver "pgx" for goose
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

// New opens a pool and checks the database is reachable.
func New(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.New: %w", err)
	}

	if err = pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pool.Ping: %w", err)
	}

	return pool, nil
}

// Migrate applies pending goose migrations from fsys (SQL files at its
// root). A PostgreSQL advisory lock lets several replicas start at once:
// one migrates, the others wait.
func Migrate(ctx context.Context, dsn string, fsys fs.FS) (applied int, err error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return 0, fmt.Errorf("sql.Open: %w", err)
	}
	defer func() {
		if closeErr := db.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("db.Close: %w", closeErr)
		}
	}()

	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return 0, fmt.Errorf("lock.NewPostgresSessionLocker: %w", err)
	}

	provider, err := goose.NewProvider(goose.DialectPostgres, db, fsys, goose.WithSessionLocker(locker))
	if err != nil {
		return 0, fmt.Errorf("goose.NewProvider: %w", err)
	}

	results, err := provider.Up(ctx)
	if err != nil {
		return 0, fmt.Errorf("provider.Up: %w", err)
	}

	return len(results), nil
}
