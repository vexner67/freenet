package inmem

import (
	"context"
	"errors"
	"sync"
	"time"
)

var ErrCodeAlreadyExists = errors.New("code already exists")
var ErrCodeNotFound = errors.New("code not found")
var ErrCodeExpired = errors.New("code expired")

type code struct {
	hash      string
	createdAt time.Time
	ttl       time.Duration
}

type CodeRepository struct {
	codes map[string]code
	mu    sync.Mutex
}

func NewCodeRepository() *CodeRepository {
	return &CodeRepository{
		codes: make(map[string]code),
	}
}

func (r *CodeRepository) Save(_ context.Context, email, codeHash string, ttl time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.codes[email]; ok {
		return ErrSessionAlreadyExists
	}

	r.codes[email] = code{
		hash:      codeHash,
		createdAt: time.Now(),
		ttl:       ttl,
	}
	return nil
}

func (r *CodeRepository) Get(_ context.Context, email string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	c, ok := r.codes[email]
	if !ok {
		return "", ErrCodeNotFound
	}

	if c.createdAt.After(time.Now().Add(c.ttl)) {
		return "", ErrCodeExpired
	}

	return c.hash, nil
}

func (r *CodeRepository) Delete(_ context.Context, email string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.codes[email]; !ok {
		return ErrCodeNotFound
	}

	delete(r.codes, email)
	return nil
}
