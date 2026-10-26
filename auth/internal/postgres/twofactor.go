package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/auth/internal/twofactor"
	"github.com/vexner67/freenet/pkg/errs"
)

// Sealer keeps the secrets of the apps encrypted in the database, bound to
// the page they are of.
type Sealer interface {
	Seal(plain []byte, label string) ([]byte, error)
	Open(sealed []byte, label string) ([]byte, error)
}

// TwoFactorRepository keeps the second steps of the pages; the codes are
// spent by conditional updates, so two requests cannot spend one twice.
type TwoFactorRepository struct {
	pool *pgxpool.Pool
	box  Sealer
}

func NewTwoFactorRepository(pool *pgxpool.Pool, box Sealer) *TwoFactorRepository {
	return &TwoFactorRepository{pool: pool, box: box}
}

const getTwoFactorQuery = `
	SELECT
		sealed_secret,
		enabled,
		last_step,
		cardinality(recovery_hashes)
	FROM two_factors
	WHERE user_id = $1
`

func (r *TwoFactorRepository) Get(ctx context.Context, userID uuid.UUID) (*twofactor.Factor, error) {
	var (
		sealed   []byte
		enabled  bool
		lastStep int64
		recovery int
	)

	err := r.pool.QueryRow(ctx, getTwoFactorQuery, userID).Scan(&sealed, &enabled, &lastStep, &recovery)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, twofactor.ErrNotSetUp
	}
	if err != nil {
		return nil, errs.Wrap("r.pool.QueryRow", err)
	}

	plain, err := r.box.Open(sealed, userID.String())
	if err != nil {
		return nil, errs.Wrap("r.box.Open", err)
	}

	secret, err := twofactor.RestoreSecret(plain)
	if err != nil {
		return nil, errs.Wrap("twofactor.RestoreSecret", err)
	}

	return twofactor.Restore(userID, secret, enabled, lastStep, recovery), nil
}

// setUpTwoFactorQuery replaces only a factor that is not enabled: an
// enabled one leaves no row changed.
const setUpTwoFactorQuery = `
	INSERT INTO two_factors (
		user_id,
		sealed_secret
	)
	VALUES ($1, $2)
	ON CONFLICT (user_id) DO UPDATE
	SET sealed_secret = EXCLUDED.sealed_secret,
		last_step = 0,
		recovery_hashes = '{}',
		updated_at = now()
	WHERE NOT two_factors.enabled
`

func (r *TwoFactorRepository) SetUp(ctx context.Context, f *twofactor.Factor) error {
	sealed, err := r.box.Seal(f.Secret(), f.UserID().String())
	if err != nil {
		return errs.Wrap("r.box.Seal", err)
	}

	tag, err := r.pool.Exec(ctx, setUpTwoFactorQuery, f.UserID(), sealed)
	if err != nil {
		return errs.Wrap("r.pool.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return twofactor.ErrEnabled
	}

	return nil
}

const enableTwoFactorQuery = `
	UPDATE two_factors
	SET enabled = true,
		last_step = $2,
		recovery_hashes = $3,
		updated_at = now()
	WHERE user_id = $1
		AND NOT enabled
`

func (r *TwoFactorRepository) Enable(ctx context.Context, userID uuid.UUID, step int64, recovery []string) error {
	tag, err := r.pool.Exec(ctx, enableTwoFactorQuery, userID, step, recovery)
	if err != nil {
		return errs.Wrap("r.pool.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return r.missing(ctx, userID)
	}

	return nil
}

const deleteTwoFactorQuery = `
	DELETE FROM two_factors
	WHERE user_id = $1
`

func (r *TwoFactorRepository) Delete(ctx context.Context, userID uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, deleteTwoFactorQuery, userID)
	if err != nil {
		return errs.Wrap("r.pool.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return twofactor.ErrNotSetUp
	}

	return nil
}

const useTwoFactorStepQuery = `
	UPDATE two_factors
	SET last_step = $2,
		updated_at = now()
	WHERE user_id = $1
		AND enabled
		AND last_step < $2
`

func (r *TwoFactorRepository) UseStep(ctx context.Context, userID uuid.UUID, step int64) error {
	return r.spend(ctx, useTwoFactorStepQuery, userID, step)
}

const useTwoFactorRecoveryQuery = `
	UPDATE two_factors
	SET recovery_hashes = array_remove(recovery_hashes, $2),
		updated_at = now()
	WHERE user_id = $1
		AND enabled
		AND $2 = ANY (recovery_hashes)
`

func (r *TwoFactorRepository) UseRecovery(ctx context.Context, userID uuid.UUID, hash string) error {
	return r.spend(ctx, useTwoFactorRecoveryQuery, userID, hash)
}

const replaceTwoFactorRecoveryQuery = `
	UPDATE two_factors
	SET last_step = $2,
		recovery_hashes = $3,
		updated_at = now()
	WHERE user_id = $1
		AND enabled
		AND last_step < $2
`

func (r *TwoFactorRepository) ReplaceRecovery(ctx context.Context, userID uuid.UUID, step int64, recovery []string) error {
	return r.spend(ctx, replaceTwoFactorRecoveryQuery, userID, step, recovery)
}

// spend runs an update that spends a code of userID; no row changed means
// the code was spent already or never was.
func (r *TwoFactorRepository) spend(ctx context.Context, query string, userID uuid.UUID, args ...any) error {
	tag, err := r.pool.Exec(ctx, query, append([]any{userID}, args...)...)
	if err != nil {
		return errs.Wrap("r.pool.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return twofactor.ErrInvalidCode
	}

	return nil
}

// missing tells why Enable changed nothing: no factor, or one enabled by
// another request first.
func (r *TwoFactorRepository) missing(ctx context.Context, userID uuid.UUID) error {
	if _, err := r.Get(ctx, userID); err != nil {
		return errs.Wrap("r.Get", err)
	}

	return twofactor.ErrInvalidCode
}
