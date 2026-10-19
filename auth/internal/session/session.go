package session

import (
	"time"

	"github.com/google/uuid"
)

type Session struct {
	id               uuid.UUID
	userID           uuid.UUID
	refreshTokenHash string
	// previousHash is the hash of the token the current one replaced, or
	// empty. It stays good until the current one is used: the answer
	// carrying the current one may not have reached the client (a page
	// left while its refresh was on the way), and the client then knows
	// only the previous one.
	previousHash string
	createdAt    time.Time
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

// Hashes are the refresh token hashes of a stored session: its current
// token and the one that token replaced, or empty.
type Hashes struct {
	Current  string
	Previous string
}

// Restore rebuilds a session from storage without side effects.
func Restore(id, userID uuid.UUID, hashes Hashes, createdAt, usedAt time.Time, device Device) *Session {
	return &Session{
		id:               id,
		userID:           userID,
		refreshTokenHash: hashes.Current,
		previousHash:     hashes.Previous,
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

// PreviousTokenHash is the hash of the token the current one replaced,
// still good until the current one is used; empty if there is none.
func (s *Session) PreviousTokenHash() string {
	return s.previousHash
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

// Rotated is the session after a refresh with its current token: with
// the new token, used now from the device, and the token it was refreshed
// with as the previous one. The session itself is not changed, readers
// may hold it.
func (s *Session) Rotated(refreshTokenHash string, device Device, now time.Time) *Session {
	rotated := s.used(refreshTokenHash, device, now)
	rotated.previousHash = s.refreshTokenHash

	return rotated
}

// Reissued is the session after a refresh with its previous token: the
// client never got the current one, which nobody used, so a new token
// takes its place and the previous one stays as it is.
func (s *Session) Reissued(refreshTokenHash string, device Device, now time.Time) *Session {
	return s.used(refreshTokenHash, device, now)
}

func (s *Session) used(refreshTokenHash string, device Device, now time.Time) *Session {
	used := *s
	used.refreshTokenHash = refreshTokenHash
	used.device = device
	used.usedAt = now

	return &used
}

// LiveSince is the time a session must have been used after to be alive:
// its last refresh token, good for ttl, has expired otherwise.
func LiveSince(now time.Time, ttl time.Duration) time.Time {
	return now.Add(-ttl)
}
