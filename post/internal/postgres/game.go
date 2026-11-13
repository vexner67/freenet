package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/post/internal/post"
)

// GameRepository keeps the saves and the best scores of HTML widgets.
type GameRepository struct {
	pool *pgxpool.Pool
}

func NewGameRepository(pool *pgxpool.Pool) *GameRepository {
	return &GameRepository{
		pool: pool,
	}
}

// conn is the transaction of ctx (service.TxManager), or the pool outside
// one.
func (r *GameRepository) conn(ctx context.Context) pgpool.Querier {
	return pgpool.Conn(ctx, r.pool)
}

const saveGameDataQuery = `
	INSERT INTO game_saves (
		game_id,
		user_id,
		data,
		updated_at
	)
	VALUES ($1, $2, $3, $4)
	ON CONFLICT (game_id, user_id) DO UPDATE
	SET data = EXCLUDED.data,
		updated_at = EXCLUDED.updated_at
`

func (r *GameRepository) SaveData(ctx context.Context, game, userID uuid.UUID, data post.GameData, at time.Time) error {
	_, err := r.conn(ctx).Exec(ctx, saveGameDataQuery, game, userID, data.String(), at.UTC())
	if err != nil {
		return errs.Wrap("r.conn.Exec", err)
	}

	return nil
}

const getGameDataQuery = `
	SELECT
		data
	FROM game_saves
	WHERE game_id = $1
		AND user_id = $2
`

func (r *GameRepository) Data(ctx context.Context, game, userID uuid.UUID) (post.GameData, error) {
	var data string

	err := r.conn(ctx).QueryRow(ctx, getGameDataQuery, game, userID).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", post.ErrGameSaveNotFound
	}
	if err != nil {
		return "", errs.Wrap("r.conn.QueryRow", err)
	}

	return post.GameData(data), nil
}

// The best stays: a lower score changes nothing, the row included.
const recordGameScoreQuery = `
	INSERT INTO game_scores (
		game_id,
		user_id,
		score,
		updated_at
	)
	VALUES ($1, $2, $3, $4)
	ON CONFLICT (game_id, user_id) DO UPDATE
	SET score = EXCLUDED.score,
		updated_at = EXCLUDED.updated_at
	WHERE game_scores.score < EXCLUDED.score
`

const getGameScoreQuery = `
	SELECT
		score
	FROM game_scores
	WHERE game_id = $1
		AND user_id = $2
`

func (r *GameRepository) RecordScore(ctx context.Context, game, userID uuid.UUID, score post.GameScore, at time.Time) (post.GameScore, error) {
	if _, err := r.conn(ctx).Exec(ctx, recordGameScoreQuery, game, userID, score.Int64(), at.UTC()); err != nil {
		return 0, errs.Wrap("r.conn.Exec", err)
	}

	var best int64
	if err := r.conn(ctx).QueryRow(ctx, getGameScoreQuery, game, userID).Scan(&best); err != nil {
		return 0, errs.Wrap("r.conn.QueryRow", err)
	}

	return post.GameScore(best), nil
}

const listGameRecordsQuery = `
	SELECT
		user_id,
		score,
		updated_at
	FROM game_scores
	WHERE game_id = $1
		AND user_id = ANY($2)
	ORDER BY score DESC, updated_at
	LIMIT $3
`

func (r *GameRepository) Records(ctx context.Context, game uuid.UUID, players []uuid.UUID, limit int) ([]post.Record, error) {
	rows, err := r.conn(ctx).Query(ctx, listGameRecordsQuery, game, players, limit)
	if err != nil {
		return nil, errs.Wrap("r.conn.Query", err)
	}
	defer rows.Close()

	var found []post.Record
	for rows.Next() {
		var record post.Record
		var score int64
		if err = rows.Scan(&record.UserID, &score, &record.UpdatedAt); err != nil {
			return nil, errs.Wrap("rows.Scan", err)
		}
		record.Score = post.GameScore(score)
		found = append(found, record)
	}

	return found, errs.Wrap("rows.Err", rows.Err())
}

const deleteGameSavesByUserQuery = `
	DELETE FROM game_saves
	WHERE user_id = $1
`

const deleteGameScoresByUserQuery = `
	DELETE FROM game_scores
	WHERE user_id = $1
`

func (r *GameRepository) DeleteByUser(ctx context.Context, userID uuid.UUID) error {
	for _, query := range []string{deleteGameSavesByUserQuery, deleteGameScoresByUserQuery} {
		if _, err := r.conn(ctx).Exec(ctx, query, userID); err != nil {
			return errs.Wrap("r.conn.Exec", err)
		}
	}

	return nil
}
