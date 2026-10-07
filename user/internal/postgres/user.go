package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/user/internal/user"
)

const (
	uniqueViolationCode = "23505"

	// Unique indexes from migrations; the name tells which field clashed.
	emailIndex    = "users_email_uidx"
	nicknameIndex = "users_nickname_uidx"
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
		email,
		name,
		surname,
		nickname,
		status,
		gender,
		relationship,
		worldview,
		political_views,
		birthday,
		show_birthday,
		show_relationship,
		show_worldview,
		show_political_views,
		wall_write,
		messages_write,
		avatar_id
	)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
`

func (r *UserRepository) Save(ctx context.Context, u *user.User) error {
	p := u.Profile()
	pr := u.Privacy()

	if _, err := r.pool.Exec(
		ctx,
		saveUserQuery,
		u.ID(),
		u.Email().String(),
		p.Name.String(),
		p.Surname.String(),
		p.Nickname.String(),
		p.Status.String(),
		p.Gender.String(),
		p.Relationship.String(),
		p.Worldview.String(),
		p.PoliticalViews.String(),
		birthdayToDB(p.Birthday),
		pr.ShowBirthday,
		pr.ShowRelationship,
		pr.ShowWorldview,
		pr.ShowPoliticalViews,
		pr.WallWrite.String(),
		pr.MessagesWrite.String(),
		avatarToDB(u.AvatarID()),
	); err != nil {
		return errs.Wrap("r.pool.Exec", translateError(err))
	}

	return nil
}

const updateUserQuery = `
	UPDATE users
	SET
		name = $2,
		surname = $3,
		nickname = $4,
		status = $5,
		gender = $6,
		relationship = $7,
		worldview = $8,
		political_views = $9,
		birthday = $10,
		show_birthday = $11,
		show_relationship = $12,
		show_worldview = $13,
		show_political_views = $14,
		wall_write = $15,
		messages_write = $16,
		avatar_id = $17,
		deactivated_at = $18,
		purge_at = $19,
		deactivation_reason = $20,
		updated_at = now()
	WHERE id = $1
`

func (r *UserRepository) Update(ctx context.Context, u *user.User) error {
	p := u.Profile()
	pr := u.Privacy()
	d := u.Deactivation()

	tag, err := r.pool.Exec(
		ctx,
		updateUserQuery,
		u.ID(),
		p.Name.String(),
		p.Surname.String(),
		p.Nickname.String(),
		p.Status.String(),
		p.Gender.String(),
		p.Relationship.String(),
		p.Worldview.String(),
		p.PoliticalViews.String(),
		birthdayToDB(p.Birthday),
		pr.ShowBirthday,
		pr.ShowRelationship,
		pr.ShowWorldview,
		pr.ShowPoliticalViews,
		pr.WallWrite.String(),
		pr.MessagesWrite.String(),
		avatarToDB(u.AvatarID()),
		timeToDB(d.At),
		timeToDB(d.PurgeAt),
		d.Reason.String(),
	)
	if err != nil {
		return errs.Wrap("r.pool.Exec", translateError(err))
	}

	if tag.RowsAffected() == 0 {
		return user.ErrNotFound
	}

	return nil
}

const getUserByIDQuery = `
	SELECT
		id,
		email,
		name,
		surname,
		nickname,
		status,
		gender,
		relationship,
		worldview,
		political_views,
		birthday,
		show_birthday,
		show_relationship,
		show_worldview,
		show_political_views,
		wall_write,
		messages_write,
		avatar_id,
		deactivated_at,
		purge_at,
		deactivation_reason
	FROM users
	WHERE id = $1
`

func (r *UserRepository) GetByID(ctx context.Context, id uuid.UUID) (*user.User, error) {
	u, err := scanUser(r.pool.QueryRow(ctx, getUserByIDQuery, id))
	if err != nil {
		return nil, errs.Wrap("scanUser", err)
	}

	return u, nil
}

const getUserByNicknameQuery = `
	SELECT
		id,
		email,
		name,
		surname,
		nickname,
		status,
		gender,
		relationship,
		worldview,
		political_views,
		birthday,
		show_birthday,
		show_relationship,
		show_worldview,
		show_political_views,
		wall_write,
		messages_write,
		avatar_id,
		deactivated_at,
		purge_at,
		deactivation_reason
	FROM users
	WHERE lower(nickname) = lower($1)
		AND nickname <> ''
