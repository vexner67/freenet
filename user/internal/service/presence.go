package service

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// PresenceRepository keeps when people last visited the site.
type PresenceRepository interface {
	// MarkSeen records the visits, when each person came; an older visit
	// than known, or one of nobody, changes nothing.
	MarkSeen(ctx context.Context, visits map[uuid.UUID]time.Time) error
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

// flushTimeout bounds the last write of the visits, when the service stops.
const flushTimeout = 5 * time.Second

// PresenceService knows who is on the site ("Онлайн", "был в сети").
//
// Everyone with the site open tells of it every couple of minutes. The
// visits are not written one by one: they wait here and Flush writes all
// that came since the last one by one statement. So a person shows as
// online a flush later, and visits not yet written are lost with the
// process, which the next ones make up for.
type PresenceService struct {
	repo PresenceRepository
	now  func() time.Time

	mu     sync.Mutex
	visits map[uuid.UUID]time.Time
}

func NewPresenceService(repo PresenceRepository) *PresenceService {
	return &PresenceService{
		repo:   repo,
		now:    time.Now,
		visits: map[uuid.UUID]time.Time{},
	}
}

// MarkOnline notes that id is on the site now; Flush records it.
func (s *PresenceService) MarkOnline(ctx context.Context, id uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.visits[id] = s.now().UTC()

	return nil
}

// Flush records the visits noted since the last one. Those it could not
// record wait for the next, unless newer ones of the same people came.
func (s *PresenceService) Flush(ctx context.Context) error {
	s.mu.Lock()
	visits := s.visits
	s.visits = map[uuid.UUID]time.Time{}
	s.mu.Unlock()

	if len(visits) == 0 {
		return nil
	}

	err := s.repo.MarkSeen(ctx, visits)
	if err == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for id, at := range visits {
		if at.After(s.visits[id]) {
			s.visits[id] = at
		}
	}

	return errs.Wrap("s.repo.MarkSeen", err)
}

// Run flushes every interval until ctx ends, and once more then; failed
// is told each failure, which the next flush makes up for.
func (s *PresenceService) Run(ctx context.Context, every time.Duration, failed func(err error)) {
	t := time.NewTicker(every)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			last, cancel := context.WithTimeout(context.WithoutCancel(ctx), flushTimeout)
			defer cancel()

			if err := s.Flush(last); err != nil {
				failed(err)
			}

			return
		case <-t.C:
		}

		if err := s.Flush(ctx); err != nil && ctx.Err() == nil {
			failed(err)
		}
	}
}

// Statistics counts the people of the site and those online now.
func (s *PresenceService) Statistics(ctx context.Context) (Statistics, error) {
	stats, err := s.repo.Count(ctx, s.now().UTC().Add(-user.OnlineFor))
	if err != nil {
		return Statistics{}, errs.Wrap("s.repo.Count", err)
	}

	return stats, nil
}
