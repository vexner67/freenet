package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/message/internal/message"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
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

// conn is the transaction of ctx (service.TxManager), or the pool outside
// one.
func (r *MessageRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
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
			photo_ids,
			post_id,
			created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
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
	_, err := r.conn(ctx).Exec(ctx, saveMessageQuery, m.ID(), m.SenderID(), m.RecipientID(), m.Text().String(), photoIDs(m.Photos()), postIDToDB(m.Post()), m.CreatedAt())
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

const getMessageQuery = `
	SELECT
		id,
		sender_id,
		recipient_id,
		text,
		photo_ids,
		post_id,
		created_at,
		edited_at,
		hidden_from_sender,
		hidden_from_recipient
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

// The dialog queries take the reader as $1 and the peer as $2: messages
// the reader deleted for themselves are left out.
const (
	listMessagesBeforeQuery = `
		SELECT
			id,
			sender_id,
			recipient_id,
			text,
			photo_ids,
			post_id,
			created_at,
			edited_at,
			hidden_from_sender,
			hidden_from_recipient
		FROM messages
		WHERE user_lo = LEAST($1::uuid, $2::uuid)
			AND user_hi = GREATEST($1::uuid, $2::uuid)
			AND id < $3
			AND NOT CASE WHEN sender_id = $1 THEN hidden_from_sender ELSE hidden_from_recipient END
		ORDER BY id DESC
		LIMIT $4
	`
	listMessagesAfterQuery = `
		SELECT
			id,
			sender_id,
			recipient_id,
			text,
			photo_ids,
			post_id,
			created_at,
			edited_at,
			hidden_from_sender,
			hidden_from_recipient
		FROM messages
		WHERE user_lo = LEAST($1::uuid, $2::uuid)
			AND user_hi = GREATEST($1::uuid, $2::uuid)
			AND id > $3
			AND NOT CASE WHEN sender_id = $1 THEN hidden_from_sender ELSE hidden_from_recipient END
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

	return collectMessages(rows)
}

// collectMessages reads the messages of rows and closes them.
func collectMessages(rows pgx.Rows) ([]*message.Message, error) {
	defer rows.Close()

	var messages []*message.Message
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, errs.Wrap("scanMessage", err)
		}
		messages = append(messages, m)
	}

	if err := rows.Err(); err != nil {
		return nil, errs.Wrap("rows.Err", err)
	}

	return messages, nil
}

func scanMessage(row pgx.Row) (*message.Message, error) {
	var (
		s      message.Stored
		text   string
		photos []uuid.UUID
		post   *uuid.UUID
	)

	var editedAt *time.Time

	err := row.Scan(&s.ID, &s.SenderID, &s.RecipientID, &text, &photos, &post, &s.CreatedAt,
		&editedAt, &s.HiddenFromSender, &s.HiddenFromRecipient)
	if err != nil {
		return nil, err
	}

	if editedAt != nil {
		s.EditedAt = editedAt.UTC()
	}

	return restore(s, text, photos, post), nil
}

// restore finishes a scanned message. Data comes from our own table and
// was validated on write.
func restore(s message.Stored, text string, photos []uuid.UUID, post *uuid.UUID) *message.Message {
	if len(photos) == 0 {
		photos = nil // no photos, as a new message has them
	}

	var postID uuid.UUID
	if post != nil {
		postID = *post
	}

	s.Content = message.RestoreContent(message.Text(text), message.Photos(photos), postID)
	s.CreatedAt = s.CreatedAt.UTC()

	return message.Restore(s)
}

// postIDToDB is NULL for no post.
func postIDToDB(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}

	return &id
}

// photoIDs is photos for the photo_ids column: pgx writes a nil slice as
// NULL, the column wants an empty array.
func photoIDs(photos message.Photos) []uuid.UUID {
	if photos == nil {
		return []uuid.UUID{}
	}

	return photos
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
		m.photo_ids,
		m.post_id,
		m.created_at,
		m.edited_at,
		m.hidden_from_sender,
		m.hidden_from_recipient
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
		d      message.Dialog
		s      message.Stored
		text   string
		photos []uuid.UUID
		post   *uuid.UUID
	)

	var editedAt *time.Time

	err := row.Scan(&d.PeerID, &d.LastIncomingID, &d.Marks.Mine, &d.Marks.Theirs,
		&s.ID, &s.SenderID, &s.RecipientID, &text, &photos, &post, &s.CreatedAt,
		&editedAt, &s.HiddenFromSender, &s.HiddenFromRecipient)
	if err != nil {
		return message.Dialog{}, err
	}

	if editedAt != nil {
		s.EditedAt = editedAt.UTC()
	}

	d.LastMessage = restore(s, text, photos, post)

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

