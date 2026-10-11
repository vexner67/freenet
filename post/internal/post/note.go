package post

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// maxNoteTitleLen is openvk's: the name of a note is a varchar(256).
const maxNoteTitleLen = 256

var (
	ErrNoteNotFound            = errors.New("note not found")
	ErrNoteTitleEmpty          = errors.New("note title is empty")
	ErrNoteTitleTooLong        = errors.New("note title is too long")
	ErrNoteEmpty               = errors.New("note text is empty")
	ErrNoteTooLong             = errors.New("note text is too long")
	ErrCannotChangeNote        = errors.New("only the author can change the note")
	ErrCannotDeleteNoteComment = errors.New("only the author of the comment or of the note can delete it")
)

// NoteTitle is what a note is called ("Название"): one line, trimmed.
type NoteTitle string

func NewNoteTitle(s string) (NoteTitle, error) {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return "", ErrNoteTitleEmpty
	}

	if utf8.RuneCountInString(s) > maxNoteTitleLen {
		return "", ErrNoteTitleTooLong
	}

	return NoteTitle(s), nil
}

func (t NoteTitle) String() string {
	return string(t)
}

// NewNoteText is what a note says ("Содержание"): as the text of a post,
// but never empty — a note is nothing else.
func NewNoteText(s string) (Text, error) {
	s, err := cleanText(s, maxTextLen, ErrNoteEmpty, ErrNoteTooLong)
	if err != nil {
		return "", err
	}

	return Text(s), nil
}

// Note is a note of a person, openvk's "Заметки": a title and a text,
// kept apart from the wall and commented on (NoteComment). Its author
// changes and deletes it whenever they like.
type Note struct {
	id        uuid.UUID
	authorID  uuid.UUID
	title     NoteTitle
	text      Text
	comments  int
	createdAt time.Time
	// editedAt is zero for a note never changed.
	editedAt time.Time
}

// NewNote writes a note; its UUIDv7 ID grows with time, so the notes of
// a person are ordered and paged by ID, newest first.
func NewNote(authorID uuid.UUID, title NoteTitle, text Text, now time.Time) (*Note, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	return &Note{
		id: id, authorID: authorID, title: title, text: text,
		// PostgreSQL keeps microseconds; trim now so it reads back equal.
		createdAt: now.UTC().Truncate(time.Microsecond),
	}, nil
}

// StoredNote is a note as storage keeps it.
type StoredNote struct {
	ID        uuid.UUID
	AuthorID  uuid.UUID
	Title     NoteTitle
	Text      Text
	Comments  int
	CreatedAt time.Time
	EditedAt  time.Time
}

// RestoreNote rebuilds a note from storage without validation.
func RestoreNote(s StoredNote) *Note {
	return &Note{
		id: s.ID, authorID: s.AuthorID, title: s.Title, text: s.Text,
		comments: s.Comments, createdAt: s.CreatedAt, editedAt: s.EditedAt,
	}
}

// Edited is the note as userID changed it at now: only its author may.
// It returns a new note: notes handed out never change.
func (n *Note) Edited(userID uuid.UUID, title NoteTitle, text Text, now time.Time) (*Note, error) {
	if !n.CanBeChangedBy(userID) {
		return nil, ErrCannotChangeNote
	}

	edited := *n
	edited.title, edited.text = title, text
	edited.editedAt = now.UTC().Truncate(time.Microsecond)

	return &edited, nil
}

// CanBeChangedBy: only the author changes and deletes a note.
func (n *Note) CanBeChangedBy(userID uuid.UUID) bool {
	return userID == n.authorID
}

// WithComments returns the note with count comments.
func (n *Note) WithComments(count int) *Note {
	counted := *n
	counted.comments = count

	return &counted
}

func (n *Note) ID() uuid.UUID {
	return n.id
}

func (n *Note) AuthorID() uuid.UUID {
	return n.authorID
}

func (n *Note) Title() NoteTitle {
	return n.title
}

func (n *Note) Text() Text {
	return n.text
}

func (n *Note) Comments() int {
	return n.comments
}

func (n *Note) CreatedAt() time.Time {
	return n.createdAt
}

// EditedAt is when the note was last changed, zero if never.
func (n *Note) EditedAt() time.Time {
	return n.editedAt
}

// NoteComment is a comment under a note: text only — openvk takes
// attachments too.
type NoteComment struct {
	id        uuid.UUID
	noteID    uuid.UUID
	authorID  uuid.UUID
	text      CommentText
	createdAt time.Time
}

// NewNoteComment comments noteID; the comments of a note are ordered and
// paged by ID, oldest first.
func NewNoteComment(noteID, authorID uuid.UUID, text CommentText, now time.Time) (*NoteComment, error) {
	if text == "" {
		return nil, ErrCommentEmpty
	}

	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	return &NoteComment{
		id: id, noteID: noteID, authorID: authorID, text: text,
		createdAt: now.UTC().Truncate(time.Microsecond),
	}, nil
}

// StoredNoteComment is a comment as storage keeps it.
type StoredNoteComment struct {
	ID        uuid.UUID
	NoteID    uuid.UUID
	AuthorID  uuid.UUID
	Text      CommentText
	CreatedAt time.Time
}

// RestoreNoteComment rebuilds a comment from storage without validation.
func RestoreNoteComment(s StoredNoteComment) *NoteComment {
	return &NoteComment{id: s.ID, noteID: s.NoteID, authorID: s.AuthorID, text: s.Text, createdAt: s.CreatedAt}
}

// CanBeDeletedBy: its author or the author of the note.
func (c *NoteComment) CanBeDeletedBy(userID uuid.UUID, note *Note) bool {
	return userID == c.authorID || userID == note.authorID
}

func (c *NoteComment) ID() uuid.UUID {
	return c.id
}

func (c *NoteComment) NoteID() uuid.UUID {
	return c.noteID
}

func (c *NoteComment) AuthorID() uuid.UUID {
	return c.authorID
}

func (c *NoteComment) Text() CommentText {
	return c.text
}

func (c *NoteComment) CreatedAt() time.Time {
	return c.createdAt
}
