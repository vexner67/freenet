package grpc

import (
	"context"

	"github.com/google/uuid"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type GameService interface {
	Save(ctx context.Context, userID, game uuid.UUID, data post.GameData) error
	Load(ctx context.Context, userID, game uuid.UUID) (post.GameData, bool, error)
	Score(ctx context.Context, userID, game uuid.UUID, score post.GameScore) (post.GameScore, error)
	Records(ctx context.Context, game uuid.UUID, players []uuid.UUID, limit int) ([]post.Record, error)
}

// SaveGame keeps what an HTML widget remembers of the caller.
func (h *PostHandler) SaveGame(ctx context.Context, req *postv1.SaveGameRequest) (*emptypb.Empty, error) {
	userID, game, err := gameCall(ctx, req.GetGameId())
	if err != nil {
		return nil, err
	}

	data, err := post.NewGameData(req.GetData())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if err = h.gameService.Save(ctx, userID, game, data); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

// LoadGame returns what an HTML widget saved of the caller.
func (h *PostHandler) LoadGame(ctx context.Context, req *postv1.LoadGameRequest) (*postv1.LoadGameResponse, error) {
	userID, game, err := gameCall(ctx, req.GetGameId())
	if err != nil {
		return nil, err
	}

	data, found, err := h.gameService.Load(ctx, userID, game)
	if err != nil {
		return nil, err
	}

	return &postv1.LoadGameResponse{Found: found, Data: data.String()}, nil
}

// RecordGameScore keeps a result of the caller in an HTML widget.
func (h *PostHandler) RecordGameScore(ctx context.Context, req *postv1.RecordGameScoreRequest) (*postv1.RecordGameScoreResponse, error) {
	userID, game, err := gameCall(ctx, req.GetGameId())
	if err != nil {
		return nil, err
	}

	score, err := post.NewGameScore(req.GetScore())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	best, err := h.gameService.Score(ctx, userID, game, score)
	if err != nil {
		return nil, err
	}

	return &postv1.RecordGameScoreResponse{Best: best.Int64()}, nil
}

// ListGameRecords returns the best results of the players the caller asks
// for (their friends and themselves), the best first.
func (h *PostHandler) ListGameRecords(ctx context.Context, req *postv1.ListGameRecordsRequest) (*postv1.ListGameRecordsResponse, error) {
	game, err := parseDocumentID(req.GetGameId())
	if err != nil {
		return nil, err
	}

	if len(req.GetUserIds()) > service.MaxRecordPlayers {
		return nil, status.Error(codes.InvalidArgument, "too many ids")
	}

	players := make([]uuid.UUID, 0, len(req.GetUserIds()))
	for _, s := range req.GetUserIds() {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid id")
		}
		players = append(players, id)
	}

	records, err := h.gameService.Records(ctx, game, players, int(req.GetLimit()))
	if err != nil {
		return nil, err
	}

	resp := &postv1.ListGameRecordsResponse{Records: make([]*postv1.GameRecord, 0, len(records))}
	for _, r := range records {
		resp.Records = append(resp.Records, &postv1.GameRecord{
			UserId:    r.UserID.String(),
			Score:     r.Score.Int64(),
			UpdatedAt: timestamppb.New(r.UpdatedAt),
		})
	}

	return resp, nil
}

// gameCall reads who plays and which game.
func gameCall(ctx context.Context, rawGame string) (userID, game uuid.UUID, err error) {
	if userID, err = currentUserID(ctx); err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	if game, err = parseDocumentID(rawGame); err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	return userID, game, nil
}