`

func (r *UserRepository) GetByNickname(ctx context.Context, nickname user.Nickname) (*user.User, error) {
	u, err := scanUser(r.pool.QueryRow(ctx, getUserByNicknameQuery, nickname.String()))
	if err != nil {
		return nil, errs.Wrap("scanUser", err)
	}

	return u, nil
}

const getUsersByIDsQuery = `
	SELECT
		id,
		email,
		name,
		surname,
		nickname,
		status,
		gender,
		relationship,
		worldview,
		political_views,
		birthday,
		show_birthday,
		show_relationship,
		show_worldview,
		show_political_views,
		wall_write,
		messages_write,
		avatar_id,
		deactivated_at,
		purge_at,
		deactivation_reason
	FROM users
	WHERE id = ANY($1)
`

func (r *UserRepository) GetByIDs(ctx context.Context, ids []uuid.UUID) ([]*user.User, error) {
	rows, err := r.pool.Query(ctx, getUsersByIDsQuery, ids)
	if err != nil {
		return nil, errs.Wrap("r.pool.Query", err)
	}
	defer rows.Close()

	users := make([]*user.User, 0, len(ids))
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, errs.Wrap("scanUser", err)
		}
		users = append(users, u)
	}

	if err = rows.Err(); err != nil {
		return nil, errs.Wrap("rows.Err", err)
	}

	return users, nil
}

const getUserIDByEmailQuery = `
	SELECT id
	FROM users
	WHERE lower(email) = lower($1)
