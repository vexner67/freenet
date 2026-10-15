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
	"github.com/vexner67/freenet/pkg/pgpool"
)

type ChatRepository struct {
	pool *pgxpool.Pool
}

func NewChatRepository(pool *pgxpool.Pool) *ChatRepository {
	return &ChatRepository{pool: pool}
}

// conn is the transaction of ctx (service.TxManager), or the pool outside
// one.
func (r *ChatRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

const createChatQuery = `
	INSERT INTO chats (
		id,
		title,
		owner_id,
		created_at
	)
	VALUES ($1, $2, $3, $4)
`

func (r *ChatRepository) Create(ctx context.Context, c *message.Chat) error {
	if _, err := r.conn(ctx).Exec(ctx, createChatQuery, c.ID(), c.Title().String(), c.OwnerID(), c.CreatedAt()); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

// lockChatQuery holds the chat until the transaction ends: sending,
// adding and removing people of one chat take turns, so a message never
// reaches someone who has just gone, nor two changes count the same room.
const lockChatQuery = `
	SELECT
		id,
		title,
		owner_id,
		created_at,
		photo_id,
		pinned_message_id
	FROM chats
	WHERE id = $1
	FOR UPDATE
`

func (r *ChatRepository) Lock(ctx context.Context, id uuid.UUID) (*message.Chat, error) {
	var (
		stored          message.StoredChat
		title           string
		photo, pinnedID *uuid.UUID
	)

	err := r.conn(ctx).QueryRow(ctx, lockChatQuery, id).Scan(&stored.ID, &title, &stored.OwnerID, &stored.CreatedAt, &photo, &pinnedID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, message.ErrChatNotFound
	}
	if err != nil {
		return nil, errs.Wrap("r.conn.QueryRow", err)
	}

	stored.Title = message.Title(title)
	stored.CreatedAt = stored.CreatedAt.UTC()
	stored.PhotoID, stored.PinnedID = orNil(photo), orNil(pinnedID)

	return message.RestoreChat(stored), nil
}

// updateChatQuery saves what changes in a chat: its title, its photo and
// the message pinned in it.
const updateChatQuery = `
	UPDATE chats
	SET title = $2,
		photo_id = $3,
		pinned_message_id = $4
	WHERE id = $1
`

func (r *ChatRepository) Update(ctx context.Context, c *message.Chat) error {
	tag, err := r.conn(ctx).Exec(ctx, updateChatQuery, c.ID(), c.Title().String(), postIDToDB(c.PhotoID()), postIDToDB(c.PinnedID()))
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return message.ErrChatNotFound
	}

	return nil
}

// chatViewColumns are a chat as one of its people (m) has it, with the
// last message they are shown (msg), if any is left.
const chatViewColumns = `
		c.id,
		c.title,
		c.owner_id,
		c.created_at,
		c.photo_id,
		c.pinned_message_id,
		(
			SELECT count(*)
			FROM chat_members a
			WHERE a.chat_id = c.id
				AND a.state = 'active'
		),
		m.user_id,
		m.state,
		m.joined_at,
		m.last_message_id,
		m.last_incoming_id,
		m.read_up_to,
		msg.id,
		msg.sender_id,
		msg.text,
		msg.photo_ids,
		msg.action,
		msg.target_id,
		msg.created_at,
		msg.edited_at,
		msg.deleted_at,
		msg.post_id,
		msg.shared_photo_id,
		msg.forwarded,
		msg.sticker,
		msg.document_ids`

const chatViewQuery = `
	SELECT` + chatViewColumns + `
	FROM chat_members m
	JOIN chats c ON c.id = m.chat_id
	LEFT JOIN chat_messages msg ON msg.id = m.last_message_id
	WHERE m.chat_id = $1
		AND m.user_id = $2
`

func (r *ChatRepository) View(ctx context.Context, chatID, userID uuid.UUID) (message.ChatView, error) {
	view, err := scanChatView(r.conn(ctx).QueryRow(ctx, chatViewQuery, chatID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return message.ChatView{}, message.ErrChatNotFound
	}
	if err != nil {
		return message.ChatView{}, errs.Wrap("scanChatView", err)
	}

	return view, nil
}

// listChatsQuery leaves out a chat with nothing left to show, as the list
// of dialogs does.
const listChatsQuery = `
	SELECT` + chatViewColumns + `
	FROM chat_members m
	JOIN chats c ON c.id = m.chat_id
	JOIN chat_messages msg ON msg.id = m.last_message_id
	WHERE m.user_id = $1
		AND m.last_message_id < $2
	ORDER BY m.last_message_id DESC
	LIMIT $3
`

func (r *ChatRepository) List(ctx context.Context, userID, before uuid.UUID, limit int) ([]message.ChatView, error) {
	if before == uuid.Nil {
		before = newestCursor
	}

	rows, err := r.conn(ctx).Query(ctx, listChatsQuery, userID, before, limit)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}
	defer rows.Close()

	var views []message.ChatView
	for rows.Next() {
		view, err := scanChatView(rows)
		if err != nil {
			return nil, errs.Wrap("scanChatView", err)
		}
		views = append(views, view)
	}

	if err = rows.Err(); err != nil {
		return nil, errs.Wrap("rows.Err", err)
	}

	return views, nil
}

// scanChatView reads chatViewColumns; the message is NULL when the member
// has none left.
func scanChatView(row pgx.Row) (message.ChatView, error) {
	var (
		view            message.ChatView
		id, owner       uuid.UUID
		title           string
		createdAt       time.Time
		photo, pinnedID *uuid.UUID
		state           string
		last            struct {
			id, sender *uuid.UUID
			text       *string
			photos     []uuid.UUID
			action     *string
			target     *uuid.UUID
			createdAt  *time.Time
			editedAt   *time.Time
			deletedAt  *time.Time
			post       *uuid.UUID
			shared     *uuid.UUID
			forwarded  []byte
			sticker    *string
			documents  []uuid.UUID
		}
	)

	err := row.Scan(
		&id, &title, &owner, &createdAt, &photo, &pinnedID, &view.Members,
		&view.Me.UserID, &state, &view.Me.JoinedAt, &view.Me.SeenUpTo, &view.Me.LastIncomingID, &view.Me.ReadUpTo,
		&last.id, &last.sender, &last.text, &last.photos, &last.action, &last.target, &last.createdAt, &last.editedAt, &last.deletedAt,
		&last.post, &last.shared, &last.forwarded, &last.sticker, &last.documents,
	)
	if err != nil {
		return message.ChatView{}, err
	}

	view.Chat = message.RestoreChat(message.StoredChat{
		ID:        id,
		Title:     message.Title(title),
		OwnerID:   owner,
		CreatedAt: createdAt.UTC(),
		PhotoID:   orNil(photo),
		PinnedID:  orNil(pinnedID),
	})
	view.Me.State = message.MemberState(state)
	view.Me.JoinedAt = view.Me.JoinedAt.UTC()

	if last.id != nil {
		s := scannedChatMessage{
			stored: message.StoredChatMessage{ID: *last.id, ChatID: id, SenderID: *last.sender, CreatedAt: *last.createdAt},
			text:   *last.text, photos: last.photos, action: *last.action, target: last.target, editedAt: last.editedAt, deletedAt: last.deletedAt,
			post: last.post, sharedPhoto: last.shared, forwarded: last.forwarded, sticker: *last.sticker, documents: last.documents,
		}

		if view.LastMessage, err = s.message(); err != nil {
			return message.ChatView{}, errs.Wrap("s.message", err)
		}
	}

	return view, nil
}

// countUnreadChatsQuery stops at the limit, as the count of dialogs.
const countUnreadChatsQuery = `
	SELECT count(*)
	FROM (
		SELECT 1
		FROM chat_members
		WHERE user_id = $1
			AND last_incoming_id > read_up_to
		LIMIT $2
	) AS unread
`

func (r *ChatRepository) CountUnread(ctx context.Context, userID uuid.UUID, limit int) (int, error) {
	var count int

	if err := r.conn(ctx).QueryRow(ctx, countUnreadChatsQuery, userID, limit).Scan(&count); err != nil {
		return 0, errs.Wrap("r.conn.QueryRow", err)
	}

	return count, nil
}

// A user whose page is gone leaves every chat, with what they wrote there
// and what was done to them; then the others' rows go back to the
// messages the chats have left, and a chat the user created goes to who
// has been in it longest.
const (
	deleteUserChatMembersQuery = `
		DELETE FROM chat_members
		WHERE user_id = $1
		RETURNING chat_id
	`

	// The deleted messages that carried other messages on are returned:
	// the copies of the photos those showed go with them.
	deleteUserChatMessagesQuery = `
		WITH gone AS (
			DELETE FROM chat_messages
			WHERE sender_id = $1
				OR target_id = $1
			RETURNING` + chatMessageColumns + `
		)
		SELECT` + chatMessageColumns + `
		FROM gone
		WHERE forwarded <> '[]'
	`

	settleChatMembersQuery = `
		UPDATE chat_members m
		SET last_message_id = COALESCE((
				SELECT msg.id
				FROM chat_messages msg
				WHERE msg.chat_id = m.chat_id
					AND msg.id <= m.last_message_id
				ORDER BY msg.id DESC
				LIMIT 1
			), '00000000-0000-0000-0000-000000000000'),
			last_incoming_id = COALESCE((
				SELECT msg.id
				FROM chat_messages msg
				WHERE msg.chat_id = m.chat_id
					AND msg.id <= m.last_incoming_id
					AND msg.sender_id <> m.user_id
				ORDER BY msg.id DESC
				LIMIT 1
			), '00000000-0000-0000-0000-000000000000')
		WHERE m.chat_id = ANY($1)
	`

	inheritChatsQuery = `
		UPDATE chats c
		SET owner_id = heir.user_id
		FROM (
			SELECT DISTINCT ON (chat_id)
				chat_id,
				user_id
			FROM chat_members
			WHERE chat_id = ANY($1)
				AND state = 'active'
			ORDER BY chat_id, joined_at, user_id
		) AS heir
		WHERE c.id = heir.chat_id
			AND c.owner_id = $2
	`
)

func (r *ChatRepository) DeleteByUser(ctx context.Context, userID uuid.UUID) ([]*message.ChatMessage, error) {
	conn := r.conn(ctx)

	rows, err := conn.Query(ctx, deleteUserChatMembersQuery, userID)
	if err != nil {
		return nil, errs.Wrap("conn.Query", err)
	}

	chatIDs, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, errs.Wrap("pgx.CollectRows", err)
	}

	if len(chatIDs) == 0 {
		return nil, nil
	}

	rows, err = conn.Query(ctx, deleteUserChatMessagesQuery, userID)
	if err != nil {
		return nil, errs.Wrap("conn.Query", err)
	}

	carrying, err := collectChatMessages(rows)
	if err != nil {
		return nil, errs.Wrap("collectChatMessages", err)
	}

	if _, err = conn.Exec(ctx, settleChatMembersQuery, chatIDs); err != nil {
		return nil, errs.Wrap("conn.Exec", err)
	}

	if _, err = conn.Exec(ctx, inheritChatsQuery, chatIDs, userID); err != nil {
		return nil, errs.Wrap("conn.Exec", err)
	}

	return carrying, nil
}

