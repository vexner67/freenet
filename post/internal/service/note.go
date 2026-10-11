package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/post/internal/post"
)

type NoteRepository interface {
	Save(ctx context.Context, n *post.Note) error
	// Get, Update and Delete return post.ErrNoteNotFound; a deleted note
	// takes its comments.
	Get(ctx context.Context, id uuid.UUID) (*post.Note, error)
	Update(ctx context.Context, n *post.Note) error
	Delete(ctx context.Context, id uuid.UUID) error
	// List returns up to limit notes of an author, newest first, with
	// IDs below before (uuid.Nil: from the newest); Count counts them all.
	List(ctx context.Context, authorID, before uuid.UUID, limit int) ([]*post.Note, error)
	Count(ctx context.Context, authorID uuid.UUID) (int, error)
}

// NoteCommentRepository keeps the comments under notes; Save and Delete
// count them in the note by the same statement.
type NoteCommentRepository interface {
	// Save returns post.ErrNoteNotFound when the note is gone.
	Save(ctx context.Context, c *post.NoteComment) error
	// Get and Delete return post.ErrCommentNotFound.
	Get(ctx context.Context, id uuid.UUID) (*post.NoteComment, error)
	Delete(ctx context.Context, id uuid.UUID) error
	// List returns up to limit comments of a note, oldest first, with
	// IDs above after (uuid.Nil: from the oldest).
	List(ctx context.Context, noteID, after uuid.UUID, limit int) ([]*post.NoteComment, error)
	// GetMany returns the comments that exist, in no particular order.
	GetMany(ctx context.Context, ids []uuid.UUID) ([]*post.NoteComment, error)
}

// NotePage is one screen of the notes of a person; NextCursor is uuid.Nil
// on the last, Total counts them on all pages.
type NotePage struct {
	Notes      []*post.Note
	NextCursor uuid.UUID
	Total      int
}

// NoteCommentPage is one screen of the comments of a note; Total counts
// them on all pages.
type NoteCommentPage struct {
	Comments   []*post.NoteComment
	NextCursor uuid.UUID
	Total      int
}

// NoteService keeps the notes of people ("Заметки" of openvk) and the
// comments under them. Who sees the notes of a person is the person's
// (the user service), which the caller asks.
type NoteService struct {
	notes    NoteRepository
	comments NoteCommentRepository
	tx       TxManager
	notifier Notifier
	now      func() time.Time
}

// NoteDeps are what NoteService works with.
type NoteDeps struct {
	Notes    NoteRepository
	Comments NoteCommentRepository
	Tx       TxManager
	Notifier Notifier
}

func NewNoteService(d NoteDeps) *NoteService {
	return &NoteService{notes: d.Notes, comments: d.Comments, tx: d.Tx, notifier: d.Notifier, now: time.Now}
}

// Create writes a note of authorID.
func (s *NoteService) Create(ctx context.Context, authorID uuid.UUID, title post.NoteTitle, text post.Text) (*post.Note, error) {
	n, err := post.NewNote(authorID, title, text, s.now())
	if err != nil {
		return nil, errs.Wrap("post.NewNote", err)
	}

	if err = s.notes.Save(ctx, n); err != nil {
		return nil, errs.Wrap("s.notes.Save", err)
	}

	return n, nil
}

// Get is one note, whole; post.ErrNoteNotFound for a missing one.
func (s *NoteService) Get(ctx context.Context, id uuid.UUID) (*post.Note, error) {
	n, err := s.notes.Get(ctx, id)

	return n, errs.Wrap("s.notes.Get", err)
}

// List pages the notes of authorID, newest first.
func (s *NoteService) List(ctx context.Context, authorID, cursor uuid.UUID, size int) (NotePage, error) {
	if size <= 0 || size > MaxPageSize {
		size = DefaultPageSize
	}

	// One extra tells whether there is a next page.
	found, err := s.notes.List(ctx, authorID, cursor, size+1)
	if err != nil {
		return NotePage{}, errs.Wrap("s.notes.List", err)
	}

	page := NotePage{Notes: found}
	if len(found) > size {
		page.Notes = found[:size]
		page.NextCursor = found[size-1].ID()
	}

	if page.Total, err = s.notes.Count(ctx, authorID); err != nil {
		return NotePage{}, errs.Wrap("s.notes.Count", err)
	}

	return page, nil
}

// Change gives a note another title and text: only its author may.
func (s *NoteService) Change(ctx context.Context, requesterID, id uuid.UUID, title post.NoteTitle, text post.Text) (*post.Note, error) {
	n, err := s.notes.Get(ctx, id)
	if err != nil {
		return nil, errs.Wrap("s.notes.Get", err)
	}

	edited, err := n.Edited(requesterID, title, text, s.now())
	if err != nil {
		return nil, errs.Wrap("n.Edited", err)
	}

	if err = s.notes.Update(ctx, edited); err != nil {
		return nil, errs.Wrap("s.notes.Update", err)
	}

	return edited, nil
}

