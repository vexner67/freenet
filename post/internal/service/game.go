package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/post/internal/post"
)

// MaxRecordPlayers is how many players the records of a game are looked
// among at once: one's friends and oneself.
const MaxRecordPlayers = 1000

// GameRepository keeps what HTML widgets remember of their players: one
// save and the best score of each player in each game.
type GameRepository interface {
	// SaveData keeps data as the save of userID in game, in place of the
	// one before.
	SaveData(ctx context.Context, game, userID uuid.UUID, data post.GameData, at time.Time) error
	// Data returns post.ErrGameSaveNotFound when there is none.
	Data(ctx context.Context, game, userID uuid.UUID) (post.GameData, error)
	// RecordScore keeps score if it beats the best of userID in game and
	// returns the best after.
	RecordScore(ctx context.Context, game, userID uuid.UUID, score post.GameScore, at time.Time) (post.GameScore, error)
	// Records returns the best scores of these players in game, the best
	// first, at most limit.
	Records(ctx context.Context, game uuid.UUID, players []uuid.UUID, limit int) ([]post.Record, error)
	// DeleteByUser forgets the saves and scores of a user.
	DeleteByUser(ctx context.Context, userID uuid.UUID) error
}

// GameDocuments finds the document a game is.
type GameDocuments interface {
	Get(ctx context.Context, id uuid.UUID) (*post.Document, error)
}

// GameService keeps the saves and the records of HTML widgets. The widget
// never learns who plays: the site asks for the player it has signed in.
type GameService struct {
	games     GameRepository
	documents GameDocuments
	now       func() time.Time
}

func NewGameService(games GameRepository, documents GameDocuments) *GameService {
	return &GameService{games: games, documents: documents, now: time.Now}
}

// game checks that id is an HTML widget.
func (s *GameService) game(ctx context.Context, id uuid.UUID) error {
	d, err := s.documents.Get(ctx, id)
	if err != nil {
		return errs.Wrap("s.documents.Get", err)
	}

	if !d.IsGame() {
		return post.ErrGameNotFound
	}

	return nil
}

// Save keeps data as the save of userID in game.
func (s *GameService) Save(ctx context.Context, userID, game uuid.UUID, data post.GameData) error {
	if err := s.game(ctx, game); err != nil {
		return errs.Wrap("s.game", err)
	}

	return errs.Wrap("s.games.SaveData", s.games.SaveData(ctx, game, userID, data, s.now()))
}

// Load returns the save of userID in game; found is false without one.
func (s *GameService) Load(ctx context.Context, userID, game uuid.UUID) (data post.GameData, found bool, err error) {
	if err = s.game(ctx, game); err != nil {
		return "", false, errs.Wrap("s.game", err)
	}

	data, err = s.games.Data(ctx, game, userID)
	if errors.Is(err, post.ErrGameSaveNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, errs.Wrap("s.games.Data", err)
	}

	return data, true, nil
}

// Score records a result of userID in game and returns their best.
func (s *GameService) Score(ctx context.Context, userID, game uuid.UUID, score post.GameScore) (post.GameScore, error) {
	if err := s.game(ctx, game); err != nil {
		return 0, errs.Wrap("s.game", err)
	}

	best, err := s.games.RecordScore(ctx, game, userID, score, s.now())

	return best, errs.Wrap("s.games.RecordScore", err)
}

// Records returns the best scores in game among players, the best first,
// at most limit (1-50, else DefaultPageSize).
func (s *GameService) Records(ctx context.Context, game uuid.UUID, players []uuid.UUID, limit int) ([]post.Record, error) {
	if len(players) > MaxRecordPlayers {
		return nil, ErrTooManyIDs
	}

	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	if err := s.game(ctx, game); err != nil {
		return nil, errs.Wrap("s.game", err)
	}

	if len(players) == 0 {
		return nil, nil
	}

	found, err := s.games.Records(ctx, game, players, limit)

	return found, errs.Wrap("s.games.Records", err)
}
