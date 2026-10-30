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
	// Stance is how it defends, Power its stats in one number, Fatigue its
	// fights in a row.
	Stance  string
	Power   int
	Fatigue int
	// SalePrice is the votes it is up for on the market, 0 when it is
	// not; SaleAt when it may go up and BossAt when it may hit the boss
	// again, zero when it may now.
	SalePrice int64
	SaleAt    time.Time
	BossAt    time.Time
}

// PetPower are the stats of a pet, or what something adds to them.
type PetPower struct {
	Attack  int
	Defense int
	Health  int
	Speed   int
}

// PetKind is a kind of the catalogue: how rare it is, whether the shop
// sells it, its stats at level 1, its chance to hit twice as hard, its
// element and trick in a fight and the shards it leaves.
type PetKind struct {
	Kind    string
	Rarity  string
	InShop  bool
	Stats   PetPower
	Crit    int
	Element string
	Special string
	Shards  int
}

// NamedPet is the pet everyone is given: when it opens and whether the
// viewer took it.
type NamedPet struct {
	Kind    string
	OpensAt time.Time
	Claimed bool
}

// PetCase is a case: the odds in a thousand of each rarity, from the most
// common, and what it costs in shards.
type PetCase struct {
	ID     string
	Odds   []int
	Shards int
}

// PetKeeper is what the caller keeps of the cases: the shards, the cases
// since the last legendary pet and the one that gives it for sure, the
// cases of today and of a day, and the pet a case gave, not taken yet.
type PetKeeper struct {
	Shards     int
	Pity       int
	PityLimit  int
	Cases      int
	CasesLimit int
	DropKind   string
	DropCase   string
}

// PetDrop is the keeper after a case and the balance after it, 0 for one
// opened for shards.
type PetDrop struct {
	Keeper  PetKeeper
	Balance int64
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
	Shop    PetShop
	Pets    []Pet
	Kinds   []PetKind
	Cases   []PetCase
	Items   []PetItem
	Keeper  PetKeeper
	Battles []PetBattle
}

// PetCare is a pet after it was adopted, fed, played with or renamed, and
// the balance after a payment (0 when nothing was paid).
type PetCare struct {
	Pet     Pet
	Balance int64
}

// PetFighter is a pet as it came to a fight.
type PetFighter struct {
	Kind    string
	Name    string
	Level   int
	Health  int
	Outfit  []string
	Stance  string
	Power   int
	Fatigue int
}

// PetHit is an act of a fight: who acted and how, whether the other
// dodged or it was a crit, the damage, what the actor healed, its special
// and the special of the other that answered, and the health of both
// after it.
type PetHit struct {
	ByAttacker     bool
	Move           string
	Dodged         bool
	Crit           bool
	Damage         int
	Heal           int
	Special        string
	Answer         string
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
	// Ratio is the power of the defender to the attacker's in percent,
	// Reward the votes the win brought.
	Ratio  int
	Reward int64
}

// PetBattle is a fight the caller leads a round after another: the
// combatants, the acts so far, the health left, and the rating a win
// brings and a defeat costs.
type PetBattle struct {
	PetID          string
	DefenderID     string
	Attacker       PetFighter
	Defender       PetFighter
	Hits           []PetHit
	Round          int
	Rounds         int
	AttackerHealth int
	DefenderHealth int
	Ratio          int
	Win            int
	Loss           int
}

// PetBattleTurn is a fight after a move, the pet that leads it, and the
// fight as the list keeps it with the balance after it once it is over.
type PetBattleTurn struct {
	Battle  PetBattle
	Pet     Pet
	Fight   *PetFight
	Balance int64
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
