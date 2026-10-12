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
)

// NoteRepository keeps notes and, as Comments, the comments under them.
type NoteRepository struct {
	pool *pgxpool.Pool
}

func NewNoteRepository(pool *pgxpool.Pool) *NoteRepository {
	return &NoteRepository{
		pool: pool,
	}
}

// conn is the transaction of ctx (service.TxManager), or the pool outside
// one.
func (r *NoteRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

const saveNoteQuery = `
	INSERT INTO notes (
		id,
		author_id,
		title,
		text,
		created_at
	)
	VALUES ($1, $2, $3, $4, $5)
`

func (r *NoteRepository) Save(ctx context.Context, n *post.Note) error {
	_, err := r.conn(ctx).Exec(ctx, saveNoteQuery, n.ID(), n.AuthorID(), n.Title().String(), n.Text().String(), n.CreatedAt())
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

const getNoteQuery = `
	SELECT
		id,
		author_id,
		title,
		text,
		comment_count - hidden_note_comments(id) AS comment_count,
		created_at,
		edited_at
	FROM notes
	WHERE id = $1
`

func (r *NoteRepository) Get(ctx context.Context, id uuid.UUID) (*post.Note, error) {
	n, err := scanNote(r.conn(ctx).QueryRow(ctx, getNoteQuery, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, post.ErrNoteNotFound
	}
	if err != nil {
		return nil, errs.Wrap("scanNote", err)
	}

	return n, nil
}

// updateNoteQuery leaves the counter of comments to the comments.
const updateNoteQuery = `
	UPDATE notes
	SET
		title = $2,
		text = $3,
		edited_at = $4
	WHERE id = $1
`

func (r *NoteRepository) Update(ctx context.Context, n *post.Note) error {
	tag, err := r.conn(ctx).Exec(ctx, updateNoteQuery, n.ID(), n.Title().String(), n.Text().String(), n.EditedAt())
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return post.ErrNoteNotFound
	}

	return nil
}

const deleteNoteQuery = `
	DELETE FROM notes
	WHERE id = $1
`

func (r *NoteRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.conn(ctx).Exec(ctx, deleteNoteQuery, id)
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	if tag.RowsAffected() == 0 {
		return post.ErrNoteNotFound
	}

	return nil
}

const listNotesQuery = `
	SELECT
		id,
		author_id,
		title,
		text,
		comment_count - hidden_note_comments(id) AS comment_count,
		created_at,
		edited_at
	FROM notes
	WHERE author_id = $1
		AND id < $2
	ORDER BY id DESC
	LIMIT $3
`

func (r *NoteRepository) List(ctx context.Context, authorID, before uuid.UUID, limit int) ([]*post.Note, error) {
	if before == uuid.Nil {
		before = newestCursor
	}

	rows, err := r.conn(ctx).Query(ctx, listNotesQuery, authorID, before, limit)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}

	notes, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*post.Note, error) {
		return scanNote(row)
	})
	if err != nil {
		return nil, errs.Wrap("pgx.CollectRows", err)
	}

	return notes, nil
}

const countNotesQuery = `
	SELECT count(*)
	FROM notes
	WHERE author_id = $1
`

func (r *NoteRepository) Count(ctx context.Context, authorID uuid.UUID) (int, error) {
	var n int

	if err := r.conn(ctx).QueryRow(ctx, countNotesQuery, authorID).Scan(&n); err != nil {
		return 0, errs.Wrap("r.conn.QueryRow", err)
	}

	return n, nil
}

// deleteUserNotesQuery deletes the comments a user left under the notes
// of others, uncounting them there, and the user's own notes, which take
// their comments with them.
const deleteUserNotesQuery = `
	WITH gone AS (
		DELETE FROM note_comments
		WHERE author_id = $1
		RETURNING note_id
	), uncounted AS (
		UPDATE notes n
		SET comment_count = n.comment_count - g.comments
		FROM (
			SELECT
				note_id,
				count(*) AS comments
			FROM gone
			GROUP BY note_id
		) g
		WHERE n.id = g.note_id
			AND n.author_id <> $1
	)
	DELETE FROM notes
	WHERE author_id = $1
`

