package post

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

const (
	// MaxGameDataBytes is how much an HTML widget may keep for one player:
	// its save, as freenet.save of the sandbox hands it over.
	MaxGameDataBytes = 64 << 10
	// maxGameScore is the largest whole number a script counts exactly.
	maxGameScore = 1<<53 - 1
)

var (
	ErrGameNotFound     = errors.New("game not found")
	ErrGameSaveNotFound = errors.New("game save not found")
	ErrGameDataTooLarge = errors.New("a game save is at most 64 KB")
	ErrInvalidGameScore = errors.New("a score is a whole number from 0")
)

// GameData is what an HTML widget keeps for a player between runs, the
// text it was given (JSON, as the sandbox sends it).
type GameData string

func NewGameData(s string) (GameData, error) {
	if len(s) > MaxGameDataBytes {
		return "", ErrGameDataTooLarge
	}

	return GameData(s), nil
}

func (d GameData) String() string {
	return string(d)
}

// GameScore is a result a widget reports, freenet.score: the best one of
// each player is kept.
type GameScore int64

func NewGameScore(n int64) (GameScore, error) {
	if n < 0 || n > maxGameScore {
		return 0, ErrInvalidGameScore
	}

	return GameScore(n), nil
}

func (s GameScore) Int64() int64 {
	return int64(s)
}

// Better tells whether s beats the best so far.
func (s GameScore) Better(best GameScore) bool {
	return s > best
}

// Record is the best score of a player in a game.
type Record struct {
	UserID    uuid.UUID
	Score     GameScore
	UpdatedAt time.Time
}

// IsGame tells whether d is a game, an HTML widget, whose players' saves
// and scores are kept.
func (d *Document) IsGame() bool {
	return d.ext == DocumentHTML
}
