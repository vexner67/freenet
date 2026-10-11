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
	// usedAt is when the tokens were last issued: at the sign-in, then at
	// every refresh.
	usedAt time.Time
	// device is where they were issued to that last time.
	device Device
}

func New(id, userID uuid.UUID, refreshTokenHash string, device Device, now time.Time) (*Session, error) {
	return &Session{
		id:               id,
		userID:           userID,
		refreshTokenHash: refreshTokenHash,
		createdAt:        now,
		usedAt:           now,
		device:           device,
	}, nil
}

// Restore rebuilds a session from storage without side effects.
func Restore(id, userID uuid.UUID, refreshTokenHash string, createdAt, usedAt time.Time, device Device) *Session {
	return &Session{
		id:               id,
		userID:           userID,
		refreshTokenHash: refreshTokenHash,
		createdAt:        createdAt,
		usedAt:           usedAt,
		device:           device,
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

func (s *Session) UsedAt() time.Time {
	return s.usedAt
}

func (s *Session) Device() Device {
	return s.device
}

// Rotated is the session after a refresh: with the new token, used now
// from the device. The session itself is not changed, readers may hold it.
func (s *Session) Rotated(refreshTokenHash string, device Device, now time.Time) *Session {
	rotated := *s
	rotated.refreshTokenHash = refreshTokenHash
	rotated.device = device
	rotated.usedAt = now

	return &rotated
}

// LiveSince is the time a session must have been used after to be alive:
// its last refresh token, good for ttl, has expired otherwise.
func LiveSince(now time.Time, ttl time.Duration) time.Time {
	return now.Add(-ttl)
}
