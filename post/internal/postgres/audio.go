package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
)

// AudioRepository keeps the songs of "Аудиозаписи", the lists they are in
// and what pages say of music: one repository, as the tables share their
// foreign keys.
type AudioRepository struct {
	pool *pgxpool.Pool
}

func NewAudioRepository(pool *pgxpool.Pool) *AudioRepository {
	return &AudioRepository{pool: pool}
}

// conn is the transaction of ctx (service.TxManager), or the pool outside
// one.
func (r *AudioRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

const saveAudioQuery = `
	INSERT INTO audios (
		id,
		owner_id,
		kind,
		link,
		cover,
		artist,
		title,
		duration,
		created_at
	)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
`

func (r *AudioRepository) Save(ctx context.Context, a *post.Audio) error {
	if _, err := r.conn(ctx).Exec(ctx, saveAudioQuery, audioArgs(a)...); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

func audioArgs(a *post.Audio) []any {
	s := a.Source()

	return []any{
		a.ID(), a.OwnerID(), s.Kind().String(), s.Link(), s.Cover(),
		a.Artist().String(), a.Title().String(), a.Duration().Seconds(), a.CreatedAt(),
	}
}

// saveTrackQuery adds a track unless someone added its address first; the
// one kept comes back either way.
const saveTrackQuery = `
	WITH added AS (
		INSERT INTO audios (
			id,
			owner_id,
			kind,
			link,
			cover,
			artist,
			title,
			duration,
			created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (link) WHERE link <> '' DO NOTHING
		RETURNING
			id,
			owner_id,
			kind,
			link,
			cover,
			artist,
			title,
			duration,
			created_at
	)
	SELECT
		id,
		owner_id,
		kind,
		link,
		cover,
		artist,
		title,
		duration,
		created_at
	FROM added
	UNION ALL
	SELECT
		id,
		owner_id,
		kind,
		link,
		cover,
		artist,
		title,
		duration,
		created_at
	FROM audios
	WHERE link = $4
		AND NOT EXISTS (SELECT 1 FROM added)
`

func (r *AudioRepository) SaveTrack(ctx context.Context, a *post.Audio) (*post.Audio, error) {
	kept, err := scanAudio(r.conn(ctx).QueryRow(ctx, saveTrackQuery, audioArgs(a)...))
	if err != nil {
		return nil, errs.Wrap("scanAudio", err)
	}

	return kept, nil
}

const getAudioQuery = `
	SELECT
		id,
		owner_id,
		kind,
		link,
		cover,
		artist,
		title,
		duration,
		created_at
	FROM audios
	WHERE id = $1
`

func (r *AudioRepository) Get(ctx context.Context, id uuid.UUID) (*post.Audio, error) {
	a, err := scanAudio(r.conn(ctx).QueryRow(ctx, getAudioQuery, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, post.ErrAudioNotFound
	}
	if err != nil {
		return nil, errs.Wrap("scanAudio", err)
	}

	return a, nil
}

const getAudiosQuery = `
	SELECT
		id,
		owner_id,
		kind,
		link,
		cover,
		artist,
		title,
		duration,
		created_at
	FROM audios
	WHERE id = ANY($1)
`

func (r *AudioRepository) GetMany(ctx context.Context, ids []uuid.UUID) ([]*post.Audio, error) {
	rows, err := r.conn(ctx).Query(ctx, getAudiosQuery, ids)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	return collectAudios(rows)
}

const updateAudioQuery = `
	UPDATE audios
	SET artist = $2,
		title = $3
	WHERE id = $1
`

func (r *AudioRepository) Update(ctx context.Context, a *post.Audio) error {
	tag, err := r.conn(ctx).Exec(ctx, updateAudioQuery, a.ID(), a.Artist().String(), a.Title().String())
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return post.ErrAudioNotFound
	}

	return nil
}

const deleteAudioQuery = `
	DELETE FROM audios
	WHERE id = $1
`

func (r *AudioRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.conn(ctx).Exec(ctx, deleteAudioQuery, id)
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return post.ErrAudioNotFound
	}

	return nil
}

// searchAudiosQuery finds the words anywhere in "artist title", case
// aside; $2 escapes the wildcards of LIKE.
const searchAudiosQuery = `
	SELECT
		id,
		owner_id,
		kind,
		link,
		cover,
		artist,
		title,
		duration,
		created_at
	FROM audios
	WHERE id < $1
		AND (artist || ' ' || title) ILIKE '%' || $2 || '%'
	ORDER BY id DESC
	LIMIT $3
`

func (r *AudioRepository) Search(ctx context.Context, query string, before uuid.UUID, limit int) ([]*post.Audio, error) {
	if before == uuid.Nil {
		before = newestCursor
	}

	rows, err := r.conn(ctx).Query(ctx, searchAudiosQuery, before, likeEscaper.Replace(query), limit)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	return collectAudios(rows)
}

// likeEscaper keeps the % and _ people type from matching anything.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

const deleteUserAudioQuery = `
	WITH uploads AS (
		DELETE FROM audios
		WHERE owner_id = $1
			AND kind = 'file'
	), lists AS (
		DELETE FROM playlists
		WHERE owner_id = $1
		RETURNING id
	), entries AS (
		DELETE FROM audio_entries
		WHERE list_id = $1
			OR list_id IN (SELECT id FROM lists)
	)
	DELETE FROM music_profiles
	WHERE user_id = $1
`

// DeleteByUser deletes the songs a user uploaded, their library and
// playlists and their page's music; tracks of other sites stay.
func (r *AudioRepository) DeleteByUser(ctx context.Context, userID uuid.UUID) error {
	if _, err := r.conn(ctx).Exec(ctx, deleteUserAudioQuery, userID); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

func collectAudios(rows pgx.Rows) ([]*post.Audio, error) {
	audios, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*post.Audio, error) {
		return scanAudio(row)
	})
	if err != nil {
		return nil, errs.Wrap("pgx.CollectRows", err)
	}

	return audios, nil
}

