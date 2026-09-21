package session

import (
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/auth/internal/errs"
)

type Session struct {
	id               uuid.UUID
	userID           uuid.UUID
	refreshTokenHash string
	createdAt        time.Time
}

func New(userID uuid.UUID, refreshTokenHash string) (*Session, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, errs.Wrap("uuid.NewV7", err)
	}

	return &Session{
		id:               id,
		userID:           userID,
		refreshTokenHash: refreshTokenHash,
		createdAt:        time.Now(),
	}, nil
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
