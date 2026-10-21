package inmem

import (
	"context"
	"sync"

	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

// CasinoRepository keeps the casino of the site in memory.
type CasinoRepository struct {
	mu     sync.RWMutex
	casino user.Casino
	bets   user.CasinoBets
	rocket user.CasinoRocket
	stats  user.CasinoStats
}

func NewCasinoRepository() *CasinoRepository {
	return &CasinoRepository{
		casino: user.DefaultCasino(),
		bets:   user.DefaultCasinoBets(),
		rocket: user.DefaultCasinoRocket(),
	}
}

func (r *CasinoRepository) Casino(ctx context.Context) (user.Casino, error) {
	if err := ctx.Err(); err != nil {
		return user.Casino{}, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.casino, nil
}

func (r *CasinoRepository) SetCasino(ctx context.Context, casino user.Casino) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.casino = casino

	return nil
}

func (r *CasinoRepository) CountSpin(ctx context.Context, spent, won int64) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.stats.Spins++
	r.stats.Spent += spent
	r.stats.Won += won

	return nil
}

func (r *CasinoRepository) Bets(ctx context.Context) (user.CasinoBets, error) {
	if err := ctx.Err(); err != nil {
		return user.CasinoBets{}, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.bets, nil
}

func (r *CasinoRepository) SetBets(ctx context.Context, bets user.CasinoBets) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.bets = bets

	return nil
}

func (r *CasinoRepository) CountBet(ctx context.Context, spent, won int64) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.stats.Wagers++
	r.stats.WagerSpent += spent
	r.stats.WagerWon += won

	return nil
}

func (r *CasinoRepository) Settings(ctx context.Context) (service.CasinoSettings, error) {
	if err := ctx.Err(); err != nil {
		return service.CasinoSettings{}, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return service.CasinoSettings{Casino: r.casino, Bets: r.bets, Rocket: r.rocket, Stats: r.stats}, nil
}

func (r *CasinoRepository) Rocket(ctx context.Context) (user.CasinoRocket, error) {
	if err := ctx.Err(); err != nil {
		return user.CasinoRocket{}, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.rocket, nil
}

func (r *CasinoRepository) SetRocket(ctx context.Context, rocket user.CasinoRocket) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.rocket = rocket

	return nil
}

func (r *CasinoRepository) CountRocket(ctx context.Context, rounds, spent, won int64) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.stats.Rockets += rounds
	r.stats.RocketSpent += spent
	r.stats.RocketWon += won

	return nil
}
