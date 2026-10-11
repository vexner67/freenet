package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// IgnoreRepository keeps the sources of the news feed a user ignores. A
// source is a wall: a user's or a community's, which the club service
// knows — so its ID is taken as it is.
type IgnoreRepository interface {
	// Ignore adds sourceID to what userID ignores unless limit sources
	// are ignored already: it tells whether the source is ignored now.
	// Ignoring twice is not an error.
	Ignore(ctx context.Context, userID, sourceID uuid.UUID, limit int) (bool, error)
	// Unignore is idempotent.
	Unignore(ctx context.Context, userID uuid.UUID, sourceIDs []uuid.UUID) error
	// Ignored returns up to limit ignored sources, newest first.
	Ignored(ctx context.Context, userID uuid.UUID, limit int) ([]uuid.UUID, error)
}

// IgnoreService is openvk's "Игнорируемые источники": the walls whose
// posts their reader's news feeds leave out.
type IgnoreService struct {
	ignoreRepo IgnoreRepository
}

func NewIgnoreService(ignoreRepo IgnoreRepository) *IgnoreService {
	return &IgnoreService{ignoreRepo: ignoreRepo}
}

// Ignore leaves the wall sourceID out of userID's news. Whether there is
// such a wall is the caller's to check: it may be a community's.
func (s *IgnoreService) Ignore(ctx context.Context, userID, sourceID uuid.UUID) error {
	if userID == sourceID {
		return user.ErrIgnoreSelf
	}

	ignored, err := s.ignoreRepo.Ignore(ctx, userID, sourceID, user.MaxIgnoredSources)
	if err != nil {
		return errs.Wrap("s.ignoreRepo.Ignore", err)
	}

	if !ignored {
		return user.ErrTooManyIgnored
	}

	return nil
}

func (s *IgnoreService) Unignore(ctx context.Context, userID uuid.UUID, sourceIDs []uuid.UUID) error {
	if len(sourceIDs) == 0 {
		return nil
	}

	if err := s.ignoreRepo.Unignore(ctx, userID, sourceIDs); err != nil {
		return errs.Wrap("s.ignoreRepo.Unignore", err)
	}

	return nil
}

// Ignored are the sources userID ignores, newest first.
func (s *IgnoreService) Ignored(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	ids, err := s.ignoreRepo.Ignored(ctx, userID, user.MaxIgnoredSources)
	if err != nil {
		return nil, errs.Wrap("s.ignoreRepo.Ignored", err)
	}

	return ids, nil
}
