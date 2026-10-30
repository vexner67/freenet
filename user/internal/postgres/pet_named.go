package postgres

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

const insertPetGrantQuery = `
	INSERT INTO pet_grants (
		user_id,
		kind,
		granted_at
	)
	VALUES ($1, $2, $3)
	ON CONFLICT (user_id, kind) DO NOTHING
`

// GrantPet runs in the transaction of its caller, so that a pet not added
// leaves no mark.
func (r *PetRepository) GrantPet(ctx context.Context, userID uuid.UUID, p *user.Pet, now time.Time) error {
	tag, err := r.conn(ctx).Exec(ctx, insertPetGrantQuery, userID, p.Kind().String(), now)

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolationCode {
		return user.ErrNotFound
	}
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}
	if tag.RowsAffected() == 0 {
		return user.ErrPetClaimed
	}

	_, err = r.conn(ctx).Exec(ctx, insertPetQuery, append(petArgs(userID, p), math.MaxInt32)...)

	return errs.Wrap("r.conn.Exec", err)
}

const selectPetGrantQuery = `
	SELECT true
	FROM pet_grants
	WHERE user_id = $1
		AND kind = $2
`

func (r *PetRepository) Granted(ctx context.Context, userID uuid.UUID, kind user.PetKind) (bool, error) {
	var granted bool
	err := r.conn(ctx).QueryRow(ctx, selectPetGrantQuery, userID, kind.String()).Scan(&granted)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}

	return granted, errs.Wrap("r.conn.QueryRow", err)
}
