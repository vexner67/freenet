package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/message/internal/message"
	"github.com/vexner67/freenet/pkg/errs"
)

// newestCursor is above every UUID, so "id < newestCursor" starts from the
// newest.
var newestCursor = uuid.Max

type MessageRepository struct {
	pool *pgxpool.Pool
}

func NewMessageRepository(pool *pgxpool.Pool) *MessageRepository {
	return &MessageRepository{
		pool: pool,
	}
}

// saveMessageQuery stores the message and brings both sides' dialogs up to
// it in one statement, so the list of dialogs never lags behind. GREATEST
// keeps a dialog right when two messages race. The sides are written in
// user_id order whoever writes: two people writing to each other at once
// would otherwise lock the two rows in opposite orders and deadlock.
const saveMessageQuery = `
	WITH saved AS (
		INSERT INTO messages (
			id,
			sender_id,
			recipient_id,
			text,
			created_at
		)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, sender_id, recipient_id
	)
	INSERT INTO dialogs (
		user_id,
		peer_id,
		last_message_id,
		last_incoming_id
	)
	SELECT user_id, peer_id, last_message_id, last_incoming_id
	FROM (
		SELECT sender_id AS user_id, recipient_id AS peer_id, id AS last_message_id,
			'00000000-0000-0000-0000-000000000000'::uuid AS last_incoming_id
		FROM saved
		UNION ALL
		SELECT recipient_id, sender_id, id, id
		FROM saved
	) AS sides
	ORDER BY user_id
	ON CONFLICT (user_id, peer_id) DO UPDATE
	SET last_message_id = GREATEST(dialogs.last_message_id, EXCLUDED.last_message_id),
		last_incoming_id = GREATEST(dialogs.last_incoming_id, EXCLUDED.last_incoming_id)
`

func (r *MessageRepository) Save(ctx context.Context, m *message.Message) error {
	_, err := r.pool.Exec(ctx, saveMessageQuery, m.ID(), m.SenderID(), m.RecipientID(), m.Text().String(), m.CreatedAt())
	if err != nil {
		return errs.Wrap("r.pool.Exec", err)
	}

	return nil
}

const getMessageQuery = `
	SELECT
		id,
		sender_id,
		recipient_id,
		text,
		created_at
	FROM messages
	WHERE id = $1
`

func (r *MessageRepository) Get(ctx context.Context, id uuid.UUID) (*message.Message, error) {
	m, err := scanMessage(r.pool.QueryRow(ctx, getMessageQuery, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, message.ErrNotFound
	}
	if err != nil {
		return nil, errs.Wrap("scanMessage", err)
	}

	return m, nil
}

// The dialog queries take the pair as $1, $2 in any order.
const (
	listMessagesBeforeQuery = `
		SELECT
			id,
			sender_id,
			recipient_id,
			text,
			created_at
		FROM messages
		WHERE user_lo = LEAST($1::uuid, $2::uuid)
			AND user_hi = GREATEST($1::uuid, $2::uuid)
			AND id < $3
		ORDER BY id DESC
		LIMIT $4
	`
	listMessagesAfterQuery = `
		SELECT
			id,
			sender_id,
			recipient_id,
			text,
			created_at
		FROM messages
		WHERE user_lo = LEAST($1::uuid, $2::uuid)
			AND user_hi = GREATEST($1::uuid, $2::uuid)
			AND id > $3
		ORDER BY id
		LIMIT $4
	`
)

func (r *MessageRepository) ListBefore(ctx context.Context, a, b, before uuid.UUID, limit int) ([]*message.Message, error) {
	if before == uuid.Nil {
		before = newestCursor
	}

	return r.list(ctx, listMessagesBeforeQuery, a, b, before, limit)
}

func (r *MessageRepository) ListAfter(ctx context.Context, a, b, after uuid.UUID, limit int) ([]*message.Message, error) {
	return r.list(ctx, listMessagesAfterQuery, a, b, after, limit)
}

func (r *MessageRepository) list(ctx context.Context, query string, a, b, bound uuid.UUID, limit int) ([]*message.Message, error) {
	rows, err := r.pool.Query(ctx, query, a, b, bound, limit)
	if err != nil {
		return nil, errs.Wrap("r.pool.Query", err)
	}
	defer rows.Close()

	var messages []*message.Message
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, errs.Wrap("scanMessage", err)
		}
		messages = append(messages, m)
	}

	if err = rows.Err(); err != nil {
		return nil, errs.Wrap("rows.Err", err)
	}

	return messages, nil
}

func scanMessage(row pgx.Row) (*message.Message, error) {
	var (
		id, senderID, recipientID uuid.UUID
		text                      string
		createdAt                 time.Time
	)

	if err := row.Scan(&id, &senderID, &recipientID, &text, &createdAt); err != nil {
		return nil, err
	}

	// Data comes from our own table and was validated on write.
	return message.Restore(id, senderID, recipientID, message.Text(text), createdAt.UTC()), nil
}

type DialogRepository struct {
	pool *pgxpool.Pool
}

func NewDialogRepository(pool *pgxpool.Pool) *DialogRepository {
	return &DialogRepository{
		pool: pool,
	}
}

