package session

import (
	"time"

	"github.com/google/uuid"
)

type Session struct {
	id               uuid.UUID
	userID           uuid.UUID
	refreshTokenHash string
	createdAt        time.Time
}

func New(id, userID uuid.UUID, refreshTokenHash string) (*Session, error) {
	return &Session{
		id:               id,
		userID:           userID,
		refreshTokenHash: refreshTokenHash,
		createdAt:        time.Now(),
	}, nil
}

// Restore rebuilds a session from storage without side effects.
func Restore(id, userID uuid.UUID, refreshTokenHash string, createdAt time.Time) *Session {
	return &Session{
		id:               id,
		userID:           userID,
		refreshTokenHash: refreshTokenHash,
		createdAt:        createdAt,
	}
}

func (s *Session) ID() uuid.UUID {
	return s.id
}

func (s *Session) UserID() uuid.UUID {
	return s.userID
}

func (s *Session) RefreshTokenHash() string {
	return s.refreshTokenHash
}

func (s *Session) CreatedAt() time.Time {
	return s.createdAt
}