type ChatMemberRepository struct {
	pool *pgxpool.Pool
}

func NewChatMemberRepository(pool *pgxpool.Pool) *ChatMemberRepository {
	return &ChatMemberRepository{pool: pool}
}

func (r *ChatMemberRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

const chatMemberColumns = `
		user_id,
		state,
		joined_at,
		last_message_id,
		last_incoming_id,
		read_up_to`

const getChatMemberQuery = `
	SELECT` + chatMemberColumns + `
	FROM chat_members
	WHERE chat_id = $1
		AND user_id = $2
`

func (r *ChatMemberRepository) Get(ctx context.Context, chatID, userID uuid.UUID) (message.Member, bool, error) {
	m, err := scanChatMember(r.conn(ctx).QueryRow(ctx, getChatMemberQuery, chatID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return message.Member{}, false, nil
	}
	if err != nil {
		return message.Member{}, false, errs.Wrap("scanChatMember", err)
	}

	return m, true, nil
}

// Those a chat started with joined at the same moment: its creator leads
// them.
const activeChatMembersQuery = `
	SELECT` + chatMemberColumns + `
	FROM chat_members
	WHERE chat_id = $1
		AND state = 'active'
	ORDER BY
		joined_at,
		user_id = (SELECT owner_id FROM chats WHERE id = $1) DESC,
		user_id
`

func (r *ChatMemberRepository) Active(ctx context.Context, chatID uuid.UUID) ([]message.Member, error) {
	rows, err := r.conn(ctx).Query(ctx, activeChatMembersQuery, chatID)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}
	defer rows.Close()

	var members []message.Member
	for rows.Next() {
		m, err := scanChatMember(rows)
		if err != nil {
			return nil, errs.Wrap("scanChatMember", err)
		}
		members = append(members, m)
	}

	if err = rows.Err(); err != nil {
		return nil, errs.Wrap("rows.Err", err)
	}

	return members, nil
}

func scanChatMember(row pgx.Row) (message.Member, error) {
	var (
		m     message.Member
		state string
	)

	if err := row.Scan(&m.UserID, &state, &m.JoinedAt, &m.SeenUpTo, &m.LastIncomingID, &m.ReadUpTo); err != nil {
		return message.Member{}, err
	}

	m.State = message.MemberState(state)
	m.JoinedAt = m.JoinedAt.UTC()

	return m, nil
}

// setChatMemberQuery keeps what someone known has of the chat: who comes
// back finds it as they left it, and the next message brings them up.
const setChatMemberQuery = `
	INSERT INTO chat_members (
		chat_id,
		user_id,
		state,
		joined_at
	)
	VALUES ($1, $2, $3, $4)
	ON CONFLICT (chat_id, user_id) DO UPDATE
	SET state = EXCLUDED.state
`

func (r *ChatMemberRepository) Set(ctx context.Context, chatID, userID uuid.UUID, state message.MemberState, now time.Time) error {
	if _, err := r.conn(ctx).Exec(ctx, setChatMemberQuery, chatID, userID, string(state), now.UTC()); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

const markChatReadQuery = `
	UPDATE chat_members
	SET read_up_to = $3
	WHERE chat_id = $1
		AND user_id = $2
		AND read_up_to < $3
`

func (r *ChatMemberRepository) MarkRead(ctx context.Context, chatID, userID, upTo uuid.UUID) error {
	if _, err := r.conn(ctx).Exec(ctx, markChatReadQuery, chatID, userID, upTo); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

type ChatMessageRepository struct {
	pool *pgxpool.Pool
}

func NewChatMessageRepository(pool *pgxpool.Pool) *ChatMessageRepository {
	return &ChatMessageRepository{pool: pool}
}

func (r *ChatMessageRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

// saveChatMessageQuery stores the message and brings everyone in the chat
// up to it in one statement, so the lists of chats never lag behind. The
// chat is locked by then (lockChatQuery): its messages come in turn.
const saveChatMessageQuery = `
	WITH saved AS (
		INSERT INTO chat_messages (
			id,
			chat_id,
			sender_id,
			text,
			photo_ids,
			action,
			target_id,
			created_at,
			post_id,
			shared_photo_id,
			forwarded,
			sticker,
			document_ids,
			reply_to_id
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING id, chat_id, sender_id
	)
	UPDATE chat_members m
	SET last_message_id = GREATEST(m.last_message_id, saved.id),
		last_incoming_id = CASE
			WHEN m.user_id = saved.sender_id THEN m.last_incoming_id
			ELSE GREATEST(m.last_incoming_id, saved.id)
		END
	FROM saved
	WHERE m.chat_id = saved.chat_id
		AND m.state = 'active'
`

func (r *ChatMessageRepository) Save(ctx context.Context, m *message.ChatMessage) error {
	forwarded, err := forwardedToDB(m.Forwarded())
	if err != nil {
		return errs.Wrap("forwardedToDB", err)
	}

	_, err = r.conn(ctx).Exec(ctx, saveChatMessageQuery,
		m.ID(), m.ChatID(), m.SenderID(), m.Text().String(), photoIDs(m.Photos()), string(m.Action()), postIDToDB(m.TargetID()), m.CreatedAt(),
		postIDToDB(m.Post()), postIDToDB(m.SharedPhoto()), forwarded, m.Sticker().String(), documentIDs(m.Documents()), postIDToDB(m.ReplyTo()))
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

const chatMessageColumns = `
		id,
		chat_id,
		sender_id,
		text,
		photo_ids,
		action,
		target_id,
		created_at,
		edited_at,
		deleted_at,
		post_id,
		shared_photo_id,
		forwarded,
		sticker,
		document_ids,
		reply_to_id`

const getChatMessageQuery = `
	SELECT` + chatMessageColumns + `
	FROM chat_messages
	WHERE id = $1
`

func (r *ChatMessageRepository) Get(ctx context.Context, id uuid.UUID) (*message.ChatMessage, error) {
	m, err := scanChatMessage(r.conn(ctx).QueryRow(ctx, getChatMessageQuery, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, message.ErrNotFound
	}
	if err != nil {
		return nil, errs.Wrap("scanChatMessage", err)
	}

	return m, nil
}

const getChatMessagesQuery = `
	SELECT` + chatMessageColumns + `
	FROM chat_messages
	WHERE id = ANY($1)
`

func (r *ChatMessageRepository) GetMany(ctx context.Context, ids []uuid.UUID) ([]*message.ChatMessage, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	rows, err := r.pool.Query(ctx, getChatMessagesQuery, ids)
	if err != nil {
		return nil, errs.Wrap("r.pool.Query", err)
	}

	return collectChatMessages(rows)
}

const (
	listChatMessagesBeforeQuery = `
		SELECT` + chatMessageColumns + `
		FROM chat_messages
		WHERE chat_id = $1
			AND id < $2
			AND id <= $3
		ORDER BY id DESC
		LIMIT $4
	`

	listChatMessagesAfterQuery = `
		SELECT` + chatMessageColumns + `
		FROM chat_messages
		WHERE chat_id = $1
			AND id > $2
			AND id <= $3
		ORDER BY id
		LIMIT $4
	`
)

func (r *ChatMessageRepository) ListBefore(ctx context.Context, chatID, before, upTo uuid.UUID, limit int) ([]*message.ChatMessage, error) {
	if before == uuid.Nil {
		before = newestCursor
	}

	return r.list(ctx, listChatMessagesBeforeQuery, chatID, before, upTo, limit)
}

func (r *ChatMessageRepository) ListAfter(ctx context.Context, chatID, after, upTo uuid.UUID, limit int) ([]*message.ChatMessage, error) {
	return r.list(ctx, listChatMessagesAfterQuery, chatID, after, upTo, limit)
}

func (r *ChatMessageRepository) list(ctx context.Context, query string, chatID, bound, upTo uuid.UUID, limit int) ([]*message.ChatMessage, error) {
	rows, err := r.conn(ctx).Query(ctx, query, chatID, bound, upTo, limit)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	return collectChatMessages(rows)
}

func collectChatMessages(rows pgx.Rows) ([]*message.ChatMessage, error) {
	defer rows.Close()

	var messages []*message.ChatMessage
	for rows.Next() {
		m, err := scanChatMessage(rows)
		if err != nil {
			return nil, errs.Wrap("scanChatMessage", err)
		}
		messages = append(messages, m)
	}

	if err := rows.Err(); err != nil {
		return nil, errs.Wrap("rows.Err", err)
	}

	return messages, nil
}

// chatMessageWords is all a message of a chat says, itself and in the
// messages it carries on, as chat_messages_search_idx has it: the
// expression here and in the migration must be the same for the index to
// serve the search.
const chatMessageWords = `message_words(msg.text, msg.forwarded)`

// searchChatMessagesQuery: $2 is one chat, NULL for all the chats of the
// user $1; $3 is the pattern of what the messages say, themselves or in
// the messages they carry on: the index finds the messages whose words,
// put together, match it, and which of them says it in one text is
// checked after. Only what people
// wrote and did not take back is found, and only what the user is shown:
// who is out of a chat keeps it as it was.
const searchChatMessagesQuery = `
	SELECT
		msg.id,
		msg.chat_id,
		msg.sender_id,
		msg.text,
		msg.photo_ids,
		msg.action,
		msg.target_id,
		msg.created_at,
		msg.edited_at,
		msg.deleted_at,
		msg.post_id,
		msg.shared_photo_id,
		msg.forwarded,
		msg.sticker,
		msg.document_ids,
		msg.reply_to_id
	FROM chat_members m
	JOIN chat_messages msg ON msg.chat_id = m.chat_id
	WHERE m.user_id = $1
		AND ($2::uuid IS NULL OR m.chat_id = $2)
		AND ` + chatMessageWords + ` ILIKE $3
		AND (
			msg.text ILIKE $3
			OR EXISTS (
				SELECT 1
				FROM jsonb_array_elements(msg.forwarded) AS f
				WHERE f->>'text' ILIKE $3
			)
		)
		AND msg.action = ''
		AND msg.deleted_at IS NULL
		AND msg.id < $4
		AND (m.state = 'active' OR msg.id <= m.last_message_id)
	ORDER BY msg.id DESC
	LIMIT $5
`

func (r *ChatMessageRepository) Search(ctx context.Context, userID, chatID uuid.UUID, q message.Query, before uuid.UUID, limit int) ([]*message.ChatMessage, error) {
	if before == uuid.Nil {
		before = newestCursor
	}

	rows, err := r.pool.Query(ctx, searchChatMessagesQuery, userID, postIDToDB(chatID), likePattern(q), before, limit)
	if err != nil {
		return nil, errs.Wrap("r.pool.Query", err)
	}

	return collectChatMessages(rows)
}

// scannedChatMessage is a row of chat_messages as the queries select it.
type scannedChatMessage struct {
	stored      message.StoredChatMessage
	text        string
	photos      []uuid.UUID
	action      string
	target      *uuid.UUID
	editedAt    *time.Time
	deletedAt   *time.Time
	post        *uuid.UUID
	sharedPhoto *uuid.UUID
	forwarded   []byte
	sticker     string
	documents   []uuid.UUID
	// replyTo is NULL for a message answering none.
	replyTo *uuid.UUID
}

func scanChatMessage(row pgx.Row) (*message.ChatMessage, error) {
	var s scannedChatMessage

	err := row.Scan(
		&s.stored.ID, &s.stored.ChatID, &s.stored.SenderID, &s.text, &s.photos, &s.action, &s.target, &s.stored.CreatedAt, &s.editedAt, &s.deletedAt,
		&s.post, &s.sharedPhoto, &s.forwarded, &s.sticker, &s.documents, &s.replyTo,
	)
	if err != nil {
		return nil, err
	}

	return s.message()
}

// message finishes a scanned message. Data comes from our own table and
// was validated on write.
func (s *scannedChatMessage) message() (*message.ChatMessage, error) {
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
	stored.Content = message.RestoreContent(message.Text(s.text), photos, postID, forwarded).WithSticker(message.Sticker(s.sticker)).
		WithDocuments(documentsFromDB(s.documents)).Answering(orNil(s.replyTo))
	if s.sharedPhoto != nil {
		stored.Content = stored.Content.WithSharedPhoto(*s.sharedPhoto)
	}

	stored.Action = message.Action(s.action)
	stored.CreatedAt = stored.CreatedAt.UTC()

	if s.target != nil {
		stored.TargetID = *s.target
	}

	if s.editedAt != nil {
		stored.EditedAt = s.editedAt.UTC()
	}

	if s.deletedAt != nil {
		stored.DeletedAt = s.deletedAt.UTC()
	}

	return message.RestoreChatMessage(stored), nil
}

const updateChatMessageQuery = `
	UPDATE chat_messages
	SET text = $2,
		photo_ids = $3,
		edited_at = $4
	WHERE id = $1
		AND deleted_at IS NULL
`

func (r *ChatMessageRepository) Update(ctx context.Context, m *message.ChatMessage) error {
	tag, err := r.conn(ctx).Exec(ctx, updateChatMessageQuery, m.ID(), m.Text().String(), photoIDs(m.Photos()), m.EditedAt())
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return message.ErrNotFound
	}

	return nil
}

const retractChatMessageQuery = `
	WITH reactions AS (
		DELETE FROM chat_message_reactions
		WHERE message_id = $1
	)
	UPDATE chat_messages
	SET text = '',
		reply_to_id = NULL,
		photo_ids = '{}',
		post_id = NULL,
		shared_photo_id = NULL,
		forwarded = '[]',
		sticker = '',
		document_ids = '{}',
		edited_at = NULL,
		deleted_at = $2
	WHERE id = $1
		AND deleted_at IS NULL
`

func (r *ChatMessageRepository) Retract(ctx context.Context, m *message.ChatMessage) error {
	tag, err := r.conn(ctx).Exec(ctx, retractChatMessageQuery, m.ID(), m.DeletedAt())
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return message.ErrNotFound
	}

	return nil
}
