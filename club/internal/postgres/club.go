// Package postgres keeps clubs in PostgreSQL.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/club/internal/club"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
)

// constraintOf is the constraint a PostgreSQL error is about, "" if none.
func constraintOf(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.ConstraintName
	}

	return ""
}

type ClubRepository struct {
	pool *pgxpool.Pool
}

func NewClubRepository(pool *pgxpool.Pool) *ClubRepository {
	return &ClubRepository{
		pool: pool,
	}
}

// createClubQuery stores the club and its owner in one statement, so a
// club never exists without one; the foreign key is checked at the end of
// the statement.
const createClubQuery = `
	WITH created AS (
		INSERT INTO clubs (
			id,
			name,
			description,
			wall,
			admins_display,
			member_count,
			created_at
		)
		VALUES ($1, $2, $3, $4, $8, 1, $5)
	)
	INSERT INTO club_members (
		id,
		club_id,
		user_id,
		role
	)
	VALUES ($6, $1, $7, 'owner')
`

func (r *ClubRepository) Create(ctx context.Context, c *club.Club, ownerID uuid.UUID) error {
	membershipID, err := uuid.NewV7()
	if err != nil {
		return errs.Wrap("uuid.NewV7", err)
	}

	_, err = pgpool.Conn(ctx, r.pool).Exec(ctx, createClubQuery,
		c.ID(), c.Name().String(), c.Description().String(), c.Wall().String(), c.CreatedAt(), membershipID, ownerID,
		c.AdminsDisplay().String())
	if err != nil {
		return errs.Wrap("r.pool.Exec", err)
	}

	return nil
}

const (
	getClubQuery = `
		SELECT
			id,
			name,
			description,
			COALESCE(address, ''),
			wall,
			created_at,
			member_count,
			avatar_id,
			website,
			admins_display,
			open_topics,
			topics_above_wall,
			backdrop_left,
			backdrop_right,
			closed
		FROM clubs
		WHERE id = $1
	`
	getClubByAddressQuery = `
		SELECT
			id,
			name,
			description,
			COALESCE(address, ''),
			wall,
			created_at,
			member_count,
			avatar_id,
			website,
			admins_display,
			open_topics,
			topics_above_wall,
			backdrop_left,
			backdrop_right,
			closed
		FROM clubs
		WHERE lower(address) = lower($1)
	`
	getClubsQuery = `
		SELECT
			id,
			name,
			description,
			COALESCE(address, ''),
			wall,
			created_at,
			member_count,
			avatar_id,
			website,
			admins_display,
			open_topics,
			topics_above_wall,
			backdrop_left,
			backdrop_right,
			closed
		FROM clubs
		WHERE id = ANY($1)
	`
)

func (r *ClubRepository) Get(ctx context.Context, id uuid.UUID) (*club.Club, error) {
	return r.get(ctx, getClubQuery, id)
}

func (r *ClubRepository) GetByAddress(ctx context.Context, address club.Address) (*club.Club, error) {
	return r.get(ctx, getClubByAddressQuery, address.String())
}

func (r *ClubRepository) get(ctx context.Context, query string, arg any) (*club.Club, error) {
	c, err := scanClub(r.pool.QueryRow(ctx, query, arg))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, club.ErrNotFound
	}
	if err != nil {
		return nil, errs.Wrap("scanClub", err)
	}

	return c, nil
}

func (r *ClubRepository) GetMany(ctx context.Context, ids []uuid.UUID) ([]*club.Club, error) {
	rows, err := r.pool.Query(ctx, getClubsQuery, ids)
	if err != nil {
		return nil, errs.Wrap("r.pool.Query", err)
	}

	return collectClubs(rows)
}

// collectClubs reads the clubs of rows and closes them.
func collectClubs(rows pgx.Rows) ([]*club.Club, error) {
	defer rows.Close()

	var found []*club.Club
	for rows.Next() {
		c, err := scanClub(rows)
		if err != nil {
			return nil, errs.Wrap("scanClub", err)
		}
		found = append(found, c)
	}

	if err := rows.Err(); err != nil {
		return nil, errs.Wrap("rows.Err", err)
	}

	return found, nil
}

// scanClub reads the columns of the queries above, after those that
// before point to. Data comes from our own table and was validated on
// write.
func scanClub(row pgx.Row, before ...any) (*club.Club, error) {
	var (
		s                                club.Stored
		name, description, address, wall string
		website, adminsDisplay           string
		avatarID                         *uuid.UUID
		backdropLeft, backdropRight      *uuid.UUID
	)

	dest := append(before, &s.ID, &name, &description, &address, &wall, &s.CreatedAt, &s.Members, &avatarID,
		&website, &adminsDisplay, &s.OpenTopics, &s.TopicsAboveWall, &backdropLeft, &backdropRight,
		&s.Closed)
	if err := row.Scan(dest...); err != nil {
		return nil, err
	}

	if avatarID != nil {
		s.AvatarID = *avatarID
	}
	if backdropLeft != nil {
		s.Backdrop.Left = *backdropLeft
	}
	if backdropRight != nil {
		s.Backdrop.Right = *backdropRight
	}

	s.Name = club.Name(name)
	s.Description = club.Description(description)
	s.Address = club.Address(address)
	s.Website = club.Website(website)
	s.Wall = club.Wall(wall)
	s.AdminsDisplay = club.AdminsDisplay(adminsDisplay)
	s.CreatedAt = s.CreatedAt.UTC()

	return club.Restore(s), nil
}

