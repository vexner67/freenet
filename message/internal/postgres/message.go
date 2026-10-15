package postgres

import (
	"context"
	"encoding/json"
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
			shared_photo_id,
			forwarded,
			created_at,
			sealed,
			sticker,
			document_ids
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
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
	forwarded, err := forwardedToDB(m.Forwarded())
	if err != nil {
		return errs.Wrap("forwardedToDB", err)
	}

	_, err = r.conn(ctx).Exec(ctx, saveMessageQuery, m.ID(), m.SenderID(), m.RecipientID(), m.Text().String(), photoIDs(m.Photos()), postIDToDB(m.Post()), postIDToDB(m.SharedPhoto()), forwarded, m.CreatedAt(), m.Sealed().Bytes(), m.Sticker().String(), documentIDs(m.Documents()))
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
		shared_photo_id,
		created_at,
		edited_at,
		hidden_from_sender,
		hidden_from_recipient,
		forwarded,
		deleted_at,
		sealed,
		sticker,
		document_ids
	FROM messages
	WHERE id = $1
`

// Lock reads the message for the transaction of ctx to change: another
// one that wants it waits.
func (r *MessageRepository) Lock(ctx context.Context, id uuid.UUID) (*message.Message, error) {
	m, err := scanMessage(r.conn(ctx).QueryRow(ctx, getMessageQuery+" FOR UPDATE", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, message.ErrNotFound
	}
	if err != nil {
		return nil, errs.Wrap("scanMessage", err)
	}

	return m, nil
}

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
			shared_photo_id,
			created_at,
			edited_at,
			hidden_from_sender,
			hidden_from_recipient,
			forwarded,
			deleted_at,
			sealed,
			sticker,
			document_ids
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
			shared_photo_id,
			created_at,
			edited_at,
			hidden_from_sender,
			hidden_from_recipient,
			forwarded,
			deleted_at,
			sealed,
			sticker,
			document_ids
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

// scanned is a row of messages as the queries select it.
type scanned struct {
	stored    message.Stored
	text      string
	photos    []uuid.UUID
	post      *uuid.UUID
	forwarded []byte
	// sharedPhoto is NULL for no photo of an album.
	sharedPhoto *uuid.UUID
	editedAt    *time.Time
	deletedAt   *time.Time
	sealed      []byte
	sticker     string
	documents   []uuid.UUID
}

// dest are the columns of a message, in the order the queries list them.
func (s *scanned) dest() []any {
	return []any{
		&s.stored.ID, &s.stored.SenderID, &s.stored.RecipientID, &s.text, &s.photos, &s.post, &s.sharedPhoto, &s.stored.CreatedAt,
		&s.editedAt, &s.stored.HiddenFromSender, &s.stored.HiddenFromRecipient, &s.forwarded, &s.deletedAt, &s.sealed, &s.sticker, &s.documents,
	}
}

func scanMessage(row pgx.Row) (*message.Message, error) {
	var s scanned

	if err := row.Scan(s.dest()...); err != nil {
		return nil, err
	}

	return s.message()
}

// message finishes a scanned message. Data comes from our own table and
// was validated on write.
func (s *scanned) message() (*message.Message, error) {
	photos := message.Photos(s.photos)
	if len(photos) == 0 {
		photos = nil // no photos, as a new message has them
	}

	var postID uuid.UUID
	if s.post != nil {
		postID = *s.post
	}

	forwarded, err := forwardedFromDB(s.forwarded)
	if err != nil {
		return nil, errs.Wrap("forwardedFromDB", err)
	}

	stored := s.stored
	stored.Content = message.RestoreContent(message.Text(s.text), photos, postID, forwarded).WithSealed(message.Sealed(s.sealed)).WithSticker(message.Sticker(s.sticker)).
		WithDocuments(documentsFromDB(s.documents))
	if s.sharedPhoto != nil {
		stored.Content = stored.Content.WithSharedPhoto(*s.sharedPhoto)
	}
	stored.CreatedAt = stored.CreatedAt.UTC()

	if s.editedAt != nil {
		stored.EditedAt = s.editedAt.UTC()
	}
	if s.deletedAt != nil {
		stored.DeletedAt = s.deletedAt.UTC()
	}

	return message.Restore(stored), nil
}

// forwardedJSON is a forwarded message in the forwarded column, a JSON
// array of them.
type forwardedJSON struct {
	SenderID  uuid.UUID   `json:"sender_id"`
	Text      string      `json:"text,omitempty"`
	Photos    []uuid.UUID `json:"photos,omitempty"`
	Post      *uuid.UUID  `json:"post,omitempty"`
	Photo     *uuid.UUID  `json:"photo,omitempty"`
	Sticker   string      `json:"sticker,omitempty"`
	Documents []uuid.UUID `json:"documents,omitempty"`
	CreatedAt time.Time   `json:"created_at"`
	// Depth is absent for a message forwarded itself, as all were before
	// forwards nested.
	Depth int `json:"depth,omitempty"`
}

func forwardedToDB(forwarded message.Forwards) ([]byte, error) {
	rows := make([]forwardedJSON, len(forwarded))
	for i, f := range forwarded {
		rows[i] = forwardedJSON{
			SenderID:  f.SenderID,
			Text:      f.Text.String(),
			Photos:    f.Photos,
			Post:      postIDToDB(f.Post),
			Photo:     postIDToDB(f.SharedPhoto),
			Sticker:   f.Sticker.String(),
			Documents: f.Documents,
			CreatedAt: f.CreatedAt,
			Depth:     f.Depth,
		}
	}

	data, err := json.Marshal(rows)
	if err != nil {
		return nil, errs.Wrap("json.Marshal", err)
	}

	return data, nil
}

func forwardedFromDB(data []byte) (message.Forwards, error) {
	var rows []forwardedJSON
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, errs.Wrap("json.Unmarshal", err)
	}

	if len(rows) == 0 {
		return nil, nil
	}

	forwarded := make(message.Forwards, len(rows))
	for i, row := range rows {
		forwarded[i] = message.Forwarded{
			SenderID:  row.SenderID,
			Text:      message.Text(row.Text),
			CreatedAt: row.CreatedAt.UTC(),
			Depth:     row.Depth,
			Sticker:   message.Sticker(row.Sticker),
			Documents: documentsFromDB(row.Documents),
		}
		if len(row.Photos) > 0 {
			forwarded[i].Photos = row.Photos
		}
		if row.Post != nil {
			forwarded[i].Post = *row.Post
		}
		if row.Photo != nil {
			forwarded[i].SharedPhoto = *row.Photo
		}
	}

	return forwarded, nil
}

// postIDToDB is NULL for no post, and for no shared photo, chat photo or
// pinned message.
func postIDToDB(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}

	return &id
}

// orNil reads what postIDToDB wrote: uuid.Nil for NULL.
func orNil(id *uuid.UUID) uuid.UUID {
	if id == nil {
		return uuid.Nil
	}

	return *id
}

// photoIDs is photos for the photo_ids column: pgx writes a nil slice as
// NULL, the column wants an empty array.
func photoIDs(photos message.Photos) []uuid.UUID {
	if photos == nil {
		return []uuid.UUID{}
	}

	return photos
}

// documentIDs is documents for the document_ids column, which wants an
// empty array rather than NULL.
func documentIDs(documents message.Documents) []uuid.UUID {
	if documents == nil {
		return []uuid.UUID{}
	}

	return documents
}

// documentsFromDB is nil for none, as a new message has them.
func documentsFromDB(ids []uuid.UUID) message.Documents {
	if len(ids) == 0 {
		return nil
	}

	return ids
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
		m.shared_photo_id,
		m.created_at,
		m.edited_at,
		m.hidden_from_sender,
		m.hidden_from_recipient,
		m.forwarded,
		m.deleted_at,
		m.sealed,
		m.sticker,
		m.document_ids
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
		d message.Dialog
		s scanned
	)

	dest := append([]any{&d.PeerID, &d.LastIncomingID, &d.Marks.Mine, &d.Marks.Theirs}, s.dest()...)
	if err := row.Scan(dest...); err != nil {
		return message.Dialog{}, err
	}

	last, err := s.message()
	if err != nil {
		return message.Dialog{}, errs.Wrap("s.message", err)
	}

	d.LastMessage = last

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

// retractMessageQuery leaves the row where the dialogs point at it and
// empties it: the message was taken back and says nothing anymore.
const retractMessageQuery = `
	UPDATE messages
	SET text = '',
		photo_ids = '{}',
		post_id = NULL,
		shared_photo_id = NULL,
		forwarded = '[]',
		sealed = NULL,
		sticker = '',
		document_ids = '{}',
		edited_at = NULL,
		deleted_at = $2
	WHERE id = $1
		AND deleted_at IS NULL
`

func (r *MessageRepository) Retract(ctx context.Context, m *message.Message) error {
	tag, err := r.conn(ctx).Exec(ctx, retractMessageQuery, m.ID(), m.DeletedAt())
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return message.ErrNotFound
	}

	return nil
}

const updateMessageQuery = `
	UPDATE messages
	SET text = $2,
		edited_at = $3,
		photo_ids = $4,
		sealed = $5
	WHERE id = $1
`

func (r *MessageRepository) Update(ctx context.Context, m *message.Message) error {
	tag, err := r.conn(ctx).Exec(ctx, updateMessageQuery, m.ID(), m.Text().String(), m.EditedAt(), photoIDs(m.Photos()), m.Sealed().Bytes())
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

// likePattern finds q as a substring: the wildcards of LIKE in it are
// themselves.
func likePattern(q message.Query) string {
	return "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(q.String()) + "%"
}

// deleteUserDialogsQuery deletes the messages of every dialog of the user
// and both sides of each dialog, and returns the deleted messages that had
// photos or carried other messages on; the CTEs see the dialogs as they were.
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
			m.shared_photo_id,
			m.created_at,
			m.edited_at,
			m.hidden_from_sender,
			m.hidden_from_recipient,
			m.forwarded,
			m.deleted_at,
			m.sealed,
			m.sticker,
			m.document_ids
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
		shared_photo_id,
		created_at,
		edited_at,
		hidden_from_sender,
		hidden_from_recipient,
		forwarded,
		deleted_at,
		sealed,
		sticker,
		document_ids
	FROM gone_messages
	WHERE cardinality(photo_ids) > 0
		OR forwarded <> '[]'
`

func (r *MessageRepository) DeleteByUser(ctx context.Context, userID uuid.UUID) ([]*message.Message, error) {
	rows, err := r.conn(ctx).Query(ctx, deleteUserDialogsQuery, userID)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	return collectMessages(rows)
}

// KeyRepository keeps the public key of each user in message_keys.
type KeyRepository struct {
	pool *pgxpool.Pool
}

func NewKeyRepository(pool *pgxpool.Pool) *KeyRepository {
	return &KeyRepository{pool: pool}
}

func (r *KeyRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

const getKeyQuery = `
	SELECT public_key
	FROM message_keys
	WHERE user_id = $1
`

func (r *KeyRepository) Get(ctx context.Context, userID uuid.UUID) (message.Key, error) {
	var key []byte

	err := r.conn(ctx).QueryRow(ctx, getKeyQuery, userID).Scan(&key)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", message.ErrNoKey
	}
	if err != nil {
		return "", errs.Wrap("r.conn.QueryRow", err)
	}

	return message.Key(key), nil
}

const setKeyQuery = `
	INSERT INTO message_keys (
		user_id,
		public_key,
		updated_at
	)
	VALUES ($1, $2, now())
	ON CONFLICT (user_id) DO UPDATE
	SET public_key = EXCLUDED.public_key,
		updated_at = EXCLUDED.updated_at
`

func (r *KeyRepository) Set(ctx context.Context, userID uuid.UUID, key message.Key) error {
	if _, err := r.conn(ctx).Exec(ctx, setKeyQuery, userID, key.Bytes()); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

const deleteKeyQuery = `
	DELETE FROM message_keys
	WHERE user_id = $1
`

func (r *KeyRepository) Delete(ctx context.Context, userID uuid.UUID) error {
	if _, err := r.conn(ctx).Exec(ctx, deleteKeyQuery, userID); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}
