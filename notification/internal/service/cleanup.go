package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/notification/internal/notification"
	"github.com/vexner67/freenet/pkg/errs"
)

// cleanupBatch is how many notifications one statement deletes: a table
// of years is not emptied in one long transaction.
const cleanupBatch = 1000

// OldNotificationDeleter deletes the notifications with IDs below before,
// at most limit of them, oldest first, and tells how many.
type OldNotificationDeleter interface {
	DeleteOlder(ctx context.Context, before uuid.UUID, limit int) (int, error)
}

// CleanupService forgets the notifications nobody will scroll down to:
// those older than the retention, read or not.
type CleanupService struct {
	repo      OldNotificationDeleter
	retention time.Duration
	now       func() time.Time
}

func NewCleanupService(repo OldNotificationDeleter, retention time.Duration) *CleanupService {
	return &CleanupService{
		repo:      repo,
		retention: retention,
		now:       time.Now,
	}
}

// DeleteOld deletes every notification older than the retention, a batch
// at a time, and returns how many it deleted.
func (s *CleanupService) DeleteOld(ctx context.Context) (int, error) {
	before := notification.Before(s.now().Add(-s.retention))
	total := 0

	for {
		deleted, err := s.repo.DeleteOlder(ctx, before, cleanupBatch)
		if err != nil {
			return total, errs.Wrap("s.repo.DeleteOlder", err)
		}

		total += deleted
		if deleted < cleanupBatch {
			return total, nil
		}
	}
}
