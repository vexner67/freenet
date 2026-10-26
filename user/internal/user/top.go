package user

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// TopBoard is a list of "Рейтинги": who has the most of something.
type TopBoard string

const (
	// TopRich are the votes people hold, "Forbes Freenet".
	TopRich TopBoard = "rich"
	// TopMiners are the votes the mine paid in a week.
	TopMiners TopBoard = "miners"
	// TopBusiness are the votes businesses brought in a week, less what
	// went into them.
	TopBusiness TopBoard = "business"
	// TopCasino are the votes won in the casino in a week, less the bets.
	TopCasino TopBoard = "casino"
	// TopPets are the pets by their rating.
	TopPets TopBoard = "pets"
)

// TopBoards are all boards in the order the site shows them.
var TopBoards = []TopBoard{TopRich, TopMiners, TopBusiness, TopCasino, TopPets}

// TopWeek is the time the weekly boards count.
const TopWeek = 7 * 24 * time.Hour

// MaxTop is the most people a board holds.
const MaxTop = 50

var ErrUnknownTop = errors.New("unknown top")

func NewTopBoard(s string) (TopBoard, error) {
	for _, b := range TopBoards {
		if string(b) == s {
			return b, nil
		}
	}

	return "", ErrUnknownTop
}

func (b TopBoard) String() string {
	return string(b)
}

// Weekly tells whether the board counts the last TopWeek.
func (b TopBoard) Weekly() bool {
	return b == TopMiners || b == TopBusiness || b == TopCasino
}

// Kinds are the lines of the statement a weekly board sums.
func (b TopBoard) Kinds() []VoteKind {
	switch b {
	case TopMiners:
		return []VoteKind{VoteMine, VoteMineChest}
	case TopBusiness:
		return []VoteKind{VoteBusiness}
	case TopCasino:
		return []VoteKind{VoteCasino, VoteAim, VoteStake, VoteRocket, VotePoker}
	default:
		return nil
	}
}

// TopEntry is a place of a board: who and how much; for the pets — the
// rating, the kind and the name of the pet.
type TopEntry struct {
	UserID  uuid.UUID
	Value   int64
	PetKind PetKind
	PetName PetName
}