// Delete deletes a note with its comments, by its author; the
// notifications about the comments go with it.
func (s *NoteService) Delete(ctx context.Context, requesterID, id uuid.UUID) error {
	n, err := s.notes.Get(ctx, id)
	if err != nil {
		return errs.Wrap("s.notes.Get", err)
	}

	if !n.CanBeChangedBy(requesterID) {
		return post.ErrCannotChangeNote
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.notes.Delete(ctx, id); err != nil {
			return errs.Wrap("s.notes.Delete", err)
		}

		return errs.Wrap("s.notifier.ForgetPost", s.notifier.ForgetPost(ctx, id))
	})
	if err != nil {
		return errs.Wrap("s.tx.WithinTx", err)
	}

	return nil
}

// Comment adds a comment under a note; its author hears about it
// ("оставил комментарий под вашей заметкой" of openvk), but not about
// their own. Who may (the privacy of the notes, a blacklist) the caller
// has checked; post.ErrNoteNotFound for a missing note.
func (s *NoteService) Comment(ctx context.Context, authorID, noteID uuid.UUID, text post.CommentText) (*post.NoteComment, error) {
	n, err := s.notes.Get(ctx, noteID)
	if err != nil {
		return nil, errs.Wrap("s.notes.Get", err)
	}

	c, err := post.NewNoteComment(noteID, authorID, text, s.now())
	if err != nil {
		return nil, errs.Wrap("post.NewNoteComment", err)
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.comments.Save(ctx, c); err != nil {
			return errs.Wrap("s.comments.Save", err)
		}

		if n.AuthorID() == authorID {
			return nil
		}

		e := Event{Kind: EventNoteComment, RecipientID: n.AuthorID(), ActorID: authorID, PostID: noteID, CommentID: c.ID()}

		return errs.Wrap("s.notifier.Notify", s.notifier.Notify(ctx, e))
	})
	if err != nil {
		return nil, errs.Wrap("s.tx.WithinTx", err)
	}

	return c, nil
}

// Comments pages the comments of a note, oldest first, as openvk shows
// them; post.ErrNoteNotFound for a missing note.
func (s *NoteService) Comments(ctx context.Context, noteID, after uuid.UUID, size int) (NoteCommentPage, error) {
	if size <= 0 || size > MaxPageSize {
		size = DefaultPageSize
	}

	n, err := s.notes.Get(ctx, noteID)
	if err != nil {
		return NoteCommentPage{}, errs.Wrap("s.notes.Get", err)
	}

	found, err := s.comments.List(ctx, noteID, after, size+1)
	if err != nil {
		return NoteCommentPage{}, errs.Wrap("s.comments.List", err)
	}

	page := NoteCommentPage{Comments: found, Total: n.Comments()}
	if len(found) > size {
		page.Comments = found[:size]
		page.NextCursor = found[size-1].ID()
	}

	return page, nil
}

// CommentByID is one comment of a note, whole.
func (s *NoteService) CommentByID(ctx context.Context, id uuid.UUID) (*post.NoteComment, error) {
	c, err := s.comments.Get(ctx, id)

	return c, errs.Wrap("s.comments.Get", err)
}

// CommentsByID returns the comments of notes with these IDs that exist,
// at most MaxPageSize, in no particular order.
func (s *NoteService) CommentsByID(ctx context.Context, ids []uuid.UUID) ([]*post.NoteComment, error) {
	if len(ids) > MaxPageSize {
		return nil, ErrTooManyIDs
	}

	if len(ids) == 0 {
		return nil, nil
	}

	found, err := s.comments.GetMany(ctx, ids)

	return found, errs.Wrap("s.comments.GetMany", err)
}

// DeleteComment deletes a comment: by its author or the author of the
// note; its notification goes with it.
func (s *NoteService) DeleteComment(ctx context.Context, requesterID, id uuid.UUID) error {
	c, err := s.comments.Get(ctx, id)
	if err != nil {
		return errs.Wrap("s.comments.Get", err)
	}

	n, err := s.notes.Get(ctx, c.NoteID())
	if err != nil {
		return errs.Wrap("s.notes.Get", err)
	}

	if !c.CanBeDeletedBy(requesterID, n) {
		return post.ErrCannotDeleteNoteComment
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.comments.Delete(ctx, id); err != nil {
			return errs.Wrap("s.comments.Delete", err)
		}

		return errs.Wrap("s.notifier.ForgetComment", s.notifier.ForgetComment(ctx, id))
	})
	if err != nil {
		return errs.Wrap("s.tx.WithinTx", err)
	}

	return nil
}
