package inmem

import (
	"cmp"
	"context"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/post/internal/post"
)

type gamePlayer struct {
	game, user uuid.UUID
}

// GameRepository keeps the saves and the best scores of HTML widgets.
type GameRepository struct {
	mu     sync.RWMutex
	saves  map[gamePlayer]post.GameData
	scores map[gamePlayer]post.Record
}

func NewGameRepository() *GameRepository {
	return &GameRepository{
		saves:  make(map[gamePlayer]post.GameData),
		scores: make(map[gamePlayer]post.Record),
	}
}

func (r *GameRepository) SaveData(ctx context.Context, game, userID uuid.UUID, data post.GameData, _ time.Time) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.saves[gamePlayer{game, userID}] = data

	return nil
}

func (r *GameRepository) Data(ctx context.Context, game, userID uuid.UUID) (post.GameData, error) {
	if err := ctx.Err(); err != nil {
		return "", errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	data, ok := r.saves[gamePlayer{game, userID}]
	if !ok {
		return "", post.ErrGameSaveNotFound
	}

	return data, nil
}

func (r *GameRepository) RecordScore(ctx context.Context, game, userID uuid.UUID, score post.GameScore, at time.Time) (post.GameScore, error) {
	if err := ctx.Err(); err != nil {
		return 0, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	key := gamePlayer{game, userID}
	best, ok := r.scores[key]
	if !ok || score.Better(best.Score) {
		best = post.Record{UserID: userID, Score: score, UpdatedAt: at.UTC().Truncate(time.Microsecond)}
		r.scores[key] = best
	}

	return best.Score, nil
}

func (r *GameRepository) Records(ctx context.Context, game uuid.UUID, players []uuid.UUID, limit int) ([]post.Record, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var found []post.Record
	for _, id := range players {
		if record, ok := r.scores[gamePlayer{game, id}]; ok && !slices.ContainsFunc(found, func(f post.Record) bool { return f.UserID == id }) {
			found = append(found, record)
		}
	}

	// The best first; of equal ones, who got there first.
	slices.SortFunc(found, func(a, b post.Record) int {
		if a.Score != b.Score {
			return cmp.Compare(b.Score, a.Score)
		}
		return a.UpdatedAt.Compare(b.UpdatedAt)
	})

	return found[:min(limit, len(found))], nil
}

func (r *GameRepository) DeleteByUser(ctx context.Context, userID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for key := range r.saves {
		if key.user == userID {
			delete(r.saves, key)
		}
	}
	for key := range r.scores {
		if key.user == userID {
			delete(r.scores, key)
		}
	}

	return nil
}