func scanAudio(row pgx.Row) (*post.Audio, error) {
	var (
		s                                post.StoredAudio
		kind, link, cover, artist, title string
		duration                         int
	)

	if err := row.Scan(&s.ID, &s.OwnerID, &kind, &link, &cover, &artist, &title, &duration, &s.CreatedAt); err != nil {
		return nil, err
	}

	// Data comes from our own table and was validated on write.
	s.Source = post.RestoreAudioSource(post.AudioKind(kind), link, cover)
	s.Artist, s.Title, s.Duration = post.AudioName(artist), post.AudioName(title), post.AudioDuration(duration)
	s.CreatedAt = s.CreatedAt.UTC()

	return post.RestoreAudio(s), nil
}

// AudioLists are the lists of songs over the same tables.
func (r *AudioRepository) AudioLists() *AudioListRepository {
	return &AudioListRepository{r: r}
}

// Music is what pages say of music over the same tables.
func (r *AudioRepository) Music() *MusicRepository {
	return &MusicRepository{r: r}
}

type AudioListRepository struct{ r *AudioRepository }

const addAudioEntryQuery = `
	INSERT INTO audio_entries (
		id,
		list_id,
		audio_id
	)
	VALUES ($1, $2, $3)
	ON CONFLICT (list_id, audio_id) DO NOTHING
`

func (l *AudioListRepository) Add(ctx context.Context, listID, audioID uuid.UUID) error {
	id, err := uuid.NewV7()
	if err != nil {
		return errs.Wrap("uuid.NewV7", err)
	}

	_, err = l.r.conn(ctx).Exec(ctx, addAudioEntryQuery, id, listID, audioID)
	if isForeignKeyViolation(err) {
		return post.ErrAudioNotFound
	}

	return errs.Wrap("l.r.conn.Exec", err)
}

const removeAudioEntryQuery = `
	DELETE FROM audio_entries
	WHERE list_id = $1
		AND audio_id = $2
`

func (l *AudioListRepository) Remove(ctx context.Context, listID, audioID uuid.UUID) error {
	_, err := l.r.conn(ctx).Exec(ctx, removeAudioEntryQuery, listID, audioID)

	return errs.Wrap("l.r.conn.Exec", err)
}

const listAudioEntriesQuery = `
	SELECT
		e.id,
		a.id,
		a.owner_id,
		a.kind,
		a.link,
		a.cover,
		a.artist,
		a.title,
		a.duration,
		a.created_at
	FROM audio_entries e
	JOIN audios a ON a.id = e.audio_id
	WHERE e.list_id = $1
		AND e.id < $2
	ORDER BY e.id DESC
	LIMIT $3
`

