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

// TopicRepository keeps the topics of communities and the messages in
// them: it is both service.TopicRepository and
// service.TopicMessageRepository.
type TopicRepository struct {
	pool *pgxpool.Pool
}

func NewTopicRepository(pool *pgxpool.Pool) *TopicRepository {
	return &TopicRepository{
		pool: pool,
	}
}

// conn is the transaction of ctx (service.TxManager), or the pool outside
// one.
func (r *TopicRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

// Messages is the same repository as service.TopicMessageRepository.
func (r *TopicRepository) Messages() *TopicMessageRepository {
	return &TopicMessageRepository{r: r}
}

const saveTopicQuery = `
	INSERT INTO topics (
		id,
		club_id,
		author_id,
		title,
		from_wall,
		closed,
		pinned,
		restricted,
		created_at,
		updated_at
	)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
`

func (r *TopicRepository) Save(ctx context.Context, t *post.Topic) error {
	_, err := r.conn(ctx).Exec(ctx, saveTopicQuery, t.ID(), t.ClubID(), t.AuthorID(), t.Title().String(),
		t.FromWall(), t.Closed(), t.Pinned(), t.Restricted(), t.CreatedAt(), t.UpdatedAt())
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

const getTopicQuery = `
	SELECT
		id,
		club_id,
		author_id,
		title,
		from_wall,
		closed,
		pinned,
		restricted,
		created_at,
		updated_at,
		message_count
	FROM topics
	WHERE id = $1
`

func (r *TopicRepository) Get(ctx context.Context, id uuid.UUID) (*post.Topic, error) {
	t, err := scanTopic(r.conn(ctx).QueryRow(ctx, getTopicQuery, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, post.ErrTopicNotFound
	}
	if err != nil {
		return nil, errs.Wrap("scanTopic", err)
	}

	return t, nil
}

const updateTopicQuery = `
	UPDATE topics
	SET title = $2,
		closed = $3,
		pinned = $4,
		restricted = $5
	WHERE id = $1
`

func (r *TopicRepository) Update(ctx context.Context, t *post.Topic) error {
	tag, err := r.conn(ctx).Exec(ctx, updateTopicQuery, t.ID(), t.Title().String(), t.Closed(), t.Pinned(), t.Restricted())
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return post.ErrTopicNotFound
	}

	return nil
}

// lockTopicQuery makes a message written meanwhile wait (its statement
// updates the topic's row) and then fail, rather than go unseen with the
// topic and leave its photos behind.
const lockTopicQuery = `
	SELECT id
	FROM topics
	WHERE id = $1
	FOR UPDATE
`

// deleteTopicQuery returns the messages the topic takes with it: the
// statement sees them as they were before the delete.
const deleteTopicQuery = `
	WITH gone AS (
		DELETE FROM topics
		WHERE id = $1
		RETURNING id
	)
	SELECT
		id,
		topic_id,
		author_id,
		text,
		photo_ids,
		from_wall,
		created_at
	FROM topic_messages
	WHERE topic_id IN (SELECT id FROM gone)
`

// Delete runs in the transaction of ctx, which holds the lock it takes.
func (r *TopicRepository) Delete(ctx context.Context, id uuid.UUID) ([]*post.TopicMessage, error) {
	var locked uuid.UUID
	err := r.conn(ctx).QueryRow(ctx, lockTopicQuery, id).Scan(&locked)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, post.ErrTopicNotFound
	}
	if err != nil {
		return nil, errs.Wrap("r.conn.QueryRow", err)
	}

	rows, err := r.conn(ctx).Query(ctx, deleteTopicQuery, id)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	return collectTopicMessages(rows)
}

// listTopicsQuery: $2 is a piece of the title, empty for all; $3, $4 and $5 the
// cursor or NULLs for the first page. The row comparison follows the
// order, all descending.
const listTopicsQuery = `
	SELECT
		id,
		club_id,
		author_id,
		title,
		from_wall,
		closed,
		pinned,
		restricted,
		created_at,
		updated_at,
		message_count
	FROM topics
	WHERE club_id = $1
		AND ($2 = '' OR strpos(lower(title), lower($2)) > 0)
		AND ($3::boolean IS NULL OR (pinned, updated_at, id) < ($3::boolean, $4::timestamptz, $5::uuid))
	ORDER BY pinned DESC, updated_at DESC, id DESC
	LIMIT $6
`

func (r *TopicRepository) List(ctx context.Context, clubID uuid.UUID, query string, after *service.TopicCursor, limit int) ([]*post.Topic, error) {
	var (
		pinned    *bool
		updatedAt *time.Time
		id        *uuid.UUID
	)
	if after != nil {
		pinned, updatedAt, id = &after.Pinned, &after.UpdatedAt, &after.ID
	}

	rows, err := r.conn(ctx).Query(ctx, listTopicsQuery, clubID, query, pinned, updatedAt, id, limit)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}
	defer rows.Close()

	var found []*post.Topic
	for rows.Next() {
		t, err := scanTopic(rows)
		if err != nil {
			return nil, errs.Wrap("scanTopic", err)
		}
		found = append(found, t)
	}

	if err = rows.Err(); err != nil {
		return nil, errs.Wrap("rows.Err", err)
	}

	return found, nil
}