// listDialogsQuery joins the last message and the peer's side, whose mark
// tells whether they have read the user's messages.
const listDialogsQuery = `
	SELECT
		d.peer_id,
		d.last_incoming_id,
		d.read_up_to,
		COALESCE(p.read_up_to, '00000000-0000-0000-0000-000000000000'::uuid),
		m.id,
		m.sender_id,
		m.recipient_id,
		m.text,
		m.created_at
	FROM dialogs d
	JOIN messages m ON m.id = d.last_message_id
	LEFT JOIN dialogs p ON p.user_id = d.peer_id AND p.peer_id = d.user_id
	WHERE d.user_id = $1
		AND d.last_message_id < $2
	ORDER BY d.last_message_id DESC
	LIMIT $3
`

func (r *DialogRepository) List(ctx context.Context, userID, before uuid.UUID, limit int) ([]message.Dialog, error) {
	if before == uuid.Nil {
		before = newestCursor
	}

	rows, err := r.pool.Query(ctx, listDialogsQuery, userID, before, limit)
	if err != nil {
		return nil, errs.Wrap("r.pool.Query", err)
	}
	defer rows.Close()

	var dialogs []message.Dialog
	for rows.Next() {
		d, err := scanDialog(rows)
		if err != nil {
			return nil, errs.Wrap("scanDialog", err)
		}
		dialogs = append(dialogs, d)
	}

	if err = rows.Err(); err != nil {
		return nil, errs.Wrap("rows.Err", err)
	}

	return dialogs, nil
}

func scanDialog(row pgx.Row) (message.Dialog, error) {
	var (
		d                         message.Dialog
		id, senderID, recipientID uuid.UUID
		text                      string
		createdAt                 time.Time
	)

	err := row.Scan(&d.PeerID, &d.LastIncomingID, &d.Marks.Mine, &d.Marks.Theirs,
		&id, &senderID, &recipientID, &text, &createdAt)
	if err != nil {
		return message.Dialog{}, err
	}

	d.LastMessage = message.Restore(id, senderID, recipientID, message.Text(text), createdAt.UTC())

	return d, nil
}

// countUnreadDialogsQuery stops at the limit instead of counting every
// dialog of a very sociable user.
const countUnreadDialogsQuery = `
	SELECT count(*)
	FROM (
		SELECT 1
		FROM dialogs
		WHERE user_id = $1
			AND last_incoming_id > read_up_to
		LIMIT $2
	) AS unread
`

func (r *DialogRepository) CountUnread(ctx context.Context, userID uuid.UUID, limit int) (int, error) {
	var count int

	if err := r.pool.QueryRow(ctx, countUnreadDialogsQuery, userID, limit).Scan(&count); err != nil {
		return 0, errs.Wrap("r.pool.QueryRow", err)
	}

	return count, nil
}

const readMarksQuery = `
	SELECT
		COALESCE((SELECT read_up_to FROM dialogs WHERE user_id = $1 AND peer_id = $2), '00000000-0000-0000-0000-000000000000'::uuid),
		COALESCE((SELECT read_up_to FROM dialogs WHERE user_id = $2 AND peer_id = $1), '00000000-0000-0000-0000-000000000000'::uuid)
`

func (r *DialogRepository) ReadMarks(ctx context.Context, userID, peerID uuid.UUID) (message.ReadMarks, error) {
	var marks message.ReadMarks

	if err := r.pool.QueryRow(ctx, readMarksQuery, userID, peerID).Scan(&marks.Mine, &marks.Theirs); err != nil {
		return message.ReadMarks{}, errs.Wrap("r.pool.QueryRow", err)
	}

	return marks, nil
}

// hasIncomingQuery reads the user's side of the dialog: its last incoming
// message is the zero UUID until the peer writes.
const hasIncomingQuery = `
	SELECT EXISTS (
		SELECT 1
		FROM dialogs
		WHERE user_id = $1
			AND peer_id = $2
			AND last_incoming_id <> '00000000-0000-0000-0000-000000000000'
	)
`

func (r *DialogRepository) HasIncoming(ctx context.Context, userID, peerID uuid.UUID) (bool, error) {
	var ok bool

	if err := r.pool.QueryRow(ctx, hasIncomingQuery, userID, peerID).Scan(&ok); err != nil {
		return false, errs.Wrap("r.pool.QueryRow", err)
	}

	return ok, nil
}

// markReadQuery never moves the mark back. The dialog exists: the user
// can only mark a message they received.
const markReadQuery = `
	UPDATE dialogs
	SET read_up_to = GREATEST(read_up_to, $3)
	WHERE user_id = $1
		AND peer_id = $2
`

func (r *DialogRepository) MarkRead(ctx context.Context, userID, peerID, upTo uuid.UUID) error {
	if _, err := r.pool.Exec(ctx, markReadQuery, userID, peerID, upTo); err != nil {
		return errs.Wrap("r.pool.Exec", err)
	}

	return nil
}

// deleteUserDialogsQuery deletes the messages of every dialog of the user
// and both sides of each dialog; the CTE sees the dialogs as they were.
const deleteUserDialogsQuery = `
	WITH gone_messages AS (
		DELETE FROM messages m
		USING dialogs d
		WHERE d.user_id = $1
			AND m.user_lo = LEAST(d.user_id, d.peer_id)
			AND m.user_hi = GREATEST(d.user_id, d.peer_id)
	)
	DELETE FROM dialogs
	WHERE user_id = $1
		OR (user_id, peer_id) IN (
			SELECT peer_id, user_id
			FROM dialogs
			WHERE user_id = $1
		)
`

func (r *MessageRepository) DeleteByUser(ctx context.Context, userID uuid.UUID) error {
	if _, err := r.pool.Exec(ctx, deleteUserDialogsQuery, userID); err != nil {
		return errs.Wrap("r.pool.Exec", err)
	}

	return nil
}
