package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
)

// Serializing membership and metadata changes also protects room quotas and
// keeps a concurrent join from observing an uncommitted ban. The bounded
// rooms workload does not need per-owner lock ordering.
type TxManager struct {
	pool *pgxpool.Pool
	tx   *pgpool.TxManager
}

func NewTxManager(pool *pgxpool.Pool) *TxManager {
	return &TxManager{pool: pool, tx: pgpool.NewTxManager(pool)}
}

const lockRoomsQuery = `SELECT pg_advisory_xact_lock(7059)`

func (m *TxManager) WithinTx(ctx context.Context, fn func(context.Context) error) error {
	return errs.Wrap("m.tx.WithinTx", m.tx.WithinTx(ctx, func(ctx context.Context) error {
		if _, err := pgpool.Conn(ctx, m.pool).Exec(ctx, lockRoomsQuery); err != nil {
			return errs.Wrap("pgpool.Conn.Exec", err)
		}
		return fn(ctx)
	}))
}
