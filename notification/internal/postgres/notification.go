package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/notification/internal/notification"
	"github.com/vexner67/freenet/notification/internal/service"
	"github.com/vexner67/freenet/pkg/errs"
)

// newestCursor is above every UUID, so "id < newestCursor" starts from the
// newest notification.
var newestCursor = uuid.Max

type NotificationRepository struct {
	pool *pgxpool.Pool
}

func NewNotificationRepository(pool *pgxpool.Pool) *NotificationRepository {
	return &NotificationRepository{
		pool: pool,
	}
}

// saveNotificationQuery skips an event already notified: the partial unique
// indexes keep one row per like, comment and follow.
const saveNotificationQuery = `
	INSERT INTO notifications (
		id,
		kind,
		recipient_id,
		actor_id,
		post_id,
		comment_id,
		created_at
	)
	VALUES ($1, $2, $3, $4, $5, $6, $7)
	ON CONFLICT DO NOTHING
`

func (r *NotificationRepository) Save(ctx context.Context, n *notification.Notification) error {
	e := n.Event()

	_, err := r.pool.Exec(ctx, saveNotificationQuery,
		n.ID(), string(e.Kind), e.RecipientID, e.ActorID, nullable(e.PostID), nullable(e.CommentID), n.CreatedAt())
	if err != nil {
		return errs.Wrap("r.pool.Exec", err)
	}

	return nil
}

const getNotificationQuery = `
	SELECT
		id,
		kind,
		recipient_id,
		actor_id,
		post_id,
		comment_id,
		created_at
	FROM notifications
	WHERE id = $1
`

func (r *NotificationRepository) Get(ctx context.Context, id uuid.UUID) (*notification.Notification, error) {
	n, err := scanNotification(r.pool.QueryRow(ctx, getNotificationQuery, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, notification.ErrNotFound
	}
	if err != nil {
		return nil, errs.Wrap("scanNotification", err)
	}

	return n, nil
}

// The delete queries match the unique indexes of the migration.
const (
	deleteLikeNotificationQuery = `
		DELETE FROM notifications
		WHERE kind = 'like'
			AND actor_id = $1
			AND post_id = $2
	`
	deleteCommentNotificationQuery = `
		DELETE FROM notifications
		WHERE kind = 'comment'
			AND comment_id = $1
	`
	deleteFollowNotificationQuery = `
		DELETE FROM notifications
		WHERE kind = 'follow'
			AND actor_id = $1
			AND recipient_id = $2
	`
)

func (r *NotificationRepository) Delete(ctx context.Context, e notification.Event) error {
	var err error

	switch e.Kind {
	case notification.KindLike:
		_, err = r.pool.Exec(ctx, deleteLikeNotificationQuery, e.ActorID, e.PostID)
	case notification.KindComment:
		_, err = r.pool.Exec(ctx, deleteCommentNotificationQuery, e.CommentID)
	case notification.KindFollow:
		_, err = r.pool.Exec(ctx, deleteFollowNotificationQuery, e.ActorID, e.RecipientID)
	default:
		return notification.ErrUnknownKind
	}

	if err != nil {
		return errs.Wrap("r.pool.Exec", err)
	}

	return nil
}

const deleteNotificationsByPostQuery = `
	DELETE FROM notifications
	WHERE post_id = $1
`

func (r *NotificationRepository) DeleteByPost(ctx context.Context, postID uuid.UUID) error {
	if _, err := r.pool.Exec(ctx, deleteNotificationsByPostQuery, postID); err != nil {
		return errs.Wrap("r.pool.Exec", err)
	}

	return nil
}

const (
	listUnreadNotificationsQuery = `
		SELECT
			id,
			kind,
			recipient_id,
			actor_id,
			post_id,
			comment_id,
			created_at
		FROM notifications
		WHERE recipient_id = $1
			AND id > $2
			AND id < $3
		ORDER BY id DESC
		LIMIT $4
	`
	listReadNotificationsQuery = `
		SELECT
			id,
			kind,
			recipient_id,
			actor_id,
			post_id,
			comment_id,
			created_at
		FROM notifications
		WHERE recipient_id = $1
			AND id <= $2
			AND id < $3
		ORDER BY id DESC
		LIMIT $4
	`
)

func (r *NotificationRepository) List(ctx context.Context, q service.Query) ([]*notification.Notification, error) {
	query := listReadNotificationsQuery
	if q.Unread {
		query = listUnreadNotificationsQuery
	}

	before := q.Before
	if before == uuid.Nil {
		before = newestCursor
	}

	rows, err := r.pool.Query(ctx, query, q.RecipientID, q.ReadUpTo, before, q.Limit)
	if err != nil {
		return nil, errs.Wrap("r.pool.Query", err)
	}
	defer rows.Close()

	var found []*notification.Notification
	for rows.Next() {
		n, err := scanNotification(rows)
		if err != nil {
			return nil, errs.Wrap("scanNotification", err)
		}
		found = append(found, n)
	}

	if err = rows.Err(); err != nil {
		return nil, errs.Wrap("rows.Err", err)
	}

	return found, nil
}

// countNewerNotificationsQuery stops at the limit instead of counting a
// long unread tail.
const countNewerNotificationsQuery = `
	SELECT count(*)
	FROM (
		SELECT 1
		FROM notifications
		WHERE recipient_id = $1
			AND id > $2
		LIMIT $3
	) AS newer
`

func (r *NotificationRepository) CountNewer(ctx context.Context, recipientID, after uuid.UUID, limit int) (int, error) {
	var count int

	if err := r.pool.QueryRow(ctx, countNewerNotificationsQuery, recipientID, after, limit).Scan(&count); err != nil {
		return 0, errs.Wrap("r.pool.QueryRow", err)
	}

	return count, nil
}

// nullable stores uuid.Nil as NULL, so the unique indexes and the CHECK
// of the migration see "no post" and "no comment".
func nullable(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}

	return &id
}

