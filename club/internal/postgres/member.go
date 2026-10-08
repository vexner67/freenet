package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/club/internal/club"
	"github.com/vexner67/freenet/club/internal/service"
	"github.com/vexner67/freenet/pkg/errs"
)

type MemberRepository struct {
	pool *pgxpool.Pool
}

func NewMemberRepository(pool *pgxpool.Pool) *MemberRepository {
	return &MemberRepository{
		pool: pool,
	}
}

const roleQuery = `
	SELECT role
	FROM club_members
	WHERE club_id = $1
		AND user_id = $2
`

func (r *MemberRepository) Role(ctx context.Context, clubID, userID uuid.UUID) (club.Role, error) {
	var role string

	err := r.pool.QueryRow(ctx, roleQuery, clubID, userID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return club.RoleNone, nil
	}
	if err != nil {
		return club.RoleNone, errs.Wrap("r.pool.QueryRow", err)
	}

	return club.Role(role), nil
}

const ownerQuery = `
	SELECT user_id
	FROM club_members
	WHERE club_id = $1
		AND role = 'owner'
`

func (r *MemberRepository) Owner(ctx context.Context, clubID uuid.UUID) (uuid.UUID, error) {
	var id uuid.UUID

	err := r.pool.QueryRow(ctx, ownerQuery, clubID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, nil
	}
	if err != nil {
		return uuid.Nil, errs.Wrap("r.pool.QueryRow", err)
	}

	return id, nil
}

// joinQuery counts the member in the statement that adds them, so the
// count never drifts; joining twice changes nothing.
const joinQuery = `
	WITH joined AS (
		INSERT INTO club_members (
			id,
			club_id,
			user_id,
			role
		)
		VALUES ($1, $2, $3, 'member')
		ON CONFLICT (club_id, user_id) DO NOTHING
		RETURNING club_id
	)
	UPDATE clubs
	SET member_count = member_count + 1
	WHERE id IN (SELECT club_id FROM joined)
`

func (r *MemberRepository) Join(ctx context.Context, clubID, userID uuid.UUID) error {
	id, err := uuid.NewV7()
	if err != nil {
		return errs.Wrap("uuid.NewV7", err)
	}

	_, err = r.pool.Exec(ctx, joinQuery, id, clubID, userID)
	if constraintOf(err) == "club_members_club_id_fkey" {
		return club.ErrNotFound
	}
	if err != nil {
		return errs.Wrap("r.pool.Exec", err)
	}

	return nil
}

// leaveQuery never takes the owner out, whatever the caller checked.
const leaveQuery = `
	WITH gone AS (
		DELETE FROM club_members
		WHERE club_id = $1
			AND user_id = $2
			AND role <> 'owner'
		RETURNING club_id
	)
	UPDATE clubs
	SET member_count = member_count - 1
	WHERE id IN (SELECT club_id FROM gone)
`

func (r *MemberRepository) Leave(ctx context.Context, clubID, userID uuid.UUID) error {
	if _, err := r.pool.Exec(ctx, leaveQuery, clubID, userID); err != nil {
		return errs.Wrap("r.pool.Exec", err)
	}

	return nil
}

const setRoleQuery = `
	UPDATE club_members
	SET role = $3
	WHERE club_id = $1
		AND user_id = $2
		AND role <> 'owner'
`

func (r *MemberRepository) SetRole(ctx context.Context, clubID, userID uuid.UUID, role club.Role) error {
	tag, err := r.pool.Exec(ctx, setRoleQuery, clubID, userID, role.String())
	if err != nil {
		return errs.Wrap("r.pool.Exec", err)
	}

	// Left between the check and the change.
	if tag.RowsAffected() == 0 {
		return club.ErrNotMember
	}

	return nil
}

const listMembersQuery = `
	SELECT
		id,
		user_id,
		role
	FROM club_members
	WHERE club_id = $1
		AND id < $2
		AND (NOT $4 OR role IN ('admin', 'owner'))
	ORDER BY id DESC
	LIMIT $3
`

