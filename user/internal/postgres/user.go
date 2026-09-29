package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{
		pool: pool,
	}
}

const saveUserQuery = `
    INSERT INTO users (
        id,
        name,
        surname,
        nickname,
        status,
        worldview,
        gender,
        relationship,
        political_views,
        birthday
        email,
    )
    VALUES (
        $1,
        $2,
        $3,
        $4,
        $5,
        $6,
        $7,
        $8,
        $9,
        $10,
        $11
    )
`

func (r *UserRepository) Save(ctx context.Context, u *user.User) error {
	if _, err := r.pool.Exec(
		ctx,
		saveUserQuery,
		u.ID(),
		u.Name().String(),
		u.Surname().String(),
		u.Nickname().String(),
		u.Status().String(),
		u.Worldview().String(),
		u.Gender().String(),
		u.Relationship().String(),
		u.PoliticalViews().String(),
		u.Birthday().Time(),
		u.Email().String(),
	); err != nil {
		return errs.Wrap("r.pool.Exec", err)
	}

	return nil
}
