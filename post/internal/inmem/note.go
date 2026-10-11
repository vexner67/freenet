package inmem

import (
	"bytes"
	"context"
	"slices"
	"sync"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/post/internal/post"
)

// NoteRepository keeps notes and the comments under them over one state,
// as the tables do in PostgreSQL: a comment is counted in its note, and a
// deleted note takes its comments. Notes and comments handed out never
// change, so sharing pointers with callers is safe.
type NoteRepository struct {
	mu       sync.RWMutex
	notes    map[uuid.UUID]*post.Note
	comments map[uuid.UUID]*post.NoteComment
}

func NewNoteRepository() *NoteRepository {
	return &NoteRepository{
		notes:    make(map[uuid.UUID]*post.Note),
		comments: make(map[uuid.UUID]*post.NoteComment),
	}
}

func (r *NoteRepository) Save(ctx context.Context, n *post.Note) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.notes[n.ID()] = n

	return nil
}

func (r *NoteRepository) Get(ctx context.Context, id uuid.UUID) (*post.Note, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	n, ok := r.notes[id]
	if !ok {
		return nil, post.ErrNoteNotFound
	}

	return n, nil
}

// Update keeps the title, the text and the time of the change: the
// comments are counted by the comments themselves.
func (r *NoteRepository) Update(ctx context.Context, n *post.Note) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	kept, ok := r.notes[n.ID()]
	if !ok {
		return post.ErrNoteNotFound
	}

	r.notes[n.ID()] = n.WithComments(kept.Comments())

	return nil
}

func (r *NoteRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.notes[id]; !ok {
		return post.ErrNoteNotFound
	}

	r.drop(id)

	return nil
}

// drop deletes a note with its comments; the caller holds the lock.
func (r *NoteRepository) drop(id uuid.UUID) {
	delete(r.notes, id)

	for commentID, c := range r.comments {
		if c.NoteID() == id {
			delete(r.comments, commentID)
		}
	}
}

// byAuthor returns the notes of authorID, newest first; the caller holds
// a lock.
func (r *NoteRepository) byAuthor(authorID uuid.UUID) []*post.Note {
	var found []*post.Note
	for _, n := range r.notes {
		if n.AuthorID() == authorID {
			found = append(found, n)
		}
	}

	// UUIDv7 bytes sort by time, as in PostgreSQL.
	slices.SortFunc(found, func(a, b *post.Note) int {
		ai, bi := a.ID(), b.ID()
		return bytes.Compare(bi[:], ai[:])
	})

	return found
}

func (r *NoteRepository) List(ctx context.Context, authorID, before uuid.UUID, limit int) ([]*post.Note, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var found []*post.Note
	for _, n := range r.byAuthor(authorID) {
		id := n.ID()
		if before == uuid.Nil || bytes.Compare(id[:], before[:]) < 0 {
			found = append(found, n)
		}
	}

	return found[:min(limit, len(found))], nil
}

func (r *NoteRepository) Count(ctx context.Context, authorID uuid.UUID) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.byAuthor(authorID)), nil
}

// DeleteByUser deletes the notes of a user with their comments and the
// comments the user left under others' notes, uncounting those.
func (r *NoteRepository) DeleteByUser(ctx context.Context, userID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for id, c := range r.comments {
		if c.AuthorID() == userID {
			r.uncount(id)
		}
	}

	for id, n := range r.notes {
		if n.AuthorID() == userID {
			r.drop(id)
		}
	}

	return nil
}

// uncount deletes a comment and takes it off its note's counter; the
// caller holds the lock.
func (r *NoteRepository) uncount(id uuid.UUID) {
	c := r.comments[id]
	delete(r.comments, id)

	if n, ok := r.notes[c.NoteID()]; ok {
		r.notes[n.ID()] = n.WithComments(n.Comments() - 1)
	}
}

// NoteCommentRepository is the service.NoteCommentRepository side of
// NoteRepository.
type NoteCommentRepository struct {
	r *NoteRepository
}

// Comments is the same repository as service.NoteCommentRepository.
func (r *NoteRepository) Comments() *NoteCommentRepository {
	return &NoteCommentRepository{r: r}
}

func (c *NoteCommentRepository) Save(ctx context.Context, nc *post.NoteComment) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	c.r.mu.Lock()
	defer c.r.mu.Unlock()

	n, ok := c.r.notes[nc.NoteID()]
	if !ok {
		return post.ErrNoteNotFound
	}

	c.r.comments[nc.ID()] = nc
	c.r.notes[n.ID()] = n.WithComments(n.Comments() + 1)

	return nil
}

func (c *NoteCommentRepository) Get(ctx context.Context, id uuid.UUID) (*post.NoteComment, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	c.r.mu.RLock()
	defer c.r.mu.RUnlock()

	nc, ok := c.r.comments[id]
	if !ok {
		return nil, post.ErrCommentNotFound
	}

	return nc, nil
}

func (c *NoteCommentRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	c.r.mu.Lock()
	defer c.r.mu.Unlock()

	if _, ok := c.r.comments[id]; !ok {
		return post.ErrCommentNotFound
	}

	c.r.uncount(id)

	return nil
}

func (c *NoteCommentRepository) List(ctx context.Context, noteID, after uuid.UUID, limit int) ([]*post.NoteComment, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	c.r.mu.RLock()
	defer c.r.mu.RUnlock()

	var found []*post.NoteComment
	for _, nc := range c.r.comments {
		id := nc.ID()
		if nc.NoteID() == noteID && bytes.Compare(id[:], after[:]) > 0 {
			found = append(found, nc)
		}
	}

	slices.SortFunc(found, func(a, b *post.NoteComment) int {
		ai, bi := a.ID(), b.ID()
		return bytes.Compare(ai[:], bi[:])
	})

	return found[:min(limit, len(found))], nil
}

func (c *NoteCommentRepository) GetMany(ctx context.Context, ids []uuid.UUID) ([]*post.NoteComment, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	c.r.mu.RLock()
	defer c.r.mu.RUnlock()

	var found []*post.NoteComment
	for _, id := range ids {
		if nc, ok := c.r.comments[id]; ok {
			found = append(found, nc)
		}
	}

	return found, nil
}
