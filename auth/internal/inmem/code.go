package inmem

import (
	"context"
	"sync"
	"time"

	"github.com/vexner67/freenet/auth/internal/account"
	"github.com/vexner67/freenet/auth/internal/service"
)

type code struct {
	hash      string
	expiresAt time.Time
	attempts  int
}

type CodeRepository struct {
	codes map[account.Email]code
	mu    sync.Mutex
}

func NewCodeRepository() *CodeRepository {
	return &CodeRepository{
		codes: make(map[account.Email]code),
	}
}

func (r *CodeRepository) Save(_ context.Context, email account.Email, codeHash string, ttl time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// A new code replaces the previous one with its attempts, like SET in Redis.
	r.codes[email] = code{
		hash:      codeHash,
		expiresAt: time.Now().Add(ttl),
	}
	return nil
}

func (r *CodeRepository) Get(_ context.Context, email account.Email) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	c, ok := r.active(email)
	if !ok {
		return "", service.ErrCodeNotFound
	}

	return c.hash, nil
}

func (r *CodeRepository) IncrementAttempts(_ context.Context, email account.Email) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	c, ok := r.active(email)
	if !ok {
		return 0, service.ErrCodeNotFound
	}

	c.attempts++
	r.codes[email] = c

	return c.attempts, nil
}

func (r *CodeRepository) Delete(_ context.Context, email account.Email) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.active(email); !ok {
		return service.ErrCodeNotFound
	}

	delete(r.codes, email)
	return nil
}

// active returns an unexpired code, dropping an expired one;
// the caller holds the lock.
func (r *CodeRepository) active(email account.Email) (code, bool) {
	c, ok := r.codes[email]
	if !ok {
		return code{}, false
	}

	if !time.Now().Before(c.expiresAt) {
		delete(r.codes, email)
		return code{}, false
	}

	return c, true
}
