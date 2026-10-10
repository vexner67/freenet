package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// PresenceRepository keeps when people last visited the site.
type PresenceRepository interface {
	// MarkSeen records a visit of id at at; an older one than known, or of
	// nobody, changes nothing.
	MarkSeen(ctx context.Context, id uuid.UUID, at time.Time) error
	// Count counts the pages that are not deleted and, of them, those
	// whose owners visited since since.
	Count(ctx context.Context, since time.Time) (Statistics, error)
}

// Statistics are how many people the site has and how many of them are
// on it now, for its page "о сайте".
type Statistics struct {
	Users  int
	Online int
}

// PresenceService knows who is on the site ("Онлайн", "был в сети").
type PresenceService struct {
	repo PresenceRepository
	now  func() time.Time
}

func NewPresenceService(repo PresenceRepository) *PresenceService {
	return &PresenceService{repo: repo, now: time.Now}
}

// MarkOnline records that id is on the site now.
func (s *PresenceService) MarkOnline(ctx context.Context, id uuid.UUID) error {
	if err := s.repo.MarkSeen(ctx, id, s.now().UTC()); err != nil {
		return errs.Wrap("s.repo.MarkSeen", err)
	}

	return nil
}

// Statistics counts the people of the site and those online now.
func (s *PresenceService) Statistics(ctx context.Context) (Statistics, error) {
	stats, err := s.repo.Count(ctx, s.now().UTC().Add(-user.OnlineFor))
	if err != nil {
		return Statistics{}, errs.Wrap("s.repo.Count", err)
	}

	return stats, nil
}
