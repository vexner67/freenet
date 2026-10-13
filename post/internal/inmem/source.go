package inmem

import (
	"bytes"
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

func (r *SourceRepository) Sync(ctx context.Context, s service.SourceSync) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	// within tells whether id is in the stretch.
	within := func(id uuid.UUID) bool {
		if bytes.Compare(id[:], s.After[:]) <= 0 {
			return false
		}

		return s.Last || bytes.Compare(id[:], s.IDs[len(s.IDs)-1][:]) <= 0
	}

	if r.s.sources == nil {
		r.s.sources = make(map[uuid.UUID]map[uuid.UUID]bool)
	}

	if s.Club {
		for reader, read := range r.s.sources {
			if _, ok := read[s.Owner]; ok && within(reader) && !slices.Contains(s.IDs, reader) {
				delete(read, s.Owner)
			}
		}

		for _, reader := range s.IDs {
			if r.s.sources[reader] == nil {
				r.s.sources[reader] = make(map[uuid.UUID]bool)
			}

			r.s.sources[reader][s.Owner] = true
		}

		return nil
	}

	read := r.s.sources[s.Owner]
	if read == nil {
		read = make(map[uuid.UUID]bool)
		r.s.sources[s.Owner] = read
	}

	for source, club := range read {
		if !club && within(source) && !slices.Contains(s.IDs, source) {
			delete(read, source)
		}
	}

	for _, source := range s.IDs {
		read[source] = false
	}

	return nil
}

func (r *SourceRepository) Forget(ctx context.Context, userID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	delete(r.s.sources, userID)
	delete(r.s.closedWalls, userID)

	for _, read := range r.s.sources {
		delete(read, userID)
	}

	return nil
}

func (r *SourceRepository) Close(ctx context.Context, wallID uuid.UUID, club bool) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	if r.s.closedWalls == nil {
		r.s.closedWalls = make(map[uuid.UUID]bool)
	}

	r.s.closedWalls[wallID] = club

	return nil
}

func (r *SourceRepository) Open(ctx context.Context, wallID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	delete(r.s.closedWalls, wallID)

	return nil
}

func (r *SourceRepository) ClosedTo(ctx context.Context, viewerID uuid.UUID, wallIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	closed := make(map[uuid.UUID]bool)

	for _, id := range wallIDs {
		if r.s.wallClosedTo(id, viewerID) {
			closed[id] = true
		}
	}

	return closed, nil
}

// wallClosedTo is wall_closed_to of the migrations: a closed page is read
// by its owner and those the owner follows, a closed community by its
// members. The caller holds the lock.
func (s *Store) wallClosedTo(wallID, viewerID uuid.UUID) bool {
	club, closed := s.closedWalls[wallID]
	if !closed || wallID == viewerID {
		return false
	}

	if viewerID == uuid.Nil {
		return true
	}

	if club {
		_, member := s.sources[viewerID][wallID]

		return !member
	}

	followedClub, followed := s.sources[wallID][viewerID]

	return !followed || followedClub
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
