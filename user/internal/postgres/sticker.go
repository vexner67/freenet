package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/user/internal/user"
)

// StickerRepository keeps the packs of stickers users have taken in
// sticker_packs.
type StickerRepository struct {
	pool *pgxpool.Pool
}

func NewStickerRepository(pool *pgxpool.Pool) *StickerRepository {
	return &StickerRepository{
		pool: pool,
	}
}

// conn is the transaction of ctx (service.TxManager), or the pool outside
// one.
func (r *StickerRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

// A pack taken already is only added back; xmax is 0 for a row the
// statement inserted. Two adds at once: the second waits for the row of
// the first and finds it there.
const addStickerPackQuery = `
	INSERT INTO sticker_packs (
		user_id,
		pack
	)
	VALUES ($1, $2)
	ON CONFLICT (user_id, pack) DO UPDATE
	SET added = true
	RETURNING xmax = 0
`

func (r *StickerRepository) Add(ctx context.Context, userID uuid.UUID, pack user.StickerPackID) (bool, error) {
	var taken bool

	err := r.conn(ctx).QueryRow(ctx, addStickerPackQuery, userID, pack.String()).Scan(&taken)

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolationCode {
		return false, user.ErrNotFound
	}
	if err != nil {
		return false, errs.Wrap("r.conn.QueryRow", err)
	}

	return taken, nil
}

const removeStickerPackQuery = `
	UPDATE sticker_packs
	SET added = false
	WHERE user_id = $1
		AND pack = $2
`

func (r *StickerRepository) Remove(ctx context.Context, userID uuid.UUID, pack user.StickerPackID) error {
	if _, err := r.conn(ctx).Exec(ctx, removeStickerPackQuery, userID, pack.String()); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

const listStickerPacksQuery = `
	SELECT
		pack,
		added
	FROM sticker_packs
	WHERE user_id = $1
`

func (r *StickerRepository) List(ctx context.Context, userID uuid.UUID) ([]user.OwnedPack, error) {
	rows, err := r.conn(ctx).Query(ctx, listStickerPacksQuery, userID)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}
	defer rows.Close()

	var owned []user.OwnedPack

	for rows.Next() {
		var (
			pack  string
			added bool
		)

		if err = rows.Scan(&pack, &added); err != nil {
			return nil, errs.Wrap("rows.Scan", err)
		}

		// Data comes from our own table and was validated on write.
		owned = append(owned, user.OwnedPack{Pack: user.StickerPackID(pack), Added: added})
	}

	if err = rows.Err(); err != nil {
		return nil, errs.Wrap("rows.Err", err)
	}

	return owned, nil
}
