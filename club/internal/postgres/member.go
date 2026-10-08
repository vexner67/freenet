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

// Transfer swaps the roles in a transaction: one statement would
// briefly hold two owners, which the unique index does not allow.
const (
	demoteOwnerQuery = `
		UPDATE club_members
		SET role = 'admin'
		WHERE club_id = $1
			AND user_id = $2
			AND role = 'owner'
	`
	promoteAdminQuery = `
		UPDATE club_members
		SET role = 'owner'
		WHERE club_id = $1
			AND user_id = $2
			AND role = 'admin'
	`
)

func (r *MemberRepository) Transfer(ctx context.Context, clubID, ownerID, newOwnerID uuid.UUID) error {
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		// The owner's row stays locked to the end: a second transfer waits
		// and finds no owner to demote.
		tag, err := tx.Exec(ctx, demoteOwnerQuery, clubID, ownerID)
		if err != nil {
			return errs.Wrap("tx.Exec", err)
		}
		if tag.RowsAffected() == 0 {
			return club.ErrCannotTransfer
		}

		if tag, err = tx.Exec(ctx, promoteAdminQuery, clubID, newOwnerID); err != nil {
			return errs.Wrap("tx.Exec", err)
		}
		if tag.RowsAffected() == 0 {
			return club.ErrNotAdmin
		}

		return nil
	})

	return errs.Wrap("pgx.BeginFunc", err)
}

const countAdminsQuery = `
	SELECT count(*)
	FROM club_members
	WHERE club_id = $1
		AND role IN ('admin', 'owner')
`

func (r *MemberRepository) CountAdmins(ctx context.Context, clubID uuid.UUID) (int, error) {
	var n int

	if err := r.pool.QueryRow(ctx, countAdminsQuery, clubID).Scan(&n); err != nil {
		return 0, errs.Wrap("r.pool.QueryRow", err)
	}

	return n, nil
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

// listUserClubsQuery: with $4, only the clubs the user runs.
const listUserClubsQuery = `
	SELECT
		m.id,
		m.role,
		m.pinned,
		c.id,
		c.name,
		c.description,
		COALESCE(c.address, ''),
		c.wall,
		c.created_at,
		c.member_count,
		c.avatar_id,
		c.website,
		c.admins_display,
		c.open_topics,
		c.topics_above_wall
	FROM club_members m
	JOIN clubs c ON c.id = m.club_id
	WHERE m.user_id = $1
		AND m.id < $2
		AND (NOT $4 OR m.role IN ('admin', 'owner'))
	ORDER BY m.id DESC
	LIMIT $3
`

func (r *MemberRepository) ListClubs(ctx context.Context, userID, before uuid.UUID, limit int, managed bool) ([]service.Membership, error) {
	rows, err := r.pool.Query(ctx, listUserClubsQuery, userID, cursor(before), limit, managed)
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
		m    service.Membership
		role string
	)

	c, err := scanClub(row, &m.ID, &role, &m.Pinned)
	if err != nil {
		return service.Membership{}, err
	}

	m.Role = club.Role(role)
	m.Club = c

	return m, nil
}

const countUserClubsQuery = `
	SELECT count(*)
	FROM club_members
	WHERE user_id = $1
		AND (NOT $2 OR role IN ('admin', 'owner'))
`

func (r *MemberRepository) CountClubs(ctx context.Context, userID uuid.UUID, managed bool) (int, error) {
	var n int

	if err := r.pool.QueryRow(ctx, countUserClubsQuery, userID, managed).Scan(&n); err != nil {
		return 0, errs.Wrap("r.pool.QueryRow", err)
	}

	return n, nil
}

// pinQuery pins only a club the user runs, and only while fewer than $4
// of those are pinned, counted in the same statement; unpinning, or
// pinning a pinned one, always goes through. Two pins at once may both
// see room for one: the menu then holds one more than the limit.
const pinQuery = `
	UPDATE club_members
	SET pinned = $3
	WHERE club_id = $1
		AND user_id = $2
		AND (
			NOT $3
			OR pinned
			OR (
				role IN ('admin', 'owner')
				AND (
					SELECT count(*)
					FROM club_members
					WHERE user_id = $2
						AND pinned
						AND role IN ('admin', 'owner')
				) < $4
			)
		)
`

func (r *MemberRepository) Pin(ctx context.Context, clubID, userID uuid.UUID, pinned bool, limit int) (bool, error) {
	tag, err := r.pool.Exec(ctx, pinQuery, clubID, userID, pinned, limit)
	if err != nil {
		return false, errs.Wrap("r.pool.Exec", err)
	}

	// Not a member is not a full menu: the service checked the role.
	if tag.RowsAffected() == 0 && pinned {
		role, err := r.Role(ctx, clubID, userID)
		if err != nil {
			return false, errs.Wrap("r.Role", err)
		}

		return !role.Manages(), nil
	}

	return true, nil
}

const pinnedClubsQuery = `
	SELECT
		c.id,
		c.name,
		c.description,
		COALESCE(c.address, ''),
		c.wall,
		c.created_at,
		c.member_count,
		c.avatar_id,
		c.website,
		c.admins_display,
		c.open_topics,
		c.topics_above_wall
	FROM club_members m
	JOIN clubs c ON c.id = m.club_id
	WHERE m.user_id = $1
		AND m.pinned
		AND m.role IN ('admin', 'owner')
	ORDER BY m.id
`

func (r *MemberRepository) Pinned(ctx context.Context, userID uuid.UUID) ([]*club.Club, error) {
	rows, err := r.pool.Query(ctx, pinnedClubsQuery, userID)
	if err != nil {
		return nil, errs.Wrap("r.pool.Query", err)
	}

	return collectClubs(rows)
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
