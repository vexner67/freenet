package model

import "time"

// Guest is a visit to the owner's page ("Гости"): who came and when last.
type Guest struct {
	UserID    string
	VisitedAt time.Time
}

// GuestPage is a page of guests, the last visit first; SeenAt is when the
// owner looked before it (zero for never): newer visits are new.
type GuestPage struct {
	Guests     []Guest
	SeenAt     time.Time
	NextCursor string
}

// TopEntry is a place of a board of "Рейтинги": votes, or the rating of
// a pet with its kind and name on the board of pets.
type TopEntry struct {
	UserID  string
	Value   int64
	PetKind string
	PetName string
}

// PetShop is what pets cost: each kind, a meal; the votes a won fight
// brings and the fights of a day.
type PetShop struct {
	Prices map[string]int64
	// Cases are what each case costs; nil leaves them as they are.
	Cases  map[string]int64
	Meal   int64
	Win    int64
	Fights int
}

// Pet is a pet as it is now ("Питомец"): satiety and joy from 0 to 100,
// when it may play again (zero — now), its level with the experience of
// it and of the next (0 at the last), how it fights and the fights of the
// day.
type Pet struct {
	ID                  string
	Kind                string
	Name                string
	BornAt              time.Time
	Satiety             int
	Joy                 int
	PlayAt              time.Time
	Experience          int64
	Level               int
	LevelExperience     int64
	NextLevelExperience int64
	Rating              int
	Wins                int
	Losses              int
	Fights              int
	Attack              int
	Defense             int
	Health              int
	Speed               int
	Crit                int
	Rarity              string
	// Wardrobe are the clothes bought for it, Outfit those it wears.
	Wardrobe []string
	Outfit   []string
	// Breed and Clothes are what its kind and its clothes give of its
	// stats; the rest is levels.
	Breed   PetPower
	Clothes PetPower
}

// PetPower are the stats of a pet, or what something adds to them.
type PetPower struct {
	Attack  int
	Defense int
	Health  int
	Speed   int
}

// PetKind is a kind of the catalogue: how rare it is, whether the shop
// sells it, its stats at level 1 and its chance to hit twice as hard.
type PetKind struct {
	Kind   string
	Rarity string
	InShop bool
	Stats  PetPower
	Crit   int
}

// PetCase is a case: the odds in a thousand of each rarity, from the most
// common.
type PetCase struct {
	ID   string
	Odds []int
}

// PetItem is a thing of clothes: its slot, price and what it adds.
type PetItem struct {
	ID    string
	Slot  string
	Price int64
	Bonus PetPower
}

// PetState is the shop and the pets of the caller, the oldest first, with
// the catalogue.
type PetState struct {
	Shop  PetShop
	Pets  []Pet
	Kinds []PetKind
	Cases []PetCase
	Items []PetItem
}

// PetCare is a pet after it was adopted, fed, played with or renamed, and
// the balance after a payment (0 when nothing was paid).
type PetCare struct {
	Pet     Pet
	Balance int64
}

// PetFighter is a pet as it came to a fight.
type PetFighter struct {
	Kind   string
	Name   string
	Level  int
	Health int
	Outfit []string
}

// PetHit is a turn of a fight: who hit, whether the other dodged or it
// was a crit, the damage and the health of both after it.
type PetHit struct {
	ByAttacker     bool
	Dodged         bool
	Crit           bool
	Damage         int
	AttackerHealth int
	DefenderHealth int
}

// PetFight is a fight of two pets: Won tells the attacker won, Rating the
// rating it won (above zero) or lost.
type PetFight struct {
	ID         string
	AttackerID string
	DefenderID string
	FoughtAt   time.Time
	Attacker   PetFighter
	Defender   PetFighter
	Hits       []PetHit
	Won        bool
	Rating     int
}

// PetFought is a fight of the caller's pet, the pet after it and the
// balance after a won fight (0 after a lost one).
type PetFought struct {
	Fight   PetFight
	Pet     Pet
	Balance int64
}

// PetStats is how the pets do: pets, fights, votes spent on them and won
// by them.
type PetStats struct {
	Pets   int64
	Fights int64
	Spent  int64
	Won    int64
}
