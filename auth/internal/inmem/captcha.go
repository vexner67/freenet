package inmem

import (
	"context"
	"sync"
	"time"

	"github.com/vexner67/freenet/auth/internal/service"
)

type captcha struct {
	hash      string
	expiresAt time.Time
}

// CaptchaRepository keeps the pictures for a lifetime, as Redis does.
type CaptchaRepository struct {
	pictures map[string]captcha
	mu       sync.Mutex
}

func NewCaptchaRepository() *CaptchaRepository {
	return &CaptchaRepository{pictures: make(map[string]captcha)}
}

func (r *CaptchaRepository) Save(_ context.Context, id, answerHash string, ttl time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	for key, c := range r.pictures {
		if now.After(c.expiresAt) {
			delete(r.pictures, key)
		}
	}
	r.pictures[id] = captcha{hash: answerHash, expiresAt: now.Add(ttl)}

	return nil
}

func (r *CaptchaRepository) Take(_ context.Context, id string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	c, ok := r.pictures[id]
	delete(r.pictures, id)
	if !ok || time.Now().After(c.expiresAt) {
		return "", service.ErrCaptcha
	}

	return c.hash, nil
}
