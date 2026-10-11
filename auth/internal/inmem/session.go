package inmem

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/auth/internal/service"
	"github.com/vexner67/freenet/auth/internal/session"
)

var ErrSessionAlreadyExists = errors.New("session already exists")

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

func (r *SessionRepository) Get(_ context.Context, id uuid.UUID) (*session.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	s, ok := r.sessions[id]
	if !ok {
		return nil, service.ErrSessionNotFound
	}

	return s, nil
}

func (r *SessionRepository) Rotate(_ context.Context, rotated *session.Session, oldHash string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	s, ok := r.sessions[rotated.ID()]
	if !ok || s.RefreshTokenHash() != oldHash {
		return service.ErrSessionNotFound
	}

	r.sessions[rotated.ID()] = rotated
	return nil
}

func (r *SessionRepository) ListByUser(_ context.Context, userID uuid.UUID, usedAfter time.Time, limit int) ([]*session.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var found []*session.Session
	for _, s := range r.sessions {
		if s.UserID() == userID && s.UsedAt().After(usedAfter) {
			found = append(found, s)
		}
	}

	// The latest used first; the ID settles those used at once.
	slices.SortFunc(found, func(a, b *session.Session) int {
		if c := b.UsedAt().Compare(a.UsedAt()); c != 0 {
			return c
		}

		x, y := a.ID(), b.ID()

		return bytes.Compare(y[:], x[:])
	})

	return found[:min(len(found), limit)], nil
}

func (r *SessionRepository) DeleteOwned(_ context.Context, userID, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if s, ok := r.sessions[id]; !ok || s.UserID() != userID {
		return service.ErrSessionNotFound
	}

	delete(r.sessions, id)
	return nil
}

func (r *SessionRepository) DeleteOthers(_ context.Context, userID, keep uuid.UUID) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	deleted := 0
	for id, s := range r.sessions {
		if s.UserID() == userID && id != keep {
			delete(r.sessions, id)
			deleted++
		}
	}

	return deleted, nil
}

func (r *SessionRepository) Delete(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.sessions[id]; !ok {
		return service.ErrSessionNotFound
	}

	delete(r.sessions, id)
	return nil
}
