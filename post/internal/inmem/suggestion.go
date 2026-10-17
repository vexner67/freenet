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

// SuggestionRepository keeps suggestions apart from posts, as the table
// does in PostgreSQL. Suggestions are immutable, so sharing pointers with
// callers is safe.
type SuggestionRepository struct {
	mu          sync.RWMutex
	suggestions map[uuid.UUID]*post.Suggestion
}

func NewSuggestionRepository() *SuggestionRepository {
	return &SuggestionRepository{
		suggestions: make(map[uuid.UUID]*post.Suggestion),
	}
}

func (r *SuggestionRepository) Save(ctx context.Context, s *post.Suggestion) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.suggestions[s.ID()] = s

	return nil
}

func (r *SuggestionRepository) Get(ctx context.Context, id uuid.UUID) (*post.Suggestion, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	s, ok := r.suggestions[id]
	if !ok {
		return nil, post.ErrSuggestionNotFound
	}

	return s, nil
}

func (r *SuggestionRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.suggestions[id]; !ok {
		return post.ErrSuggestionNotFound
	}

	delete(r.suggestions, id)

	return nil
}

// matching returns the suggestions for wallID, of authorID unless it is
// uuid.Nil, newest first; the caller holds a lock.
func (r *SuggestionRepository) matching(wallID, authorID uuid.UUID) []*post.Suggestion {
	var found []*post.Suggestion
	for _, s := range r.suggestions {
		if s.WallID() == wallID && (authorID == uuid.Nil || s.AuthorID() == authorID) {
			found = append(found, s)
		}
	}

	// UUIDv7 bytes sort by time, as in PostgreSQL.
	slices.SortFunc(found, func(a, b *post.Suggestion) int {
		ai, bi := a.ID(), b.ID()
		return bytes.Compare(bi[:], ai[:])
	})

	return found
}

func (r *SuggestionRepository) List(ctx context.Context, wallID, authorID, before uuid.UUID, limit int) ([]*post.Suggestion, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var found []*post.Suggestion
	for _, s := range r.matching(wallID, authorID) {
		id := s.ID()
		if before == uuid.Nil || bytes.Compare(id[:], before[:]) < 0 {
			found = append(found, s)
		}
	}

	return found[:min(limit, len(found))], nil
}

func (r *SuggestionRepository) Count(ctx context.Context, wallID, authorID uuid.UUID) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.matching(wallID, authorID)), nil
}

func (r *SuggestionRepository) DeleteByUser(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	var deleted []uuid.UUID

	for id, s := range r.suggestions {
		if s.AuthorID() == userID {
			delete(r.suggestions, id)
			deleted = append(deleted, id)
		}
	}

	return deleted, nil
}

func (r *SuggestionRepository) DeleteOnWall(ctx context.Context, wallID uuid.UUID) ([]*post.Suggestion, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	var deleted []*post.Suggestion

	for id, s := range r.suggestions {
		if s.WallID() == wallID {
			delete(r.suggestions, id)
			deleted = append(deleted, s)
		}
	}

	return deleted, nil
}