// deleteMessageQuery deletes the message and brings each side of its
// dialog back to the newest message left, if the deleted one was its last
// (the zero UUID when none is left: the dialog leaves the list, its read
// mark stays). The sides are locked in user_id order first, as Save
// writes them. A message sent meanwhile has moved the side on, and a side
// pointing elsewhere is left alone: the CTEs see the messages as they
// were, hence "id <> g.id". A side comes back only to a message it has
// not deleted for itself.
const deleteMessageQuery = `
	WITH gone AS (
		DELETE FROM messages
		WHERE id = $1
		RETURNING id, user_lo, user_hi
	), sides AS (
		SELECT d.user_id, d.peer_id
		FROM dialogs d
		JOIN gone g ON (d.user_id, d.peer_id) IN ((g.user_lo, g.user_hi), (g.user_hi, g.user_lo))
		ORDER BY d.user_id
		FOR UPDATE OF d
	), moved AS (
		UPDATE dialogs d
		SET last_message_id = CASE
				WHEN d.last_message_id <> g.id THEN d.last_message_id
				ELSE COALESCE((
					SELECT m.id
					FROM messages m
					WHERE m.user_lo = g.user_lo
						AND m.user_hi = g.user_hi
						AND m.id <> g.id
						AND NOT CASE WHEN m.sender_id = d.user_id THEN m.hidden_from_sender ELSE m.hidden_from_recipient END
					ORDER BY m.id DESC
					LIMIT 1
				), '00000000-0000-0000-0000-000000000000')
			END,
			last_incoming_id = CASE
				WHEN d.last_incoming_id <> g.id THEN d.last_incoming_id
				ELSE COALESCE((
					SELECT m.id
					FROM messages m
					WHERE m.user_lo = g.user_lo
						AND m.user_hi = g.user_hi
						AND m.sender_id = d.peer_id
						AND m.id <> g.id
						AND NOT m.hidden_from_recipient
					ORDER BY m.id DESC
					LIMIT 1
				), '00000000-0000-0000-0000-000000000000')
			END
		FROM sides s, gone g
		WHERE d.user_id = s.user_id
			AND d.peer_id = s.peer_id
	)
	SELECT count(*)
	FROM gone
`

func (r *MessageRepository) Delete(ctx context.Context, m *message.Message) error {
	var deleted int

	if err := r.conn(ctx).QueryRow(ctx, deleteMessageQuery, m.ID()).Scan(&deleted); err != nil {
		return errs.Wrap("r.conn.QueryRow", err)
	}

	if deleted == 0 {
		return message.ErrNotFound
	}

	return nil
}

const updateMessageQuery = `
	UPDATE messages
	SET text = $2,
		edited_at = $3
	WHERE id = $1
`

func (r *MessageRepository) Update(ctx context.Context, m *message.Message) error {
	tag, err := r.conn(ctx).Exec(ctx, updateMessageQuery, m.ID(), m.Text().String(), m.EditedAt())
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return message.ErrNotFound
	}

	return nil
}