const countTopicsQuery = `
	SELECT count(*)
	FROM topics
	WHERE club_id = $1
		AND ($2 = '' OR strpos(lower(title), lower($2)) > 0)
`

func (r *TopicRepository) Count(ctx context.Context, clubID uuid.UUID, query string) (int, error) {
	var n int
	if err := r.conn(ctx).QueryRow(ctx, countTopicsQuery, clubID, query).Scan(&n); err != nil {
		return 0, errs.Wrap("r.conn.QueryRow", err)
	}

	return n, nil
}

// deleteUserTopicMessagesQuery deletes a user's messages and uncounts
// them in their topics in one statement.
const deleteUserTopicMessagesQuery = `
	WITH deleted AS (
		DELETE FROM topic_messages
		WHERE author_id = $1
		RETURNING topic_id
	)
	UPDATE topics t
	SET message_count = t.message_count - d.n
	FROM (
		SELECT topic_id, count(*) AS n
		FROM deleted
		GROUP BY topic_id
	) d
	WHERE t.id = d.topic_id
`

// deleteUserTopicsQuery deletes the topics a user started, returning the
// messages others wrote in them, which go too.
const deleteUserTopicsQuery = `
	WITH gone AS (
		DELETE FROM topics
		WHERE author_id = $1
		RETURNING id
	)
	SELECT
		id,
		topic_id,
		author_id,
		text,
		photo_ids,
		from_wall,
		created_at
	FROM topic_messages
	WHERE topic_id IN (SELECT id FROM gone)
		AND author_id <> $1
`

// DeleteByUser deletes the messages a user wrote and the topics they
// started, returning the messages of others the topics took with them.
// It runs in the transaction of ctx.
func (r *TopicRepository) DeleteByUser(ctx context.Context, userID uuid.UUID) ([]*post.TopicMessage, error) {
	if _, err := r.conn(ctx).Exec(ctx, deleteUserTopicMessagesQuery, userID); err != nil {
		return nil, errs.Wrap("r.conn.Exec", err)
	}

	rows, err := r.conn(ctx).Query(ctx, deleteUserTopicsQuery, userID)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	return collectTopicMessages(rows)
}

func scanTopic(row pgx.Row) (*post.Topic, error) {
	var (
		s     post.StoredTopic
		title string
	)

	err := row.Scan(&s.ID, &s.ClubID, &s.AuthorID, &title, &s.FromWall, &s.Closed, &s.Pinned, &s.Restricted,
		&s.CreatedAt, &s.UpdatedAt, &s.Messages)
	if err != nil {
		return nil, err
	}

	// Data comes from our own table and was validated on write.
	s.Title = post.TopicTitle(title)
	s.CreatedAt, s.UpdatedAt = s.CreatedAt.UTC(), s.UpdatedAt.UTC()

	return post.RestoreTopic(s), nil
}

// TopicMessageRepository is the service.TopicMessageRepository side of
// TopicRepository.
type TopicMessageRepository struct {
	r *TopicRepository
}

// saveTopicMessageQuery counts the message in its topic and dates the
// topic with it in one statement; nothing is added when the topic is gone.
const saveTopicMessageQuery = `
	WITH topic AS (
		UPDATE topics
		SET message_count = message_count + 1,
			updated_at = $7
		WHERE id = $2
		RETURNING id
	)
	INSERT INTO topic_messages (
		id,
		topic_id,
		author_id,
		text,
		photo_ids,
		from_wall,
		created_at
	)
	SELECT $1, topic.id, $3, $4, $5, $6, $7
	FROM topic
`

