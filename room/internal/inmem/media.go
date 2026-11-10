package inmem

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/room/internal/service"
)

// Media records commands in service tests; development uses the real LiveKit
// adapter even with in-memory storage, so a successful join can carry voice.
type Media struct {
	mu      sync.Mutex
	clock   service.Clock
	removed [][2]uuid.UUID
	deleted []uuid.UUID
	muted   [][2]uuid.UUID
}

func NewMedia(clock service.Clock) *Media { return &Media{clock: clock} }
func (m *Media) Token(_ context.Context, id, user uuid.UUID) (service.MediaToken, error) {
	return service.MediaToken{URL: "ws://localhost:7880", Token: id.String() + ":" + user.String(), ExpiresAt: m.clock.Now().Add(10 * time.Minute)}, nil
}
func (m *Media) Remove(_ context.Context, id, user uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removed = append(m.removed, [2]uuid.UUID{id, user})
	return nil
}
func (m *Media) Mute(_ context.Context, id, user uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.muted = append(m.muted, [2]uuid.UUID{id, user})
	return nil
}
func (m *Media) Delete(_ context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deleted = append(m.deleted, id)
	return nil
}
func (m *Media) Removed() [][2]uuid.UUID {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([][2]uuid.UUID(nil), m.removed...)
}
func (m *Media) Deleted() []uuid.UUID {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]uuid.UUID(nil), m.deleted...)
}
func (m *Media) Muted() [][2]uuid.UUID {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([][2]uuid.UUID(nil), m.muted...)
}
