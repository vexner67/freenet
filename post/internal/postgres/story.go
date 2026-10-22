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
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
)

type StoryRepository struct {
	pool *pgxpool.Pool
}

func NewStoryRepository(pool *pgxpool.Pool) *StoryRepository {
	return &StoryRepository{
		pool: pool,
	}
}

// conn is the transaction of ctx (service.TxManager), or the pool outside
// one.
func (r *StoryRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

// saveStoryQuery inserts nothing when $7 stories of the author told after
// $6 are live already. Two at once may both pass: the cap is against
// piling up, not exact.
const saveStoryQuery = `
	INSERT INTO stories (
		id,
		author_id,
		photo_id,
		text,
		created_at
	)
	SELECT $1, $2, $3, $4, $5
	WHERE (
		SELECT count(*)
		FROM stories
		WHERE author_id = $2
			AND created_at > $6
	) < $7
`

func (r *StoryRepository) Save(ctx context.Context, s *post.Story) error {
	tag, err := r.conn(ctx).Exec(ctx, saveStoryQuery,
		s.ID(), s.AuthorID(), s.PhotoID(), s.Text().String(), s.CreatedAt(), post.LiveSince(s.CreatedAt()), post.MaxStories)
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return post.ErrTooManyStories
	}

	return nil
}

const getStoryQuery = `
	SELECT
		id,
		author_id,
		photo_id,
		text,
		created_at
	FROM stories
	WHERE id = $1
`

func (r *StoryRepository) Get(ctx context.Context, id uuid.UUID) (*post.Story, error) {
	s, err := scanStory(r.conn(ctx).QueryRow(ctx, getStoryQuery, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, post.ErrStoryNotFound
	}
	if err != nil {
		return nil, errs.Wrap("scanStory", err)
	}

	return s, nil
}

const deleteStoryQuery = `
	DELETE FROM stories
	WHERE id = $1
`

func (r *StoryRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.conn(ctx).Exec(ctx, deleteStoryQuery, id)
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return post.ErrStoryNotFound
	}

	return nil
}

// shownStoriesWhere: live since $1, not of a deleted page, of a wall not
// closed to the viewer $2, not of the people hidden $3.
const shownStoriesWhere = `
	WHERE s.created_at > $1
		AND NOT user_hidden(s.author_id)
		AND NOT wall_closed_to(s.author_id, $2)
		AND NOT (s.author_id = ANY($3))
`

// authorStoriesQuery: the stories of the author $4.
const authorStoriesQuery = `
	SELECT
		s.id,
		s.author_id,
		s.photo_id,
		s.text,
		s.created_at
	FROM stories s
	` + shownStoriesWhere + `
		AND s.author_id = $4
	ORDER BY s.created_at, s.id
`

// feedStoriesQuery: of the viewer and of the people they follow, but those
// ignored $4.
const feedStoriesQuery = `
	SELECT
		s.id,
		s.author_id,
		s.photo_id,
		s.text,
		s.created_at
	FROM stories s
	` + shownStoriesWhere + `
		AND (
			s.author_id = $2
			OR s.author_id IN (
				SELECT fs.source_id
				FROM feed_sources fs
				WHERE fs.reader_id = $2
					AND NOT fs.club
					AND NOT (fs.source_id = ANY($4))
			)
		)
	ORDER BY s.created_at, s.id
`

func (r *StoryRepository) Shown(ctx context.Context, authorID uuid.UUID, audience service.StoryAudience, now time.Time) ([]*post.Story, error) {
	since, viewer, hidden := post.LiveSince(now), reader(audience.Viewer), ids(audience.Hidden)
	if authorID != uuid.Nil {
		return r.query(ctx, authorStoriesQuery, since, viewer, hidden, authorID)
	}

	return r.query(ctx, feedStoriesQuery, since, viewer, hidden, ids(audience.Ignored))
}

const seenStoryQuery = `
	SELECT
		s.id,
		s.author_id,
		s.photo_id,
		s.text,
		s.created_at
	FROM stories s
	` + shownStoriesWhere + `
		AND s.id = $4
`

