package user

import (
	"errors"
	"time"
)

// The rules of cases: the case that gives a legendary pet for sure, the
// cases a day until the administrators say and the most they may allow.
const (
	PetPity             = 40
	DefaultPetCasesADay = 20
	MaxPetCasesADay     = 1000
)

var (
	ErrPetDropWaiting  = errors.New("take or leave the pet of the case first")
	ErrNoPetDrop       = errors.New("there is no pet from a case")
	ErrPetCasesOver    = errors.New("you have opened enough cases today")
	ErrNotEnoughShards = errors.New("not enough shards")
)

// shards are what a pet of each rarity leaves when it is let go.
var shards = map[PetRarity]int{PetCommon: 1, PetRare: 3, PetEpic: 8, PetLegendary: 20}

// Shards are what a pet of r leaves when it is let go.
func (r PetRarity) Shards() int {
	return shards[r]
}

// caseShards are what each case costs in shards.
var caseShards = map[PetCase]int{PetCaseBronze: 10, PetCaseSilver: 25, PetCaseGold: 60}

// Shards are what c costs in shards.
func (c PetCase) Shards() int {
	return caseShards[c]
}

// PetKeeperState is what a person keeps of the cases: the shards, the
// cases opened since the last legendary pet, those of the day CasesOn
// and the pet a case gave, not taken yet.
type PetKeeperState struct {
	Shards   int
	Pity     int
	CasesOn  time.Time
	Cases    int
	Drop     PetKind
	DropCase PetCase
}

// PetKeeper is the cases of a person: a case gives a pet the person takes
// or leaves for shards, as a pet let go leaves them; shards open cases
// too. Each PetPity-th case without a legendary pet gives one.
type PetKeeper struct {
	s PetKeeperState
}

// NewPetKeeper is a keeper with nothing.
func NewPetKeeper() *PetKeeper {
	return &PetKeeper{}
}

func RestorePetKeeper(s PetKeeperState) *PetKeeper {
	return &PetKeeper{s: s}
}

// State is what storage keeps.
func (k *PetKeeper) State() PetKeeperState { return k.s }

func (k *PetKeeper) Shards() int       { return k.s.Shards }
func (k *PetKeeper) Pity() int         { return k.s.Pity }
func (k *PetKeeper) Drop() PetKind     { return k.s.Drop }
func (k *PetKeeper) DropCase() PetCase { return k.s.DropCase }

// CasesAt are the cases opened on the day of now.
func (k *PetKeeper) CasesAt(now time.Time) int {
	if !k.s.CasesOn.Equal(BonusDay(now)) {
		return 0
	}

	return k.s.Cases
}

// Opened is the keeper after the case c is opened at now, for shards with
// withShards, when a keeper has casesADay a day: its pet waits to be taken
// or left. rarity and kind draw it as PetCase.Draw does. ErrPetDropWaiting,
// ErrPetCasesOver, ErrNotEnoughShards.
func (k *PetKeeper) Opened(c PetCase, withShards bool, casesADay int, rarity, kind int64, now time.Time) (*PetKeeper, error) {
	switch {
	case k.s.Drop != "":
		return nil, ErrPetDropWaiting
	case k.CasesAt(now) >= casesADay:
		return nil, ErrPetCasesOver
	case withShards && k.s.Shards < c.Shards():
		return nil, ErrNotEnoughShards
	}

	s := k.s
	if withShards {
		s.Shards -= c.Shards()
	}

	drop := c.Draw(rarity, kind)
	if s.Pity+1 >= PetPity {
		legendary := PetKindsOf(PetLegendary)
		drop = legendary[kind%int64(len(legendary))]
	}

	s.Pity++
	if drop.Rarity() == PetLegendary {
		s.Pity = 0
	}

	s.Cases, s.CasesOn = k.CasesAt(now)+1, BonusDay(now)
	s.Drop, s.DropCase = drop, c

	return &PetKeeper{s: s}, nil
}

// Took is the keeper after the pet of the case is taken, and its kind;
// ErrNoPetDrop.
func (k *PetKeeper) Took() (*PetKeeper, PetKind, error) {
	if k.s.Drop == "" {
		return nil, "", ErrNoPetDrop
	}

	s := k.s
	s.Drop, s.DropCase = "", ""

	return &PetKeeper{s: s}, k.s.Drop, nil
}

// Left is the keeper after the pet of the case is left: it leaves its
// shards. ErrNoPetDrop.
func (k *PetKeeper) Left() (*PetKeeper, error) {
	taken, kind, err := k.Took()
	if err != nil {
		return nil, err
	}

	return taken.Released(kind), nil
}

// Released is the keeper after a pet of kind is let go: it leaves its
// shards.
func (k *PetKeeper) Released(kind PetKind) *PetKeeper {
	s := k.s
	s.Shards += kind.Rarity().Shards()

	return &PetKeeper{s: s}
}

// Earned is the keeper with shards more: of the tasks of the day, of the
// boss.
func (k *PetKeeper) Earned(shards int) *PetKeeper {
	s := k.s
	s.Shards += max(shards, 0)

	return &PetKeeper{s: s}
}
