package inmem

import (
	"context"
	"slices"
	"sync"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/auth/internal/twofactor"
)

type factor struct {
	secret   twofactor.Secret
	enabled  bool
	lastStep int64
	recovery []string
}

// TwoFactorRepository keeps the second steps of the pages in memory.
type TwoFactorRepository struct {
	factors map[uuid.UUID]factor
	mu      sync.Mutex
}

func NewTwoFactorRepository() *TwoFactorRepository {
	return &TwoFactorRepository{factors: make(map[uuid.UUID]factor)}
}

func (r *TwoFactorRepository) Get(_ context.Context, userID uuid.UUID) (*twofactor.Factor, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	f, ok := r.factors[userID]
	if !ok {
		return nil, twofactor.ErrNotSetUp
	}

	return twofactor.Restore(userID, slices.Clone(f.secret), f.enabled, f.lastStep, len(f.recovery)), nil
}

func (r *TwoFactorRepository) SetUp(_ context.Context, f *twofactor.Factor) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.factors[f.UserID()].enabled {
		return twofactor.ErrEnabled
	}

	r.factors[f.UserID()] = factor{secret: slices.Clone(f.Secret())}

	return nil
}

func (r *TwoFactorRepository) Enable(_ context.Context, userID uuid.UUID, step int64, recovery []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	f, ok := r.factors[userID]
	if !ok {
		return twofactor.ErrNotSetUp
	}
	if f.enabled {
		return twofactor.ErrInvalidCode
	}

	r.factors[userID] = factor{secret: f.secret, enabled: true, lastStep: step, recovery: slices.Clone(recovery)}

	return nil
}

func (r *TwoFactorRepository) Delete(_ context.Context, userID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.factors[userID]; !ok {
		return twofactor.ErrNotSetUp
	}

	delete(r.factors, userID)

	return nil
}

func (r *TwoFactorRepository) UseStep(_ context.Context, userID uuid.UUID, step int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	f, ok := r.factors[userID]
	if !ok || !f.enabled || step <= f.lastStep {
		return twofactor.ErrInvalidCode
	}

	f.lastStep = step
	r.factors[userID] = f

	return nil
}

func (r *TwoFactorRepository) UseRecovery(_ context.Context, userID uuid.UUID, hash string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	f, ok := r.factors[userID]
	i := slices.Index(f.recovery, hash)
	if !ok || !f.enabled || i < 0 {
		return twofactor.ErrInvalidCode
	}

	f.recovery = slices.Delete(slices.Clone(f.recovery), i, i+1)
	r.factors[userID] = f

	return nil
}

func (r *TwoFactorRepository) ReplaceRecovery(_ context.Context, userID uuid.UUID, step int64, recovery []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	f, ok := r.factors[userID]
	if !ok || !f.enabled || step <= f.lastStep {
		return twofactor.ErrInvalidCode
	}

	f.lastStep = step
	f.recovery = slices.Clone(recovery)
	r.factors[userID] = f

	return nil
}
