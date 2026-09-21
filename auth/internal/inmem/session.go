package inmem

import (
	"context"
	"errors"
	"sync"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/auth/internal/session"
)

var ErrSessionAlreadyExists = errors.New("session already exists")
var ErrSessionNotFound = errors.New("session not found")

type SessionRepository struct {
	sessions map[uuid.UUID]*session.Session
	mu       sync.Mutex
}

func NewSessionRepository() *SessionRepository {
	return &SessionRepository{
		sessions: make(map[uuid.UUID]*session.Session),
	}
}

func (r *SessionRepository) Save(_ context.Context, s *session.Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.sessions[s.ID()]; ok {
		return ErrSessionAlreadyExists
	}

	r.sessions[s.ID()] = s
	return nil
}

func (r *SessionRepository) Delete(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.sessions[id]; !ok {
		return ErrSessionNotFound
	}

	delete(r.sessions, id)
	return nil
}
