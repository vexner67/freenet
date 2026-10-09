package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
)

// PresenceRepository keeps when people last visited the site.
type PresenceRepository interface {
	// MarkSeen records a visit of id at at; an older one than known, or of
	// nobody, changes nothing.
	MarkSeen(ctx context.Context, id uuid.UUID, at time.Time) error
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
