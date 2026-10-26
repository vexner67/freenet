package inmem

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/auth/internal/service"
)

type ticket struct {
	userID    uuid.UUID
	expiresAt time.Time
	attempts  int
}

// TicketRepository keeps the sign-ins waiting for their second step, with
// a lifetime, as Redis does.
type TicketRepository struct {
	tickets map[string]ticket
	mu      sync.Mutex
}

func NewTicketRepository() *TicketRepository {
	return &TicketRepository{tickets: make(map[string]ticket)}
}

func (r *TicketRepository) Save(_ context.Context, hash string, userID uuid.UUID, ttl time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.tickets[hash] = ticket{userID: userID, expiresAt: time.Now().Add(ttl)}

	return nil
}

func (r *TicketRepository) Get(_ context.Context, hash string) (uuid.UUID, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	t, ok := r.active(hash)
	if !ok {
		return uuid.Nil, service.ErrTicketNotFound
	}

	return t.userID, nil
}

func (r *TicketRepository) IncrementAttempts(_ context.Context, hash string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	t, ok := r.active(hash)
	if !ok {
		return 0, service.ErrTicketNotFound
	}

	t.attempts++
	r.tickets[hash] = t

	return t.attempts, nil
}

func (r *TicketRepository) Delete(_ context.Context, hash string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.active(hash); !ok {
		return service.ErrTicketNotFound
	}

	delete(r.tickets, hash)

	return nil
}

// active returns an unexpired ticket, dropping an expired one; the caller
// holds the lock.
func (r *TicketRepository) active(hash string) (ticket, bool) {
	t, ok := r.tickets[hash]
	if !ok {
		return ticket{}, false
	}

	if !time.Now().Before(t.expiresAt) {
		delete(r.tickets, hash)
		return ticket{}, false
	}

	return t, true
}