func (l *AudioListRepository) Entries(ctx context.Context, listID, before uuid.UUID, limit int) ([]service.AudioEntry, error) {
	if before == uuid.Nil {
		before = newestCursor
	}

	rows, err := l.r.conn(ctx).Query(ctx, listAudioEntriesQuery, listID, before, limit)
	if err != nil {
		return nil, errs.Wrap("l.r.conn.Query", err)
	}

	entries, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (service.AudioEntry, error) {
		var e service.AudioEntry

		a, err := scanAudio(entryRow{row: row, id: &e.ID})
		e.Audio = a

		return e, err
	})
	if err != nil {
		return nil, errs.Wrap("pgx.CollectRows", err)
	}

	return entries, nil
}

// entryRow reads the ID of an entry before the song it holds.
type entryRow struct {
	row pgx.Row
	id  *uuid.UUID
}

func (e entryRow) Scan(dest ...any) error {
	return e.row.Scan(append([]any{e.id}, dest...)...)
}

const countAudioEntriesQuery = `
	SELECT count(*)
	FROM audio_entries
	WHERE list_id = $1
`

func (l *AudioListRepository) Count(ctx context.Context, listID uuid.UUID) (int, error) {
	var n int

	if err := l.r.conn(ctx).QueryRow(ctx, countAudioEntriesQuery, listID).Scan(&n); err != nil {
		return 0, errs.Wrap("l.r.conn.QueryRow", err)
	}

	return n, nil
}

const containsAudiosQuery = `
	SELECT audio_id
	FROM audio_entries
	WHERE list_id = $1
		AND audio_id = ANY($2)
`

func (l *AudioListRepository) Contains(ctx context.Context, listID uuid.UUID, audioIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	rows, err := l.r.conn(ctx).Query(ctx, containsAudiosQuery, listID, audioIDs)
	if err != nil {
		return nil, errs.Wrap("l.r.conn.Query", err)
	}

	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, errs.Wrap("pgx.CollectRows", err)
	}

	has := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		has[id] = true
	}

	return has, nil
}

type MusicRepository struct{ r *AudioRepository }

const setListeningQuery = `
	INSERT INTO music_profiles (
		user_id,
		listening_id,
		listening_at
	)
	VALUES ($1, $2, $3)
	ON CONFLICT (user_id) DO UPDATE
	SET listening_id = EXCLUDED.listening_id,
		listening_at = EXCLUDED.listening_at
`

func (m *MusicRepository) SetListening(ctx context.Context, userID, audioID uuid.UUID, at time.Time) error {
	var when *time.Time
	if audioID != uuid.Nil {
		when = &at
	}

	return m.exec(ctx, setListeningQuery, userID, nullID(audioID), when)
}

const setPageAudioQuery = `
	INSERT INTO music_profiles (
		user_id,
		page_audio_id
	)
	VALUES ($1, $2)
	ON CONFLICT (user_id) DO UPDATE
	SET page_audio_id = EXCLUDED.page_audio_id
`

func (m *MusicRepository) SetPageAudio(ctx context.Context, userID, audioID uuid.UUID) error {
	return m.exec(ctx, setPageAudioQuery, userID, nullID(audioID))
}

func (m *MusicRepository) exec(ctx context.Context, query string, args ...any) error {
	_, err := m.r.conn(ctx).Exec(ctx, query, args...)
	if isForeignKeyViolation(err) {
		return post.ErrAudioNotFound
	}

	return errs.Wrap("m.r.conn.Exec", err)
}

const getMusicProfileQuery = `
	SELECT
		listening_id,
		listening_at,
		page_audio_id
	FROM music_profiles
	WHERE user_id = $1
`

func (m *MusicRepository) Profile(ctx context.Context, userID uuid.UUID) (service.MusicProfile, error) {
	var (
		listening, page *uuid.UUID
		at              *time.Time
		p               service.MusicProfile
	)

	err := m.r.conn(ctx).QueryRow(ctx, getMusicProfileQuery, userID).Scan(&listening, &at, &page)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, nil
	}
	if err != nil {
		return p, errs.Wrap("m.r.conn.QueryRow", err)
	}

	if listening != nil && at != nil {
		p.Listening, p.ListeningAt = *listening, at.UTC()
	}
	if page != nil {
		p.PageAudio = *page
	}

	return p, nil
}
