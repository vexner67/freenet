package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/post/internal/post"
)

// PlaylistRepository keeps the playlists of people.
type PlaylistRepository struct {
	pool *pgxpool.Pool
}

func NewPlaylistRepository(pool *pgxpool.Pool) *PlaylistRepository {
	return &PlaylistRepository{pool: pool}
}

func (r *PlaylistRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

const savePlaylistQuery = `
	INSERT INTO playlists (
		id,
		owner_id,
		title,
		created_at
	)
	VALUES ($1, $2, $3, $4)
`

func (r *PlaylistRepository) Save(ctx context.Context, p *post.Playlist) error {
	if _, err := r.conn(ctx).Exec(ctx, savePlaylistQuery, p.ID(), p.OwnerID(), p.Title().String(), p.CreatedAt()); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

const getPlaylistQuery = `
	SELECT
		id,
		owner_id,
		title,
		created_at
	FROM playlists
	WHERE id = $1
`

func (r *PlaylistRepository) Get(ctx context.Context, id uuid.UUID) (*post.Playlist, error) {
	p, err := scanPlaylist(r.conn(ctx).QueryRow(ctx, getPlaylistQuery, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, post.ErrPlaylistNotFound
	}
	if err != nil {
		return nil, errs.Wrap("scanPlaylist", err)
	}

	return p, nil
}

const updatePlaylistQuery = `
	UPDATE playlists
	SET title = $2
	WHERE id = $1
`

func (r *PlaylistRepository) Update(ctx context.Context, p *post.Playlist) error {
	tag, err := r.conn(ctx).Exec(ctx, updatePlaylistQuery, p.ID(), p.Title().String())
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return post.ErrPlaylistNotFound
	}

	return nil
}

// deletePlaylistQuery takes the list of the playlist along: its songs are
// filed under its ID, which no foreign key ties.
const deletePlaylistQuery = `
	WITH entries AS (
		DELETE FROM audio_entries
		WHERE list_id = $1
	)
	DELETE FROM playlists
	WHERE id = $1
`

func (r *PlaylistRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.conn(ctx).Exec(ctx, deletePlaylistQuery, id)
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return post.ErrPlaylistNotFound
	}

	return nil
}

const listPlaylistsQuery = `
	SELECT
		id,
		owner_id,
		title,
		created_at
	FROM playlists
	WHERE owner_id = $1
		AND id < $2
	ORDER BY id DESC
	LIMIT $3
`

func (r *PlaylistRepository) List(ctx context.Context, ownerID, before uuid.UUID, limit int) ([]*post.Playlist, error) {
	if before == uuid.Nil {
		before = newestCursor
	}

	rows, err := r.conn(ctx).Query(ctx, listPlaylistsQuery, ownerID, before, limit)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	playlists, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*post.Playlist, error) {
		return scanPlaylist(row)
	})
	if err != nil {
		return nil, errs.Wrap("pgx.CollectRows", err)
	}

	return playlists, nil
}

const countPlaylistsQuery = `
	SELECT count(*)
	FROM playlists
	WHERE owner_id = $1
`

func (r *PlaylistRepository) Count(ctx context.Context, ownerID uuid.UUID) (int, error) {
	var n int

	if err := r.conn(ctx).QueryRow(ctx, countPlaylistsQuery, ownerID).Scan(&n); err != nil {
		return 0, errs.Wrap("r.conn.QueryRow", err)
	}

	return n, nil
}

func scanPlaylist(row pgx.Row) (*post.Playlist, error) {
	var (
		s     post.StoredPlaylist
		title string
	)

	if err := row.Scan(&s.ID, &s.OwnerID, &title, &s.CreatedAt); err != nil {
		return nil, err
	}

	// Data comes from our own table and was validated on write.
	s.Title = post.PlaylistTitle(title)
	s.CreatedAt = s.CreatedAt.UTC()

	return post.RestorePlaylist(s), nil
}