func (r *StoryRepository) Seen(ctx context.Context, id, viewerID uuid.UUID, now time.Time) (*post.Story, error) {
	s, err := scanStory(r.conn(ctx).QueryRow(ctx, seenStoryQuery, post.LiveSince(now), reader(viewerID), []uuid.UUID{}, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, post.ErrStoryNotFound
	}
	if err != nil {
		return nil, errs.Wrap("scanStory", err)
	}

	return s, nil
}

const expiredStoriesQuery = `
	SELECT
		id,
		author_id,
		photo_id,
		text,
		created_at
	FROM stories
	WHERE created_at <= $1
	ORDER BY created_at, id
	LIMIT $2
`

func (r *StoryRepository) Expired(ctx context.Context, now time.Time, limit int) ([]*post.Story, error) {
	return r.query(ctx, expiredStoriesQuery, post.LiveSince(now), limit)
}

const (
	deleteUserStoriesQuery = `
		DELETE FROM stories
		WHERE author_id = $1
	`
	deleteUserStoryViewsQuery = `
		DELETE FROM story_views
		WHERE viewer_id = $1
	`
	deleteUserStoryLikesQuery = `
		DELETE FROM story_likes
		WHERE user_id = $1
	`
	deleteUserStoryCommentsQuery = `
		DELETE FROM story_comments
		WHERE author_id = $1
	`
)

// DeleteByUser deletes the stories the user told and their views, likes
// and comments of others'; media erases the photos with the rest of the
// user.
func (r *StoryRepository) DeleteByUser(ctx context.Context, userID uuid.UUID) error {
	for _, query := range []string{deleteUserStoriesQuery, deleteUserStoryViewsQuery, deleteUserStoryLikesQuery, deleteUserStoryCommentsQuery} {
		if _, err := r.conn(ctx).Exec(ctx, query, userID); err != nil {
			return errs.Wrap("r.conn.Exec", err)
		}
	}

	return nil
}

func (r *StoryRepository) query(ctx context.Context, query string, args ...any) ([]*post.Story, error) {
	rows, err := r.conn(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}
	defer rows.Close()

	var found []*post.Story
	for rows.Next() {
		s, err := scanStory(rows)
		if err != nil {
			return nil, errs.Wrap("scanStory", err)
		}
		found = append(found, s)
	}

	if err = rows.Err(); err != nil {
		return nil, errs.Wrap("rows.Err", err)
	}

	return found, nil
}

func scanStory(row pgx.Row) (*post.Story, error) {
	var (
		id, authorID, photoID uuid.UUID
		text                  string
		createdAt             time.Time
	)

	if err := row.Scan(&id, &authorID, &photoID, &text, &createdAt); err != nil {
		return nil, err
	}

	// Data comes from our own table and was validated on write.
	return post.RestoreStory(id, authorID, photoID, post.StoryText(text), createdAt), nil
}

type StoryViewRepository struct {
	pool *pgxpool.Pool
}

func NewStoryViewRepository(pool *pgxpool.Pool) *StoryViewRepository {
	return &StoryViewRepository{
		pool: pool,
	}
}

const viewStoryQuery = `
	INSERT INTO story_views (
		story_id,
		viewer_id,
		viewed_at,
		hidden
	)
	VALUES ($1, $2, $3, $4)
	ON CONFLICT (story_id, viewer_id) DO NOTHING
`

func (r *StoryViewRepository) View(ctx context.Context, storyID, viewerID uuid.UUID, at time.Time, hidden bool) error {
	_, err := r.pool.Exec(ctx, viewStoryQuery, storyID, viewerID, at, hidden)
	if isForeignKeyViolation(err) {
		return post.ErrStoryNotFound
	}
	if err != nil {
		return errs.Wrap("r.pool.Exec", err)
	}

	return nil
}

const viewedStoriesQuery = `
	SELECT story_id
	FROM story_views
	WHERE viewer_id = $1
		AND story_id = ANY($2)
`

func (r *StoryViewRepository) Viewed(ctx context.Context, viewerID uuid.UUID, storyIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	rows, err := r.pool.Query(ctx, viewedStoriesQuery, viewerID, storyIDs)
	if err != nil {
		return nil, errs.Wrap("r.pool.Query", err)
	}

	found, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, errs.Wrap("pgx.CollectRows", err)
	}

	viewed := make(map[uuid.UUID]bool, len(found))
	for _, id := range found {
		viewed[id] = true
	}

	return viewed, nil
}

// countedViewsWhere: the views of others than the author, whose page is
// not deleted, and not made unseen (Freenet Prime).
const countedViewsWhere = `
	WHERE v.viewer_id <> s.author_id
		AND NOT v.hidden
		AND NOT user_hidden(v.viewer_id)
`

// storyViewersQuery pages after ($2, $3), when $2 is not NULL.
const storyViewersQuery = `
	SELECT
		v.viewer_id,
		v.viewed_at
	FROM story_views v
	JOIN stories s ON s.id = v.story_id
	` + countedViewsWhere + `
		AND v.story_id = $1
		AND ($2::timestamptz IS NULL OR (v.viewed_at, v.viewer_id) < ($2, $3::uuid))
	ORDER BY v.viewed_at DESC, v.viewer_id DESC
	LIMIT $4
`

func (r *StoryViewRepository) Viewers(ctx context.Context, storyID uuid.UUID, after *service.ViewCursor, limit int) ([]service.StoryView, error) {
	var (
		at     *time.Time
		viewer uuid.UUID
	)
	if after != nil {
		at, viewer = &after.ViewedAt, after.ViewerID
	}

	rows, err := r.pool.Query(ctx, storyViewersQuery, storyID, at, viewer, limit)
	if err != nil {
		return nil, errs.Wrap("r.pool.Query", err)
	}

	views, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (service.StoryView, error) {
		var v service.StoryView
		err := row.Scan(&v.ViewerID, &v.ViewedAt)
		v.ViewedAt = v.ViewedAt.UTC()

		return v, err
	})
	if err != nil {
		return nil, errs.Wrap("pgx.CollectRows", err)
	}

	return views, nil
}

const countStoryViewsQuery = `
	SELECT
		v.story_id,
		count(*)
	FROM story_views v
	JOIN stories s ON s.id = v.story_id
	` + countedViewsWhere + `
		AND v.story_id = ANY($1)
	GROUP BY v.story_id
`

func (r *StoryViewRepository) Count(ctx context.Context, storyIDs []uuid.UUID) (map[uuid.UUID]int, error) {
	rows, err := r.pool.Query(ctx, countStoryViewsQuery, storyIDs)
	if err != nil {
		return nil, errs.Wrap("r.pool.Query", err)
	}
	defer rows.Close()

	counts := make(map[uuid.UUID]int)
	for rows.Next() {
		var (
			id    uuid.UUID
			count int
		)
		if err = rows.Scan(&id, &count); err != nil {
			return nil, errs.Wrap("rows.Scan", err)
		}
		counts[id] = count
	}

	if err = rows.Err(); err != nil {
		return nil, errs.Wrap("rows.Err", err)
	}

	return counts, nil
}
