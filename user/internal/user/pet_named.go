package user

import (
	"errors"
	"time"
)

// PetBig2ZName is what the pet is called when given.
const PetBig2ZName PetName = "BIG 2Z"

// PetBig2ZOpensAt is when the pet can be taken: 22:30 in Moscow.
var PetBig2ZOpensAt = time.Date(2026, time.October, 30, 19, 30, 0, 0, time.UTC)

var (
	ErrPetLocked  = errors.New("the pet is still locked")
	ErrPetClaimed = errors.New("the pet is already taken")
)
