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
	"github.com/vexner67/freenet/room/internal/room"
	"github.com/vexner67/freenet/room/internal/service"
)

type RoomRepository struct {
	pool        *pgxpool.Pool
	readTimeout time.Duration
}

func NewRoomRepository(pool *pgxpool.Pool, readTimeout time.Duration) *RoomRepository {
	return &RoomRepository{pool: pool, readTimeout: readTimeout}
}

const saveRoomQuery = `
 INSERT INTO rooms (
  id,
  owner_id,
  club_id,
  title,
  kind,
  access,
  control,
  permanent,
  created_at,
  closed_at
 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
 ON CONFLICT (id) DO UPDATE SET
  title=EXCLUDED.title,
  kind=EXCLUDED.kind,
  access=EXCLUDED.access,
  control=EXCLUDED.control,
  closed_at=COALESCE(rooms.closed_at,EXCLUDED.closed_at)
 WHERE rooms.closed_at IS NULL OR EXCLUDED.closed_at IS NOT NULL
`

func (r *RoomRepository) Save(ctx context.Context, value *room.Room) error {
	var clubID *uuid.UUID
	if id := value.ClubID(); id != uuid.Nil {
		clubID = &id
	}
	var closedAt *time.Time
	if value.Closed() {
		closed := value.ClosedAt()
		closedAt = &closed
	}
	tag, err := pgpool.Conn(ctx, r.pool).Exec(ctx, saveRoomQuery, value.ID(), value.OwnerID(), clubID, value.Title().String(), value.Kind().String(), value.Access().String(), value.Control().String(), value.Permanent(), value.CreatedAt(), closedAt)
	if err != nil {
		return errs.Wrap("pgpool.Conn.Exec", err)
	}
	if tag.RowsAffected() == 0 {
		return room.ErrClosed
	}
	return nil
}

const getRoomQuery = `
 SELECT
  id,
  owner_id,
  club_id,
  title,
  kind,
  access,
  control,
  permanent,
  created_at,
  closed_at
 FROM rooms WHERE id=$1
`

func (r *RoomRepository) Get(ctx context.Context, id uuid.UUID) (*room.Room, error) {
	ctx, cancel := context.WithTimeout(ctx, r.readTimeout)
	defer cancel()
	result, err := scanRoom(pgpool.Conn(ctx, r.pool).QueryRow(ctx, getRoomQuery, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, room.ErrNotFound
	}
	if err != nil {
		return nil, errs.Wrap("scanRoom", err)
	}
	return result, nil
}

const listRoomsQuery = `
 SELECT
  id,
  owner_id,
  club_id,
  title,
  kind,
  access,
  control,
  permanent,
  created_at,
  closed_at
 FROM rooms
 WHERE ($7 OR closed_at IS NULL)
  AND ($1::uuid IS NULL OR owner_id=$1)
  AND (NOT $2 OR club_id IS NULL)
  AND ($3::uuid IS NULL OR club_id=$3)
  AND ($4::uuid IS NULL OR id<$4)
  AND (NOT $5 OR permanent)
 ORDER BY id DESC
 LIMIT $6
`

func nullableID(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}
func (r *RoomRepository) List(ctx context.Context, f service.Filter) ([]*room.Room, error) {
	ctx, cancel := context.WithTimeout(ctx, r.readTimeout)
	defer cancel()
	rows, err := pgpool.Conn(ctx, r.pool).Query(ctx, listRoomsQuery, nullableID(f.OwnerID), f.PersonalOnly, nullableID(f.ClubID), nullableID(f.Cursor), f.PermanentOnly, f.Limit, f.IncludeClosed)
	if err != nil {
		return nil, errs.Wrap("pgpool.Conn.Query", err)
	}
	defer rows.Close()
	result := make([]*room.Room, 0)
	for rows.Next() {
		value, err := scanRoom(rows)
		if err != nil {
			return nil, errs.Wrap("scanRoom", err)
		}
		result = append(result, value)
	}
	if err = rows.Err(); err != nil {
		return nil, errs.Wrap("rows.Err", err)
	}
	return result, nil
}

type scanner interface{ Scan(...any) error }

func scanRoom(row scanner) (*room.Room, error) {
	var id, owner uuid.UUID
	var club *uuid.UUID
	var title, kind, access, control string
	var permanent bool
	var created time.Time
	var closed *time.Time
	if err := row.Scan(&id, &owner, &club, &title, &kind, &access, &control, &permanent, &created, &closed); err != nil {
		return nil, errs.Wrap("row.Scan", err)
	}
	in := room.Input{OwnerID: owner, Title: room.Title(title), Kind: room.Kind(kind), Access: room.Access(access), Control: room.Control(control), Permanent: permanent}
	if club != nil {
		in.ClubID = *club
	}
	var closedAt time.Time
	if closed != nil {
		closedAt = *closed
	}
	return room.Restore(id, in, created, closedAt), nil
}

const deleteRoomQuery = `DELETE FROM rooms WHERE id=$1`

func (r *RoomRepository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := pgpool.Conn(ctx, r.pool).Exec(ctx, deleteRoomQuery, id)
	return errs.Wrap("pgpool.Conn.Exec", err)
}
