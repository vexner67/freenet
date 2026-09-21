package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/auth/internal/errs"
)

func New(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, errs.Wrap("pgxpool.New", err)
	}

	if err = pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, errs.Wrap("pool.Ping", err)
	}

	return pool, nil
}
