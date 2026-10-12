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
	"github.com/vexner67/freenet/pkg/pgpool"
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

const rolesQuery = `
	SELECT
		club_id,
		role
	FROM club_members
	WHERE user_id = $1
		AND club_id = ANY($2)
`

func (r *MemberRepository) Roles(ctx context.Context, userID uuid.UUID, clubIDs []uuid.UUID) (map[uuid.UUID]club.Role, error) {
	rows, err := r.pool.Query(ctx, rolesQuery, userID, clubIDs)
	if err != nil {
		return nil, errs.Wrap("r.pool.Query", err)
	}
	defer rows.Close()

	roles := make(map[uuid.UUID]club.Role, len(clubIDs))
	for rows.Next() {
		var (
			id   uuid.UUID
			role string
		)
		if err = rows.Scan(&id, &role); err != nil {
			return nil, errs.Wrap("rows.Scan", err)
		}
		roles[id] = club.Role(role)
	}
	if err = rows.Err(); err != nil {
		return nil, errs.Wrap("rows.Err", err)
	}

	return roles, nil
}

const seatQuery = `
	SELECT
		role,
		title,
		hidden,
		maybe
	FROM club_members
	WHERE club_id = $1
		AND user_id = $2
`

func (r *MemberRepository) Seat(ctx context.Context, clubID, userID uuid.UUID) (club.Seat, error) {
	var (
		seat        club.Seat
		role, title string
	)

	err := r.pool.QueryRow(ctx, seatQuery, clubID, userID).Scan(&role, &title, &seat.Hidden, &seat.Maybe)
	if errors.Is(err, pgx.ErrNoRows) {
		return club.Seat{}, nil
	}
	if err != nil {
		return club.Seat{}, errs.Wrap("r.pool.QueryRow", err)
	}

	seat.Role, seat.Title = club.Role(role), club.Title(title)

	return seat, nil
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
// counts never drift. A member who joins again changes only whether they
// come for sure, and the owner not even that; xmax tells the new row from
// the changed one.
const joinQuery = `
	WITH joined AS (
		INSERT INTO club_members (
			id,
			club_id,
			user_id,
			role,
			maybe
		)
		VALUES ($1, $2, $3, 'member', $4)
		ON CONFLICT (club_id, user_id) DO UPDATE
		SET maybe = EXCLUDED.maybe
		WHERE club_members.role <> 'owner'
			AND club_members.maybe <> EXCLUDED.maybe
		RETURNING club_id, (xmax = 0) AS added
	)
	UPDATE clubs c
	SET member_count = c.member_count + joined.added::integer,
		maybe_count = c.maybe_count + CASE
			WHEN joined.added THEN $4::boolean::integer
			WHEN $4 THEN 1
			ELSE -1
		END
	FROM joined
	WHERE c.id = joined.club_id
`

func (r *MemberRepository) Join(ctx context.Context, clubID, userID uuid.UUID, maybe bool) error {
	id, err := uuid.NewV7()
	if err != nil {
		return errs.Wrap("uuid.NewV7", err)
	}

	_, err = r.pool.Exec(ctx, joinQuery, id, clubID, userID, maybe)
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
		RETURNING club_id, maybe
	)
	UPDATE clubs c
	SET member_count = c.member_count - 1,
		maybe_count = c.maybe_count - gone.maybe::integer
	FROM gone
	WHERE c.id = gone.club_id
`

func (r *MemberRepository) Leave(ctx context.Context, clubID, userID uuid.UUID) error {
	if _, err := r.pool.Exec(ctx, leaveQuery, clubID, userID); err != nil {
		return errs.Wrap("r.pool.Exec", err)
	}

	return nil
}

// setRoleQuery: a member has no title and is not hidden.
const setRoleQuery = `
	UPDATE club_members
	SET
		role = $3,
		title = CASE WHEN $3 = 'member' THEN '' ELSE title END,
		hidden = hidden AND $3 <> 'member'
	WHERE club_id = $1
		AND user_id = $2
		AND role <> 'owner'
`

// SetRole runs in the transaction of ctx, if any: an appointment is
// recorded in the outbox with it.
func (r *MemberRepository) SetRole(ctx context.Context, clubID, userID uuid.UUID, role club.Role) error {
	tag, err := pgpool.Conn(ctx, r.pool).Exec(ctx, setRoleQuery, clubID, userID, role.String())
	if err != nil {
		return errs.Wrap("r.pool.Exec", err)
	}

	// Left between the check and the change.
	if tag.RowsAffected() == 0 {
		return club.ErrNotMember
	}

	return nil
}

// What the change does not name stays: each of the two is its own
// assignment, so changes of different things do not undo each other.
const describeMemberQuery = `
	UPDATE club_members
	SET
		title = COALESCE($3, title),
		hidden = COALESCE($4, hidden)
	WHERE club_id = $1
		AND user_id = $2
		AND role IN ('admin', 'owner')
`

func (r *MemberRepository) Describe(ctx context.Context, clubID, userID uuid.UUID, d club.SeatChange) error {
	var title *string
	if d.Title != nil {
		title = new(d.Title.String())
	}

	tag, err := r.pool.Exec(ctx, describeMemberQuery, clubID, userID, title, d.Hidden)
	if err != nil {
		return errs.Wrap("r.pool.Exec", err)
	}

	// Dismissed between the check and the change.
	if tag.RowsAffected() == 0 {
		return club.ErrNotManager
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
		AND NOT ($2 AND hidden)
`

func (r *MemberRepository) CountAdmins(ctx context.Context, clubID uuid.UUID, shownOnly bool) (int, error) {
	var n int

	if err := r.pool.QueryRow(ctx, countAdminsQuery, clubID, shownOnly).Scan(&n); err != nil {
		return 0, errs.Wrap("r.pool.QueryRow", err)
	}

	return n, nil
}

// listMembersQuery: with $4, only those who run the club; with $5, not
// those of them it hides; $6, if not NULL, picks those who only might
// come to the event or those who are sure to.
const listMembersQuery = `
	SELECT
		id,
		user_id,
		role,
		title,
		hidden,
		maybe
	FROM club_members
	WHERE club_id = $1
		AND id < $2
		AND (NOT $4 OR role IN ('admin', 'owner'))
		AND NOT ($5 AND hidden)
		AND ($6::boolean IS NULL OR maybe = $6)
	ORDER BY id DESC
	LIMIT $3
`

func (r *MemberRepository) List(ctx context.Context, clubID, before uuid.UUID, limit int, filter service.MemberFilter) ([]service.Member, error) {
	managers := filter == service.Managers || filter == service.ShownManagers

	var maybe *bool
	if filter == service.SureComers || filter == service.MaybeComers {
		maybe = new(bool)
		*maybe = filter == service.MaybeComers
	}

	rows, err := r.pool.Query(ctx, listMembersQuery, clubID, cursor(before), limit,
		managers, filter == service.ShownManagers, maybe)
	if err != nil {
		return nil, errs.Wrap("r.pool.Query", err)
	}
	defer rows.Close()

	var found []service.Member
	for rows.Next() {
		var (
			m           service.Member
			role, title string
		)
		if err = rows.Scan(&m.ID, &m.UserID, &role, &title, &m.Hidden, &m.Maybe); err != nil {
			return nil, errs.Wrap("rows.Scan", err)
		}
		m.Role, m.Title = club.Role(role), club.Title(title)
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

// listUserClubsQuery: with $4, only the clubs the user runs; with $5,
// only those of the kind; with $6, only the events over ('past') or not
// ('upcoming') at $7 — an event is over when it has ended or, having no
// end, begun (club.Schedule.Over).
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
		c.topics_above_wall,
		c.backdrop_left,
		c.backdrop_right,
		c.kind,
		c.starts_at,
		c.ends_at,
		c.maybe_count
	FROM club_members m
	JOIN clubs c ON c.id = m.club_id
	WHERE m.user_id = $1
		AND m.id < $2
		AND (NOT $4 OR m.role IN ('admin', 'owner'))
		AND ($5 = '' OR c.kind = $5)
		AND ($6 = '' OR (c.starts_at IS NOT NULL AND (COALESCE(c.ends_at, c.starts_at) <= $7) = ($6 = 'past')))
	ORDER BY m.id DESC
	LIMIT $3
`

func (r *MemberRepository) ListClubs(ctx context.Context, userID, before uuid.UUID, limit int, filter service.ClubFilter) ([]service.Membership, error) {
	rows, err := r.pool.Query(ctx, listUserClubsQuery, userID, cursor(before), limit, filter.Managed, filter.Kind.String(),
		filter.Period.String(), filter.At)
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
	FROM club_members m
	JOIN clubs c ON c.id = m.club_id
	WHERE m.user_id = $1
		AND (NOT $2 OR m.role IN ('admin', 'owner'))
		AND ($3 = '' OR c.kind = $3)
		AND ($4 = '' OR (c.starts_at IS NOT NULL AND (COALESCE(c.ends_at, c.starts_at) <= $5) = ($4 = 'past')))
`

func (r *MemberRepository) CountClubs(ctx context.Context, userID uuid.UUID, filter service.ClubFilter) (int, error) {
	var n int

	if err := r.pool.QueryRow(ctx, countUserClubsQuery, userID, filter.Managed, filter.Kind.String(),
		filter.Period.String(), filter.At).Scan(&n); err != nil {
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
		c.topics_above_wall,
		c.backdrop_left,
		c.backdrop_right,
		c.kind,
		c.starts_at,
		c.ends_at,
		c.maybe_count
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

const (
	// deleteUserMembershipsQuery takes the user out of every club, the
	// owned ones too, with the counts in the same statement, and tells
	// which clubs they owned.
	deleteUserMembershipsQuery = `
		WITH gone AS (
			DELETE FROM club_members
			WHERE user_id = $1
			RETURNING club_id, maybe, role
		), counted AS (
			UPDATE clubs c
			SET member_count = c.member_count - 1,
				maybe_count = c.maybe_count - gone.maybe::integer
			FROM gone
			WHERE c.id = gone.club_id
		)
		SELECT club_id
		FROM gone
		WHERE role = 'owner'
	`
	// inheritClubsQuery gives each club left without an owner to who has
	// been in it longest: of its admins or, when it has none, of all its
	// members. An owner is sure to come to an event.
	inheritClubsQuery = `
		WITH heirs AS (
			SELECT DISTINCT ON (club_id)
				id,
				club_id,
				maybe
			FROM club_members
			WHERE club_id = ANY($1)
			ORDER BY club_id, (role = 'admin') DESC, id
		), promoted AS (
			UPDATE club_members m
			SET role = 'owner',
				maybe = false
			FROM heirs
			WHERE m.id = heirs.id
			RETURNING m.id
		)
		UPDATE clubs c
		SET maybe_count = c.maybe_count - 1
		FROM heirs
		JOIN promoted ON promoted.id = heirs.id
		WHERE c.id = heirs.club_id
			AND heirs.maybe
	`
)

// DeleteByUser runs in a transaction: one statement would not see the
// owner gone when it names the next one, and the unique index allows a
// club one owner.
func (r *MemberRepository) DeleteByUser(ctx context.Context, userID uuid.UUID) error {
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, deleteUserMembershipsQuery, userID)
		if err != nil {
			return errs.Wrap("tx.Query", err)
		}

		orphaned, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
		if err != nil {
			return errs.Wrap("pgx.CollectRows", err)
		}

		if len(orphaned) == 0 {
			return nil
		}

		if _, err = tx.Exec(ctx, inheritClubsQuery, orphaned); err != nil {
			return errs.Wrap("tx.Exec", err)
		}

		return nil
	})

	return errs.Wrap("pgx.BeginFunc", err)
}
