package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
)

// ErrSourceIsReader: nobody follows themselves; a reader's own posts are
// in their news anyway.
var ErrSourceIsReader = errors.New("a reader cannot be their own source")

// SourceRepository keeps whom each reader reads in the news: a copy of
// the follows of the user service and of the memberships of the club
// service, kept here so that the feed joins it. Add and Remove are
// idempotent; Forget drops an erased user from both sides.
type SourceRepository interface {
	Add(ctx context.Context, readerID, sourceID uuid.UUID, club bool) error
	Remove(ctx context.Context, readerID, sourceID uuid.UUID) error
	Forget(ctx context.Context, userID uuid.UUID) error
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
