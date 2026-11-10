package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
)

type BanRepository struct {
	pool        *pgxpool.Pool
	readTimeout time.Duration
}

func NewBanRepository(pool *pgxpool.Pool, readTimeout time.Duration) *BanRepository {
	return &BanRepository{pool: pool, readTimeout: readTimeout}
}

const getBanQuery = `SELECT until FROM room_bans WHERE room_id=$1 AND user_id=$2`

func (r *BanRepository) Until(ctx context.Context, id, user uuid.UUID) (time.Time, error) {
	ctx, cancel := context.WithTimeout(ctx, r.readTimeout)
	defer cancel()
	var until time.Time
	err := pgpool.Conn(ctx, r.pool).QueryRow(ctx, getBanQuery, id, user).Scan(&until)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, errs.Wrap("pgpool.Conn.QueryRow.Scan", err)
	}
	return until, nil
}

const saveBanQuery = `
 INSERT INTO room_bans (
  room_id,
  user_id,
  until
 ) VALUES ($1,$2,$3)
 ON CONFLICT (room_id,user_id) DO UPDATE SET until=EXCLUDED.until
`

func (r *BanRepository) SaveBan(ctx context.Context, id, user uuid.UUID, until time.Time) error {
	_, err := pgpool.Conn(ctx, r.pool).Exec(ctx, saveBanQuery, id, user, until)
	return errs.Wrap("pgpool.Conn.Exec", err)
}

const deleteUserBansQuery = `DELETE FROM room_bans WHERE user_id=$1`

func (r *BanRepository) DeleteUser(ctx context.Context, id uuid.UUID) error {
	_, err := pgpool.Conn(ctx, r.pool).Exec(ctx, deleteUserBansQuery, id)
	return errs.Wrap("pgpool.Conn.Exec", err)
}