func (r *MemberRepository) List(ctx context.Context, clubID, before uuid.UUID, limit int, adminsOnly bool) ([]service.Member, error) {
	rows, err := r.pool.Query(ctx, listMembersQuery, clubID, cursor(before), limit, adminsOnly)
	if err != nil {
		return nil, errs.Wrap("r.pool.Query", err)
	}
	defer rows.Close()

	var found []service.Member
	for rows.Next() {
		var (
			m    service.Member
			role string
		)
		if err = rows.Scan(&m.ID, &m.UserID, &role); err != nil {
			return nil, errs.Wrap("rows.Scan", err)
		}
		m.Role = club.Role(role)
		found = append(found, m)
	}

	if err = rows.Err(); err != nil {
		return nil, errs.Wrap("rows.Err", err)
	}

	return found, nil
}

// cursor is before for "id < before": uuid.Nil starts from the newest.
func cursor(before uuid.UUID) uuid.UUID {
	if before == uuid.Nil {
		return uuid.Max
	}

	return before
}

const listUserClubsQuery = `
	SELECT
		m.id,
		m.role,
		c.id,
		c.name,
		c.description,
		COALESCE(c.address, ''),
		c.wall,
		c.created_at,
		c.member_count
	FROM club_members m
	JOIN clubs c ON c.id = m.club_id
	WHERE m.user_id = $1
		AND m.id < $2
	ORDER BY m.id DESC
	LIMIT $3
`

func (r *MemberRepository) ListClubs(ctx context.Context, userID, before uuid.UUID, limit int) ([]service.Membership, error) {
	rows, err := r.pool.Query(ctx, listUserClubsQuery, userID, cursor(before), limit)
	if err != nil {
		return nil, errs.Wrap("r.pool.Query", err)
	}
	defer rows.Close()

	var found []service.Membership
	for rows.Next() {
		m, err := scanMembership(rows)
		if err != nil {
			return nil, errs.Wrap("scanMembership", err)
		}
		found = append(found, m)
	}

	if err = rows.Err(); err != nil {
		return nil, errs.Wrap("rows.Err", err)
	}

	return found, nil
}

func scanMembership(row pgx.Row) (service.Membership, error) {
	var (
		m                                      service.Membership
		s                                      club.Stored
		role, name, description, address, wall string
	)

	err := row.Scan(&m.ID, &role, &s.ID, &name, &description, &address, &wall, &s.CreatedAt, &s.Members)
	if err != nil {
		return service.Membership{}, err
	}

	s.Name = club.Name(name)
	s.Description = club.Description(description)
	s.Address = club.Address(address)
	s.Wall = club.Wall(wall)
	s.CreatedAt = s.CreatedAt.UTC()

	m.Role = club.Role(role)
	m.Club = club.Restore(s)

	return m, nil
}

const userClubIDsQuery = `
	SELECT club_id
	FROM club_members
	WHERE user_id = $1
	ORDER BY id DESC
	LIMIT $2
`

func (r *MemberRepository) ClubIDs(ctx context.Context, userID uuid.UUID, limit int) ([]uuid.UUID, error) {
	rows, err := r.pool.Query(ctx, userClubIDsQuery, userID, limit)
	if err != nil {
		return nil, errs.Wrap("r.pool.Query", err)
	}

	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, errs.Wrap("pgx.CollectRows", err)
	}

	return ids, nil
}

// deleteUserMembershipsQuery takes the user out of every club, the owned
// ones too, with the counts in the same statement.
const deleteUserMembershipsQuery = `
	WITH gone AS (
		DELETE FROM club_members
		WHERE user_id = $1
		RETURNING club_id
	)
	UPDATE clubs c
	SET member_count = c.member_count - 1
	FROM gone
	WHERE c.id = gone.club_id
`

func (r *MemberRepository) DeleteByUser(ctx context.Context, userID uuid.UUID) error {
	if _, err := r.pool.Exec(ctx, deleteUserMembershipsQuery, userID); err != nil {
		return errs.Wrap("r.pool.Exec", err)
	}

	return nil
}
