package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/auth/internal/session"
	"github.com/vexner67/freenet/pkg/errs"
)

// MaxSessions is how many sessions the owner is shown, the latest used.
const MaxSessions = 100

// ErrNoSuchSession: the session to end is not one of the caller's.
var ErrNoSuchSession = errors.New("no such session")

// OwnedSessionRepository is the sessions as their owner manages them.
type OwnedSessionRepository interface {
	// ListByUser returns the user's sessions used after the time, the
	// latest used first, at most limit.
	ListByUser(ctx context.Context, userID uuid.UUID, usedAfter time.Time, limit int) ([]*session.Session, error)
	// DeleteOwned deletes the session if it is the user's, otherwise
	// returns ErrSessionNotFound.
	DeleteOwned(ctx context.Context, userID, id uuid.UUID) error
	// DeleteOthers deletes every session of the user but keep, returning
	// how many.
	DeleteOthers(ctx context.Context, userID, keep uuid.UUID) (int, error)
}

// SessionService is the owner's view of where they are signed in.
type SessionService struct {
	repo OwnedSessionRepository
	// ttl is how long a refresh token is good for: a session not used
	// for that long cannot be continued and is not shown.
	ttl time.Duration
	now func() time.Time
}

func NewSessionService(repo OwnedSessionRepository, ttl time.Duration) *SessionService {
	return &SessionService{
		repo: repo,
		ttl:  ttl,
		now:  time.Now,
	}
}

// List returns the sessions of the user that can still be continued.
func (s *SessionService) List(ctx context.Context, userID uuid.UUID) ([]*session.Session, error) {
	sessions, err := s.repo.ListByUser(ctx, userID, session.LiveSince(s.now(), s.ttl), MaxSessions)
	if err != nil {
		return nil, errs.Wrap("s.repo.ListByUser", err)
	}

	return sessions, nil
}

// End signs the user out of one of their sessions: its refresh token
// stops working, an access token already issued lives out its minutes.
func (s *SessionService) End(ctx context.Context, userID, sessionID uuid.UUID) error {
	err := s.repo.DeleteOwned(ctx, userID, sessionID)
	if errors.Is(err, ErrSessionNotFound) {
		return ErrNoSuchSession
	}
	if err != nil {
		return errs.Wrap("s.repo.DeleteOwned", err)
	}

	return nil
}

// EndOthers signs the user out everywhere but in the session they ask
// from, returning how many sessions ended.
func (s *SessionService) EndOthers(ctx context.Context, userID, current uuid.UUID) (int, error) {
	ended, err := s.repo.DeleteOthers(ctx, userID, current)
	if err != nil {
		return 0, errs.Wrap("s.repo.DeleteOthers", err)
	}

	return ended, nil
}