// hideMessageQuery deletes the message for one of its sides ($2) and, in
// the same statement, brings that side of the dialog back to the newest
// message it still has, if the hidden one was its last, as
// deleteMessageQuery does for both. The CTEs see the messages as they
// were, hence "id <> h.id".
const hideMessageQuery = `
	WITH hidden AS (
		UPDATE messages
		SET hidden_from_sender = hidden_from_sender OR sender_id = $2,
			hidden_from_recipient = hidden_from_recipient OR recipient_id = $2
		WHERE id = $1
			AND $2 IN (sender_id, recipient_id)
		RETURNING id, user_lo, user_hi
	), moved AS (
		UPDATE dialogs d
		SET last_message_id = CASE
				WHEN d.last_message_id <> h.id THEN d.last_message_id
				ELSE COALESCE((
					SELECT m.id
					FROM messages m
					WHERE m.user_lo = h.user_lo
						AND m.user_hi = h.user_hi
						AND m.id <> h.id
						AND NOT CASE WHEN m.sender_id = d.user_id THEN m.hidden_from_sender ELSE m.hidden_from_recipient END
					ORDER BY m.id DESC
					LIMIT 1
				), '00000000-0000-0000-0000-000000000000')
			END,
			last_incoming_id = CASE
				WHEN d.last_incoming_id <> h.id THEN d.last_incoming_id
				ELSE COALESCE((
					SELECT m.id
					FROM messages m
					WHERE m.user_lo = h.user_lo
						AND m.user_hi = h.user_hi
						AND m.sender_id = d.peer_id
						AND m.id <> h.id
						AND NOT m.hidden_from_recipient
					ORDER BY m.id DESC
					LIMIT 1
				), '00000000-0000-0000-0000-000000000000')
			END
		FROM hidden h
		WHERE d.user_id = $2
			AND d.peer_id IN (h.user_lo, h.user_hi)
	)
	SELECT count(*)
	FROM hidden
`

func (r *MessageRepository) Hide(ctx context.Context, m *message.Message, userID uuid.UUID) error {
	var hidden int

	if err := r.conn(ctx).QueryRow(ctx, hideMessageQuery, m.ID(), userID).Scan(&hidden); err != nil {
		return errs.Wrap("r.conn.QueryRow", err)
	}

	if hidden == 0 {
		return message.ErrNotFound
	}

	return nil
}

// searchMessagesQuery: $2 is the peer of one dialog, NULL for all the
// dialogs of the user $1; $3 is the pattern of what the messages say.
const searchMessagesQuery = `
	SELECT
		id,
		sender_id,
		recipient_id,
		text,
		photo_ids,
		post_id,
		created_at,
		edited_at,
		hidden_from_sender,
		hidden_from_recipient
	FROM messages
	WHERE $1 IN (sender_id, recipient_id)
		AND ($2::uuid IS NULL OR $2 IN (sender_id, recipient_id))
		AND text ILIKE $3
		AND id < $4
		AND NOT CASE WHEN sender_id = $1 THEN hidden_from_sender ELSE hidden_from_recipient END
	ORDER BY id DESC
	LIMIT $5
`

func (r *MessageRepository) Search(ctx context.Context, userID, peerID uuid.UUID, q message.Query, before uuid.UUID, limit int) ([]*message.Message, error) {
	if before == uuid.Nil {
		before = newestCursor
	}

	// The wildcards of LIKE in the query are themselves.
	pattern := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(q.String()) + "%"

	rows, err := r.pool.Query(ctx, searchMessagesQuery, userID, postIDToDB(peerID), pattern, before, limit)
	if err != nil {
		return nil, errs.Wrap("r.pool.Query", err)
	}

	return collectMessages(rows)
}

// deleteUserDialogsQuery deletes the messages of every dialog of the user
// and both sides of each dialog, and returns the deleted messages that had
// photos; the CTEs see the dialogs as they were.
const deleteUserDialogsQuery = `
	WITH gone_messages AS (
		DELETE FROM messages m
		USING dialogs d
		WHERE d.user_id = $1
			AND m.user_lo = LEAST(d.user_id, d.peer_id)
			AND m.user_hi = GREATEST(d.user_id, d.peer_id)
		RETURNING
			m.id,
			m.sender_id,
			m.recipient_id,
			m.text,
			m.photo_ids,
			m.post_id,
			m.created_at,
			m.edited_at,
			m.hidden_from_sender,
			m.hidden_from_recipient
	), gone_dialogs AS (
		DELETE FROM dialogs
		WHERE user_id = $1
			OR (user_id, peer_id) IN (
				SELECT peer_id, user_id
				FROM dialogs
				WHERE user_id = $1
			)
	)
	SELECT
		id,
		sender_id,
		recipient_id,
		text,
		photo_ids,
		post_id,
		created_at,
		edited_at,
		hidden_from_sender,
		hidden_from_recipient
	FROM gone_messages
	WHERE cardinality(photo_ids) > 0
`

func (r *MessageRepository) DeleteByUser(ctx context.Context, userID uuid.UUID) ([]*message.Message, error) {
	rows, err := r.conn(ctx).Query(ctx, deleteUserDialogsQuery, userID)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	return collectMessages(rows)
}
