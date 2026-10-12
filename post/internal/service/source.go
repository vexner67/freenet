package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/post/internal/post"
)

// ErrSourceIsReader: nobody follows themselves; a reader's own posts are
// in their news anyway.
var ErrSourceIsReader = errors.New("a reader cannot be their own source")

// SourceRepository keeps whom each reader reads in the news: a copy of
// the follows of the user service and of the memberships of the club
// service, kept here so that the feed joins it. Add and Remove are
// idempotent; Forget drops an erased user from both sides.
//
// It also keeps which walls are closed: of a person with a closed
// profile, read by the owner and those the owner follows, and of a
// closed community, read by its members. Close and Open are idempotent;
// Forget opens the wall of an erased user too.
type SourceRepository interface {
	Add(ctx context.Context, readerID, sourceID uuid.UUID, club bool) error
	Remove(ctx context.Context, readerID, sourceID uuid.UUID) error
	Forget(ctx context.Context, userID uuid.UUID) error
	Close(ctx context.Context, wallID uuid.UUID, club bool) error
	Open(ctx context.Context, wallID uuid.UUID) error
	Walls
}

// Walls tells which of wallIDs are closed to viewerID (uuid.Nil: nobody
// signed in): those are in the answer.
type Walls interface {
	ClosedTo(ctx context.Context, viewerID uuid.UUID, wallIDs []uuid.UUID) (map[uuid.UUID]bool, error)
}

// closedTo tells whether the wall is closed to viewerID; without walls
// nothing is.
func closedTo(ctx context.Context, walls Walls, viewerID, wallID uuid.UUID) (bool, error) {
	if walls == nil {
		return false, nil
	}

	closed, err := walls.ClosedTo(ctx, viewerID, []uuid.UUID{wallID})
	if err != nil {
		return false, errs.Wrap("walls.ClosedTo", err)
	}

	return closed[wallID], nil
}

// readable are the posts whose walls viewerID reads, in their order.
func readable(ctx context.Context, walls Walls, viewerID uuid.UUID, posts []*post.Post) ([]*post.Post, error) {
	if walls == nil || len(posts) == 0 {
		return posts, nil
	}

	wallIDs := make([]uuid.UUID, 0, len(posts))
	for _, p := range posts {
		wallIDs = append(wallIDs, p.WallID())
	}

	closed, err := walls.ClosedTo(ctx, viewerID, wallIDs)
	if err != nil {
		return nil, errs.Wrap("walls.ClosedTo", err)
	}

	if len(closed) == 0 {
		return posts, nil
	}

	shown := make([]*post.Post, 0, len(posts))
	for _, p := range posts {
		if !closed[p.WallID()] {
			shown = append(shown, p)
		}
	}

	return shown, nil
}

// SourceService takes what the user and club services tell about follows
// and memberships.
type SourceService struct {
	sources SourceRepository
}

func NewSourceService(sources SourceRepository) *SourceService {
	return &SourceService{
		sources: sources,
	}
}

// Follow puts sourceID into the news of readerID: a person, with what
// they write anywhere and everything on their wall, or a club, with its
// wall.
func (s *SourceService) Follow(ctx context.Context, readerID, sourceID uuid.UUID, club bool) error {
	if readerID == sourceID {
		return ErrSourceIsReader
	}

	return errs.Wrap("s.sources.Add", s.sources.Add(ctx, readerID, sourceID, club))
}

// Unfollow takes it out again.
func (s *SourceService) Unfollow(ctx context.Context, readerID, sourceID uuid.UUID) error {
	return errs.Wrap("s.sources.Remove", s.sources.Remove(ctx, readerID, sourceID))
}

// CloseWall keeps a wall from everyone but those it is for: the wall of
// a person with a closed profile, or of a closed community (club).
func (s *SourceService) CloseWall(ctx context.Context, wallID uuid.UUID, club bool) error {
	return errs.Wrap("s.sources.Close", s.sources.Close(ctx, wallID, club))
}

// OpenWall lets everyone read it again.
func (s *SourceService) OpenWall(ctx context.Context, wallID uuid.UUID) error {
	return errs.Wrap("s.sources.Open", s.sources.Open(ctx, wallID))
}