// DeleteByUser deletes the notes of a user with their comments and the
// comments the user left under others' notes.
func (r *NoteRepository) DeleteByUser(ctx context.Context, userID uuid.UUID) error {
	if _, err := r.conn(ctx).Exec(ctx, deleteUserNotesQuery, userID); err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

func scanNote(row pgx.Row) (*post.Note, error) {
	var (
		s        post.StoredNote
		title    string
		text     string
		editedAt *time.Time
	)

	if err := row.Scan(&s.ID, &s.AuthorID, &title, &text, &s.Comments, &s.CreatedAt, &editedAt); err != nil {
		return nil, err
	}

	// Data comes from our own table and was validated on write.
	s.Title, s.Text = post.NoteTitle(title), post.Text(text)
	s.CreatedAt = s.CreatedAt.UTC()

	if editedAt != nil {
		s.EditedAt = editedAt.UTC()
	}

	return post.RestoreNote(s), nil
}

// NoteCommentRepository is the service.NoteCommentRepository side of
// NoteRepository: comments counted in the note by the same statement.
type NoteCommentRepository struct {
	r *NoteRepository
}

// Comments is the same repository as service.NoteCommentRepository.
func (r *NoteRepository) Comments() *NoteCommentRepository {
	return &NoteCommentRepository{r: r}
}

const saveNoteCommentQuery = `
	WITH saved AS (
		INSERT INTO note_comments (
			id,
			note_id,
			author_id,
			text,
			created_at
		)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING note_id
	)
	UPDATE notes n
	SET comment_count = n.comment_count + 1
	FROM saved
	WHERE n.id = saved.note_id
`

func (c *NoteCommentRepository) Save(ctx context.Context, nc *post.NoteComment) error {
	_, err := c.r.conn(ctx).Exec(ctx, saveNoteCommentQuery, nc.ID(), nc.NoteID(), nc.AuthorID(), nc.Text().String(), nc.CreatedAt())
	if isForeignKeyViolation(err) {
		// The note was deleted meanwhile.
		return post.ErrNoteNotFound
	}
	if err != nil {
		return errs.Wrap("c.r.conn.Exec", err)
	}

	return nil
}

const getNoteCommentQuery = `
	SELECT
		id,
		note_id,
		author_id,
		text,
		created_at
	FROM note_comments
	WHERE id = $1
`

func (c *NoteCommentRepository) Get(ctx context.Context, id uuid.UUID) (*post.NoteComment, error) {
	nc, err := scanNoteComment(c.r.conn(ctx).QueryRow(ctx, getNoteCommentQuery, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, post.ErrCommentNotFound
	}
	if err != nil {
		return nil, errs.Wrap("scanNoteComment", err)
	}

	return nc, nil
}

const deleteNoteCommentQuery = `
	WITH gone AS (
		DELETE FROM note_comments
		WHERE id = $1
		RETURNING note_id
	), uncounted AS (
		UPDATE notes n
		SET comment_count = n.comment_count - 1
		FROM gone
		WHERE n.id = gone.note_id
	)
	SELECT count(*)
	FROM gone
`

func (c *NoteCommentRepository) Delete(ctx context.Context, id uuid.UUID) error {
	var deleted int

	if err := c.r.conn(ctx).QueryRow(ctx, deleteNoteCommentQuery, id).Scan(&deleted); err != nil {
		return errs.Wrap("c.r.conn.QueryRow", err)
	}

	if deleted == 0 {
		return post.ErrCommentNotFound
	}

	return nil
}

const listNoteCommentsQuery = `
	SELECT
		id,
		note_id,
		author_id,
		text,
		created_at
	FROM note_comments
	WHERE note_id = $1
		AND id > $2
		AND NOT user_hidden(author_id)
	ORDER BY id
	LIMIT $3
`

func (c *NoteCommentRepository) List(ctx context.Context, noteID, after uuid.UUID, limit int) ([]*post.NoteComment, error) {
	rows, err := c.r.conn(ctx).Query(ctx, listNoteCommentsQuery, noteID, after, limit)
	if err != nil {
		return nil, errs.Wrap("c.r.conn.Query", err)
	}

	comments, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*post.NoteComment, error) {
		return scanNoteComment(row)
	})
	if err != nil {
		return nil, errs.Wrap("pgx.CollectRows", err)
	}

	return comments, nil
}

const getNoteCommentsQuery = `
	SELECT
		id,
		note_id,
		author_id,
		text,
		created_at
	FROM note_comments
	WHERE id = ANY($1)
`

func (c *NoteCommentRepository) GetMany(ctx context.Context, ids []uuid.UUID) ([]*post.NoteComment, error) {
	rows, err := c.r.conn(ctx).Query(ctx, getNoteCommentsQuery, ids)
	if err != nil {
		return nil, errs.Wrap("c.r.conn.Query", err)
	}

	comments, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*post.NoteComment, error) {
		return scanNoteComment(row)
	})
	if err != nil {
		return nil, errs.Wrap("pgx.CollectRows", err)
	}

	return comments, nil
}

func scanNoteComment(row pgx.Row) (*post.NoteComment, error) {
	var (
		s    post.StoredNoteComment
		text string
	)

	if err := row.Scan(&s.ID, &s.NoteID, &s.AuthorID, &text, &s.CreatedAt); err != nil {
		return nil, err
	}

	// Data comes from our own table and was validated on write.
	s.Text = post.CommentText(text)
	s.CreatedAt = s.CreatedAt.UTC()

	return post.RestoreNoteComment(s), nil
}