const updateClubQuery = `
	UPDATE clubs
	SET name = $2,
		description = $3,
		address = NULLIF($4, ''),
		wall = $5,
		website = $6,
		admins_display = $7,
		open_topics = $8,
		topics_above_wall = $9,
		closed = $10
	WHERE id = $1
`

func (r *ClubRepository) Update(ctx context.Context, c *club.Club) error {
	tag, err := pgpool.Conn(ctx, r.pool).Exec(ctx, updateClubQuery,
		c.ID(), c.Name().String(), c.Description().String(), c.Address().String(), c.Wall().String(),
		c.Website().String(), c.AdminsDisplay().String(), c.OpenTopics(), c.TopicsAboveWall(), c.Closed())
	if constraintOf(err) == "clubs_address_key" {
		return club.ErrAddressTaken
	}
	if err != nil {
		return errs.Wrap("r.pool.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return club.ErrNotFound
	}

	return nil
}

const setClubAvatarQuery = `
	UPDATE clubs
	SET avatar_id = $2
	WHERE id = $1
`

func (r *ClubRepository) SetAvatar(ctx context.Context, c *club.Club) error {
	var avatarID *uuid.UUID
	if id := c.AvatarID(); id != uuid.Nil {
		avatarID = &id
	}

	tag, err := r.pool.Exec(ctx, setClubAvatarQuery, c.ID(), avatarID)
	if err != nil {
		return errs.Wrap("r.pool.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return club.ErrNotFound
	}

	return nil
}

// setClubBackdropQueries change one side only: admins replacing the two
// sides at once do not undo each other.
var setClubBackdropQueries = map[club.BackdropSide]string{
	club.BackdropLeft: `
		UPDATE clubs
		SET backdrop_left = $2
		WHERE id = $1
	`,
	club.BackdropRight: `
		UPDATE clubs
		SET backdrop_right = $2
		WHERE id = $1
	`,
}

func (r *ClubRepository) SetBackdrop(ctx context.Context, c *club.Club, side club.BackdropSide) error {
	photoID := c.Backdrop().Left
	if side == club.BackdropRight {
		photoID = c.Backdrop().Right
	}

	var arg *uuid.UUID
	if photoID != uuid.Nil {
		arg = &photoID
	}

	tag, err := r.pool.Exec(ctx, setClubBackdropQueries[side], c.ID(), arg)
	if err != nil {
		return errs.Wrap("r.pool.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return club.ErrNotFound
	}

	return nil
}

// searchClubsSelect is completed by searchClubsQuery with one LIKE per word.
const searchClubsSelect = `
	SELECT
		id,
		name,
		description,
		COALESCE(address, ''),
		wall,
		created_at,
		member_count,
		avatar_id,
		website,
		admins_display,
		open_topics,
		topics_above_wall,
		backdrop_left,
		backdrop_right,
		closed
	FROM clubs
	WHERE id < $1`

// searchHaystack must match the expression of clubs_search_idx exactly,
// or the trigram index is not used.
const searchHaystack = `lower(name || ' ' || COALESCE(address, ''))`

// searchClubsQuery ANDs one "LIKE $n" per word: unlike LIKE ALL (array),
// such conditions are served by the trigram index. Only placeholders are
// added to the text; the words are passed as parameters.
func searchClubsQuery(words int) string {
	var b strings.Builder

	b.WriteString(searchClubsSelect)
	for i := range words {
		fmt.Fprintf(&b, "\n\t\tAND %s LIKE $%d", searchHaystack, i+3)
	}
	b.WriteString("\n\tORDER BY id DESC\n\tLIMIT $2")

	return b.String()
}

func (r *ClubRepository) Search(ctx context.Context, q club.SearchQuery, before uuid.UUID, limit int) ([]*club.Club, error) {
	args := []any{cursor(before), limit}
	for _, w := range q.Words() {
		args = append(args, "%"+escapeLike(w)+"%")
	}

	rows, err := r.pool.Query(ctx, searchClubsQuery(len(q.Words())), args...)
	if err != nil {
		return nil, errs.Wrap("r.pool.Query", err)
	}

	return collectClubs(rows)
}

// countClubsQuery reads the whole table: the page "о сайте" is opened
// seldom.
const countClubsQuery = `
	SELECT count(*)
	FROM clubs
`

func (r *ClubRepository) Count(ctx context.Context) (int, error) {
	var n int

	if err := r.pool.QueryRow(ctx, countClubsQuery).Scan(&n); err != nil {
		return 0, errs.Wrap("r.pool.QueryRow", err)
	}

	return n, nil
}

// deleteClubQuery deletes the club only while ownerID owns it; its
// members and requests go by cascade.
const deleteClubQuery = `
	DELETE FROM clubs
	WHERE id = $1
		AND EXISTS (
			SELECT 1
			FROM club_members
			WHERE club_id = $1
				AND user_id = $2
				AND role = 'owner'
		)
`

func (r *ClubRepository) Delete(ctx context.Context, clubID, ownerID uuid.UUID) error {
	tag, err := pgpool.Conn(ctx, r.pool).Exec(ctx, deleteClubQuery, clubID, ownerID)
	if err != nil {
		return errs.Wrap("r.pool.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return club.ErrCannotDelete
	}

	return nil
}

// escapeLike makes % and _ in a word match themselves.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