func scanNotification(row pgx.Row) (*notification.Notification, error) {
	var (
		id, recipientID, actorID uuid.UUID
		kind                     string
		postID, commentID        *uuid.UUID
		createdAt                time.Time
	)

	if err := row.Scan(&id, &kind, &recipientID, &actorID, &postID, &commentID, &createdAt); err != nil {
		return nil, err
	}

	e := notification.Event{
		Kind:        notification.Kind(kind),
		RecipientID: recipientID,
		ActorID:     actorID,
	}
	if postID != nil {
		e.PostID = *postID
	}
	if commentID != nil {
		e.CommentID = *commentID
	}

	// Data comes from our own table and was validated on write.
	return notification.Restore(id, e, createdAt.UTC()), nil
}

type ReadMarkRepository struct {
	pool *pgxpool.Pool
}

func NewReadMarkRepository(pool *pgxpool.Pool) *ReadMarkRepository {
	return &ReadMarkRepository{
		pool: pool,
	}
}

const getReadMarkQuery = `
	SELECT read_up_to
	FROM notification_reads
	WHERE user_id = $1
`

func (r *ReadMarkRepository) ReadUpTo(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	var mark uuid.UUID

	err := r.pool.QueryRow(ctx, getReadMarkQuery, userID).Scan(&mark)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, nil
	}
	if err != nil {
		return uuid.Nil, errs.Wrap("r.pool.QueryRow", err)
	}

	return mark, nil
}

// markReadQuery never moves the mark back: two tabs marking different
// pages leave the newer one.
const markReadQuery = `
	INSERT INTO notification_reads (
		user_id,
		read_up_to
	)
	VALUES ($1, $2)
	ON CONFLICT (user_id) DO UPDATE
	SET read_up_to = GREATEST(notification_reads.read_up_to, EXCLUDED.read_up_to)
`

func (r *ReadMarkRepository) MarkRead(ctx context.Context, userID, notificationID uuid.UUID) error {
	if _, err := r.pool.Exec(ctx, markReadQuery, userID, notificationID); err != nil {
		return errs.Wrap("r.pool.Exec", err)
	}

	return nil
}
