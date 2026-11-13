package backend

import (
	"context"

	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/pkg/errs"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
)

// Games is rest.Games over the post service, which keeps the saves and
// scores of HTML widgets.
type Games struct {
	api postv1.PostServiceClient
}

func NewGames(api postv1.PostServiceClient) *Games {
	return &Games{
		api: api,
	}
}

func (g *Games) Save(ctx context.Context, accessToken, gameID, data string) error {
	if _, err := g.api.SaveGame(withToken(ctx, accessToken), &postv1.SaveGameRequest{GameId: gameID, Data: data}); err != nil {
		return errs.Wrap("g.api.SaveGame", fromStatus(err))
	}

	return nil
}

func (g *Games) Load(ctx context.Context, accessToken, gameID string) (string, bool, error) {
	resp, err := g.api.LoadGame(withToken(ctx, accessToken), &postv1.LoadGameRequest{GameId: gameID})
	if err != nil {
		return "", false, errs.Wrap("g.api.LoadGame", fromStatus(err))
	}

	return resp.GetData(), resp.GetFound(), nil
}

func (g *Games) Score(ctx context.Context, accessToken, gameID string, score int64) (int64, error) {
	resp, err := g.api.RecordGameScore(withToken(ctx, accessToken), &postv1.RecordGameScoreRequest{GameId: gameID, Score: score})
	if err != nil {
		return 0, errs.Wrap("g.api.RecordGameScore", fromStatus(err))
	}

	return resp.GetBest(), nil
}

func (g *Games) Records(ctx context.Context, accessToken, gameID string, userIDs []string, limit int) ([]model.GameRecord, error) {
	resp, err := g.api.ListGameRecords(withToken(ctx, accessToken), &postv1.ListGameRecordsRequest{GameId: gameID, UserIds: userIDs, Limit: toLimit(limit)})
	if err != nil {
		return nil, errs.Wrap("g.api.ListGameRecords", fromStatus(err))
	}

	records := make([]model.GameRecord, 0, len(resp.GetRecords()))
	for _, r := range resp.GetRecords() {
		records = append(records, model.GameRecord{UserID: r.GetUserId(), Score: r.GetScore()})
	}

	return records, nil
}