`

func (r *UserRepository) GetIDByEmail(ctx context.Context, email user.Email) (uuid.UUID, error) {
	var id uuid.UUID

	err := r.pool.QueryRow(ctx, getUserIDByEmailQuery, email.String()).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, user.ErrNotFound
	}
	if err != nil {
		return uuid.Nil, errs.Wrap("r.pool.QueryRow", err)
	}

	return id, nil
}

func scanUser(row pgx.Row) (*user.User, error) {
	var (
		id                                         uuid.UUID
		email, name, surname, nickname, status     string
		gender, relationship, worldview, political string
		birthday                                   *time.Time
		privacy                                    user.Privacy
		wallWrite, messagesWrite                   string
		avatarID                                   *uuid.UUID
		deactivatedAt, purgeAt                     *time.Time
		reason                                     string
	)

	err := row.Scan(
		&id,
		&email,
		&name,
		&surname,
		&nickname,
		&status,
		&gender,
		&relationship,
		&worldview,
		&political,
		&birthday,
		&privacy.ShowBirthday,
		&privacy.ShowRelationship,
		&privacy.ShowWorldview,
		&privacy.ShowPoliticalViews,
		&wallWrite,
		&messagesWrite,
		&avatarID,
		&deactivatedAt,
		&purgeAt,
		&reason,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, user.ErrNotFound
	}
	if err != nil {
		return nil, errs.Wrap("row.Scan", err)
	}

	// Data comes from our own table and was validated on write.
	privacy.WallWrite = user.WallWrite(wallWrite)
	privacy.MessagesWrite = user.MessagesWrite(messagesWrite)

	return user.Restore(user.Stored{
		ID:    id,
		Email: user.Email(email),
		Profile: user.Profile{
			Name:           user.Name(name),
			Surname:        user.Surname(surname),
			Nickname:       user.Nickname(nickname),
			Status:         user.Status(status),
			Gender:         user.Gender(gender),
			Relationship:   user.Relationship(relationship),
			Worldview:      user.Worldview(worldview),
			PoliticalViews: user.PoliticalViews(political),
			Birthday:       birthdayFromDB(birthday),
		},
		Privacy:  privacy,
		AvatarID: avatarFromDB(avatarID),
		Deactivation: user.Deactivation{
			At:      timeFromDB(deactivatedAt),
			PurgeAt: timeFromDB(purgeAt),
			Reason:  user.DeactivationReason(reason),
		},
	}), nil
}

// An unset birthday is stored as NULL rather than year 1.
func birthdayToDB(b user.Birthday) *time.Time {
	if b.Time().IsZero() {
		return nil
	}

	t := b.Time()

	return &t
}

func birthdayFromDB(t *time.Time) user.Birthday {
	if t == nil {
		return user.Birthday{}
	}

	return user.Birthday(*t)
}

// No photo is stored as NULL.
func avatarToDB(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}

	return &id
}

func avatarFromDB(id *uuid.UUID) uuid.UUID {
	if id == nil {
		return uuid.Nil
	}

	return *id
}

// timeToDB stores the zero time as NULL.
func timeToDB(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}

	return &t
}

func timeFromDB(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}

	return t.UTC()
}

// searchUsersSelect is completed by searchUsersQuery with one LIKE per word.
const searchUsersSelect = `
	SELECT
		id,
		email,
		name,
		surname,
		nickname,
		status,
		gender,
		relationship,
		worldview,
		political_views,
		birthday,
		show_birthday,
		show_relationship,
		show_worldview,
		show_political_views,
		wall_write,
		messages_write,
		avatar_id,
		deactivated_at,
		purge_at,
		deactivation_reason
	FROM users
	WHERE nickname <> ''
		AND deactivated_at IS NULL
		AND id < $1`

// searchHaystack must match the expression of users_search_idx exactly,
// or the trigram index is not used.
const searchHaystack = `lower(nickname || ' ' || name || ' ' || surname)`

// searchUsersQuery ANDs one "LIKE $n" per word: unlike LIKE ALL (array),
// such conditions are served by the trigram index. Only placeholders are
// added to the text; the words are passed as parameters.
func searchUsersQuery(words int) string {
	var b strings.Builder

	b.WriteString(searchUsersSelect)
	for i := range words {
		fmt.Fprintf(&b, "\n\t\tAND %s LIKE $%d", searchHaystack, i+3)
	}
	b.WriteString("\n\tORDER BY id DESC\n\tLIMIT $2")

	return b.String()
}

func (r *UserRepository) Search(ctx context.Context, q user.SearchQuery, before uuid.UUID, limit int) ([]*user.User, error) {
	if before == uuid.Nil {
		before = uuid.Max
	}

	args := []any{before, limit}
	for _, w := range q.Words() {
		args = append(args, "%"+escapeLike(w)+"%")
	}

	rows, err := r.pool.Query(ctx, searchUsersQuery(len(q.Words())), args...)
	if err != nil {
		return nil, errs.Wrap("r.pool.Query", err)
	}
	defer rows.Close()

	var users []*user.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, errs.Wrap("scanUser", err)
		}
		users = append(users, u)
	}

	if err = rows.Err(); err != nil {
		return nil, errs.Wrap("rows.Err", err)
	}

	return users, nil
}

// escapeLike makes % and _ in a search word match themselves.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

func escapeLike(s string) string {
	return likeEscaper.Replace(s)
}

func translateError(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != uniqueViolationCode {
		return err
	}

	switch pgErr.ConstraintName {
	case emailIndex:
		return user.ErrAlreadyExists
	case nicknameIndex:
		return user.ErrNicknameTaken
	default:
		return err
	}
}

// deleteDueUsersQuery deletes up to $2 pages due to be erased by $1; SKIP
// LOCKED lets replicas purge side by side without taking the same ones.
const deleteDueUsersQuery = `
	DELETE FROM users
	WHERE id IN (
		SELECT id
		FROM users
		WHERE purge_at <= $1
		ORDER BY purge_at
		LIMIT $2
		FOR UPDATE SKIP LOCKED
	)
	RETURNING id
`

func (r *UserRepository) DeleteDue(ctx context.Context, now time.Time, limit int) ([]uuid.UUID, error) {
	rows, err := r.conn(ctx).Query(ctx, deleteDueUsersQuery, now, limit)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, errs.Wrap("pgx.CollectRows", err)
	}

	return ids, nil
}

// conn is the transaction of ctx (service.TxManager), or the pool outside
// one.
func (r *UserRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}