func (m *TopicMessageRepository) Save(ctx context.Context, msg *post.TopicMessage) error {
	content := msg.Content()

	tag, err := m.r.conn(ctx).Exec(ctx, saveTopicMessageQuery, msg.ID(), msg.TopicID(), msg.AuthorID(),
		content.Text().String(), photoIDs(content.Photos()), msg.FromWall(), msg.CreatedAt())
	if err != nil {
		return errs.Wrap("m.r.conn.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return post.ErrTopicNotFound
	}

	return nil
}

const getTopicMessageQuery = `
	SELECT
		id,
		topic_id,
		author_id,
		text,
		photo_ids,
		from_wall,
		created_at
	FROM topic_messages
	WHERE id = $1
`

func (m *TopicMessageRepository) Get(ctx context.Context, id uuid.UUID) (*post.TopicMessage, error) {
	msg, err := scanTopicMessage(m.r.conn(ctx).QueryRow(ctx, getTopicMessageQuery, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, post.ErrMessageNotFound
	}
	if err != nil {
		return nil, errs.Wrap("scanTopicMessage", err)
	}

	return msg, nil
}

// deleteTopicMessageQuery removes a message and uncounts it in one
// statement.
const deleteTopicMessageQuery = `
	WITH deleted AS (
		DELETE FROM topic_messages
		WHERE id = $1
		RETURNING topic_id
	)
	UPDATE topics
	SET message_count = message_count - 1
	FROM deleted
	WHERE topics.id = deleted.topic_id
`

func (m *TopicMessageRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := m.r.conn(ctx).Exec(ctx, deleteTopicMessageQuery, id)
	if err != nil {
		return errs.Wrap("m.r.conn.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return post.ErrMessageNotFound
	}

	return nil
}

const listTopicMessagesQuery = `
	SELECT
		id,
		topic_id,
		author_id,
		text,
		photo_ids,
		from_wall,
		created_at
	FROM topic_messages
	WHERE topic_id = $1
		AND id > $2
	ORDER BY id
	LIMIT $3
`

func (m *TopicMessageRepository) List(ctx context.Context, topicID, after uuid.UUID, limit int) ([]*post.TopicMessage, error) {
	rows, err := m.r.conn(ctx).Query(ctx, listTopicMessagesQuery, topicID, after, limit)
	if err != nil {
		return nil, errs.Wrap("m.r.conn.Query", err)
	}

	return collectTopicMessages(rows)
}

const lastTopicMessagesQuery = `
	SELECT DISTINCT ON (topic_id)
		id,
		topic_id,
		author_id,
		text,
		photo_ids,
		from_wall,
		created_at
	FROM topic_messages
	WHERE topic_id = ANY($1)
	ORDER BY topic_id, id DESC
`

func (m *TopicMessageRepository) Last(ctx context.Context, topicIDs []uuid.UUID) ([]*post.TopicMessage, error) {
	rows, err := m.r.conn(ctx).Query(ctx, lastTopicMessagesQuery, topicIDs)
	if err != nil {
		return nil, errs.Wrap("m.r.conn.Query", err)
	}

	return collectTopicMessages(rows)
}

// collectTopicMessages reads the messages of rows and closes them.
func collectTopicMessages(rows pgx.Rows) ([]*post.TopicMessage, error) {
	defer rows.Close()

	var found []*post.TopicMessage
	for rows.Next() {
		msg, err := scanTopicMessage(rows)
		if err != nil {
			return nil, errs.Wrap("scanTopicMessage", err)
		}
		found = append(found, msg)
	}

	if err := rows.Err(); err != nil {
		return nil, errs.Wrap("rows.Err", err)
	}

	return found, nil
}

func scanTopicMessage(row pgx.Row) (*post.TopicMessage, error) {
	var (
		s      post.StoredTopicMessage
		text   string
		photos []uuid.UUID
	)

	if err := row.Scan(&s.ID, &s.TopicID, &s.AuthorID, &text, &photos, &s.FromWall, &s.CreatedAt); err != nil {
		return nil, err
	}

	if len(photos) == 0 {
		photos = nil
	}

	// Data comes from our own table and was validated on write.
	s.Content = post.RestoreCommentContent(post.CommentText(text), post.Photos(photos))
	s.CreatedAt = s.CreatedAt.UTC()

	return post.RestoreTopicMessage(s), nil
}
