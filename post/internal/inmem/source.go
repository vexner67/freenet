package inmem

import (
	"context"
	"slices"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
)

// SourceRepository keeps whom each reader reads in the Store, where the
// feed looks it up.
type SourceRepository struct {
	s *Store
}

func (s *Store) Sources() *SourceRepository {
	return &SourceRepository{s: s}
}

func (r *SourceRepository) Add(ctx context.Context, readerID, sourceID uuid.UUID, club bool) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	if r.s.sources == nil {
		r.s.sources = make(map[uuid.UUID]map[uuid.UUID]bool)
	}

	if r.s.sources[readerID] == nil {
		r.s.sources[readerID] = make(map[uuid.UUID]bool)
	}

	r.s.sources[readerID][sourceID] = club

	return nil
}

func (r *SourceRepository) Remove(ctx context.Context, readerID, sourceID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	delete(r.s.sources[readerID], sourceID)

	return nil
}

func (r *SourceRepository) Forget(ctx context.Context, userID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	delete(r.s.sources, userID)

	for _, read := range r.s.sources {
		delete(read, userID)
	}

	return nil
}

// reads tells whether p is in the news of the reader of sources, as
// listFeedQuery does. The caller holds the lock.
func (s *Store) reads(sources service.FeedSources, p *post.Post) bool {
	read := s.sources[sources.Reader]

	if p.WallID() == sources.Reader {
		return true
	}

	if _, ok := read[p.WallID()]; ok {
		return true
	}

	if p.FromWall() || slices.Contains(sources.Ignored, p.AuthorID()) {
		return false
	}

	if p.AuthorID() == sources.Reader {
		return true
	}

	club, ok := read[p.AuthorID()]

	return ok && !club
}
