package user

import (
	"errors"
	"maps"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// PetKind is who a pet is.
type PetKind string

const (
	// PetCat is quick: it dodges more.
	PetCat PetKind = "cat"
	// PetDog is strong and sturdy.
	PetDog PetKind = "dog"
	// PetHamster is small and lucky: it hits twice as hard more often.
	PetHamster PetKind = "hamster"
	// PetDragon is strong at everything and dear.
	PetDragon PetKind = "dragon"
	// The kinds only cases give.
	PetRabbit  PetKind = "rabbit"
	PetTurtle  PetKind = "turtle"
	PetFox     PetKind = "fox"
	PetOwl     PetKind = "owl"
	PetWolf    PetKind = "wolf"
	PetPanda   PetKind = "panda"
	PetUnicorn PetKind = "unicorn"
	PetPhoenix PetKind = "phoenix"
)

// PetKinds are all kinds in the order the site shows them.
var PetKinds = []PetKind{PetCat, PetDog, PetHamster, PetDragon, PetRabbit, PetTurtle, PetFox, PetOwl, PetWolf, PetPanda, PetUnicorn, PetPhoenix}

// ShopPetKinds are the kinds the shop sells; the others fall out of cases.
var ShopPetKinds = []PetKind{PetCat, PetDog, PetHamster, PetDragon}

// PetRarity is how rarely a kind falls out of a case.
type PetRarity string

const (
	PetCommon    PetRarity = "common"
	PetRare      PetRarity = "rare"
	PetEpic      PetRarity = "epic"
	PetLegendary PetRarity = "legendary"
)

// PetRarities are the rarities from the most common.
var PetRarities = []PetRarity{PetCommon, PetRare, PetEpic, PetLegendary}

func (r PetRarity) String() string {
	return string(r)
}

// Rarity is how rare the kind is.
func (k PetKind) Rarity() PetRarity {
	return breeds[k].rarity
}

// PetKindsOf are the kinds of rarity r in the order of PetKinds.
func PetKindsOf(r PetRarity) []PetKind {
	var kinds []PetKind
	for _, k := range PetKinds {
		if k.Rarity() == r {
			kinds = append(kinds, k)
		}
	}

	return kinds
}

// The rules of a pet: what a meal and a game give, how fast hunger and
// boredom come and what a fight takes.
const (
	MaxPetFeeling = 100
	// PetSatietyLoss and PetJoyLoss are how long a point of satiety and of
	// joy lasts.
	PetSatietyLoss = 15 * time.Minute
	PetJoyLoss     = 20 * time.Minute
	PetMeal        = 40
	PetGame        = 30
	// PetGameHunger is the satiety a game takes.
	PetGameHunger = 5
	PetPlayEvery  = 30 * time.Minute
	// PetFightNeeds is the satiety and joy a fight needs, PetFightTakes —
	// what it takes.
	PetFightNeeds = 15
	PetFightTakes = 10
	PetCareGain   = 5
	PetWinGain    = 20
	PetLossGain   = 5
	PetDefendGain = 5
	MaxPetLevel   = 30
	// PetLevelStep is the experience of the levels: level n needs
	// PetLevelStep·n·(n−1).
	PetLevelStep   = 50
	PetStartRating = 1000
	// PetEloFactor is how far a fight moves the ratings.
	PetEloFactor = 32
	// PetDiceRange is the range of the random numbers of fights and
	// cases.
	PetDiceRange = 1 << 30
	// MaxPetNameLength is in characters.
	MaxPetNameLength = 20
	// MaxPetFights are the fights a list of them holds.
	MaxPetFights = 20
	// MaxPetFightsADay is the most fights a day the shop may allow.
	MaxPetFightsADay = 1_000
	// MaxPets are the pets a person may keep.
	MaxPets = 10
)

var (
	ErrUnknownPetKind = errors.New("unknown pet")
	ErrPetNameEmpty   = errors.New("the name of a pet is empty")
	ErrPetNameTooLong = errors.New("the name of a pet is too long")
	ErrPetNameLines   = errors.New("the name of a pet is one line")
	ErrPetNotFound    = errors.New("pet not found")
	ErrTooManyPets    = errors.New("you have too many pets")
	ErrPetFull        = errors.New("the pet is not hungry")
	ErrPetTired       = errors.New("the pet has played recently")
	ErrPetHungry      = errors.New("the pet is too hungry to fight")
	ErrPetSad         = errors.New("the pet is too sad to fight")
	ErrPetFightsOver  = errors.New("the pet has fought enough today")
	ErrPetFightSelf   = errors.New("a pet does not fight itself")
	ErrNoPetOpponent  = errors.New("there is nobody to fight")
	ErrPetShopPrice   = errors.New("the prices of the pet shop must be from 0")
	ErrPetShopFights  = errors.New("the fights of a day must be from 1 to 1000")
	ErrNotInPetShop   = errors.New("the shop does not sell this pet")
	ErrUnknownPetCase = errors.New("unknown case")
	ErrUnknownPetItem = errors.New("unknown pet clothes")
	ErrPetItemOwned   = errors.New("the pet has these clothes already")
	ErrPetItemMissing = errors.New("the pet has no such clothes")
)

func NewPetKind(s string) (PetKind, error) {
	for _, k := range PetKinds {
		if string(k) == s {
			return k, nil
		}
	}

	return "", ErrUnknownPetKind
}

func (k PetKind) String() string {
	return string(k)
}

// PetName is what an owner calls their pet: one line.
type PetName string

func NewPetName(s string) (PetName, error) {
	s, err := visibleText(s)
	if err != nil {
		return "", err
	}

	switch {
	case s == "":
		return "", ErrPetNameEmpty
	case strings.ContainsAny(s, "\r\n"):
		return "", ErrPetNameLines
	case utf8.RuneCountInString(s) > MaxPetNameLength:
		return "", ErrPetNameTooLong
	}

	return PetName(s), nil
}

func (n PetName) String() string {
	return string(n)
}

// PetStats are how a pet fights: Speed is the chance in a hundred to
// dodge a hit and, the faster pet, to hit first.
type PetStats struct {
	Attack  int
	Defense int
	Health  int
	Speed   int
}

// Add is s with the stats of o added.
func (s PetStats) Add(o PetStats) PetStats {
	return PetStats{Attack: s.Attack + o.Attack, Defense: s.Defense + o.Defense, Health: s.Health + o.Health, Speed: s.Speed + o.Speed}
}

// Power is the stats in one number, what matching opponents compares.
func (s PetStats) Power() int {
	return s.Attack*4 + s.Defense*3 + s.Health + s.Speed*2
}

// scaled is s with attack, defense and health at percent of them, at
// least one each.
func (s PetStats) scaled(percent int) PetStats {
	return PetStats{
		Attack:  max(s.Attack*percent/100, 1),
		Defense: max(s.Defense*percent/100, 1),
		Health:  max(s.Health*percent/100, 1),
		Speed:   s.Speed,
	}
}

// breed is a kind: its stats at level 1, its chance in a hundred to hit
// twice as hard, its rarity, its element and its special.
type breed struct {
	stats   PetStats
	crit    int
	rarity  PetRarity
	element PetElement
	special PetSpecial
}

// breeds are the kinds: the rarer, the stronger at level 1.
var breeds = map[PetKind]breed{
	PetCat:     {PetStats{Attack: 6, Defense: 4, Health: 40, Speed: 14}, 5, PetCommon, PetFire, PetCounter},
	PetDog:     {PetStats{Attack: 7, Defense: 6, Health: 46, Speed: 6}, 5, PetCommon, PetWater, PetRage},
	PetHamster: {PetStats{Attack: 5, Defense: 4, Health: 36, Speed: 10}, 20, PetCommon, PetForest, PetCheeks},
	PetRabbit:  {PetStats{Attack: 5, Defense: 3, Health: 36, Speed: 18}, 5, PetCommon, PetForest, PetLeap},
	PetTurtle:  {PetStats{Attack: 4, Defense: 10, Health: 50, Speed: 2}, 5, PetCommon, PetWater, PetShell},
	PetFox:     {PetStats{Attack: 7, Defense: 5, Health: 42, Speed: 16}, 8, PetRare, PetFire, PetTrick},
	PetOwl:     {PetStats{Attack: 7, Defense: 5, Health: 42, Speed: 12}, 18, PetRare, PetWater, PetSight},
	PetDragon:  {PetStats{Attack: 8, Defense: 7, Health: 52, Speed: 8}, 5, PetEpic, PetFire, PetFlame},
	PetWolf:    {PetStats{Attack: 10, Defense: 6, Health: 50, Speed: 12}, 8, PetEpic, PetForest, PetHowl},
	PetPanda:   {PetStats{Attack: 8, Defense: 10, Health: 62, Speed: 5}, 6, PetEpic, PetForest, PetHide},
	PetUnicorn: {PetStats{Attack: 11, Defense: 9, Health: 60, Speed: 14}, 10, PetLegendary, PetWater, PetRainbow},
	PetPhoenix: {PetStats{Attack: 12, Defense: 8, Health: 58, Speed: 14}, 14, PetLegendary, PetFire, PetRebirth},
}

// growth is the percent of the stats of a level a pet of each rarity
// gains: the rarer pets start stronger and grow slower, so that levels
// catch up with a rare kind.
var growth = map[PetRarity]int{PetCommon: 100, PetRare: 90, PetEpic: 80, PetLegendary: 70}

// Element is what the kind is of.
func (k PetKind) Element() PetElement {
	return breeds[k].element
}

// Special is the trick of the kind in a fight.
func (k PetKind) Special() PetSpecial {
	return breeds[k].special
}

// critOf is the chance in a hundred of a kind to hit twice as hard.
func critOf(k PetKind) int {
	return breeds[k].crit
}

// PetCase is a case of pets: opened, it gives a pet of a kind drawn by
// the odds of its rarity.
type PetCase string

const (
	PetCaseBronze PetCase = "bronze"
	PetCaseSilver PetCase = "silver"
	PetCaseGold   PetCase = "gold"
)

// PetCases are all cases from the cheapest.
var PetCases = []PetCase{PetCaseBronze, PetCaseSilver, PetCaseGold}

// PetCaseOdds is what the odds of a case add up to: they are in a
// thousand.
const PetCaseOdds = 1000

// caseOdds are the odds of each rarity of PetRarities in a case.
var caseOdds = map[PetCase][]int{
	PetCaseBronze: {720, 240, 38, 2},
	PetCaseSilver: {430, 420, 130, 20},
	PetCaseGold:   {0, 600, 370, 30},
}

func NewPetCase(s string) (PetCase, error) {
	if _, ok := caseOdds[PetCase(s)]; !ok {
		return "", ErrUnknownPetCase
	}

	return PetCase(s), nil
}

func (c PetCase) String() string {
	return string(c)
}

// Odds are the chances in PetCaseOdds of each rarity of PetRarities.
func (c PetCase) Odds() []int {
	return slices.Clone(caseOdds[c])
}

// Draw is the kind the case gives for rarity, a number in [0,
// PetCaseOdds) that picks the rarity by the odds, and kind, any number
// from 0 that picks one of its kinds.
func (c PetCase) Draw(rarity, kind int64) PetKind {
	odds := caseOdds[c]
	picked := PetRarities[len(PetRarities)-1]
	for i, chance := range odds {
		if rarity < int64(chance) {
			picked = PetRarities[i]
			break
		}
		rarity -= int64(chance)
	}

	kinds := PetKindsOf(picked)

	return kinds[kind%int64(len(kinds))]
}

// PetSlot is where on a pet clothes go: one thing a slot.
type PetSlot string

const (
	PetHead PetSlot = "head"
	PetEyes PetSlot = "eyes"
	PetNeck PetSlot = "neck"
	PetBody PetSlot = "body"
)

// PetSlots are the slots from the head down.
var PetSlots = []PetSlot{PetHead, PetEyes, PetNeck, PetBody}

// PetItem is a thing of clothes a pet wears.
type PetItem string

// PetItemSpec is a thing of clothes: where it goes, what it costs and
// what it adds to the stats of the pet that wears it.
type PetItemSpec struct {
	Item  PetItem
	Slot  PetSlot
	Price int64
	Bonus PetStats
}

// PetItems are the clothes the shop sells, a slot after another, from
// the cheapest; the prices are fixed, as the catalogue is.
var PetItems = []PetItemSpec{
	{Item: "cap", Slot: PetHead, Price: 60, Bonus: PetStats{Speed: 2}},
	{Item: "helmet", Slot: PetHead, Price: 300, Bonus: PetStats{Defense: 3}},
	{Item: "crown", Slot: PetHead, Price: 1500, Bonus: PetStats{Attack: 3, Defense: 3}},
	{Item: "glasses", Slot: PetEyes, Price: 80, Bonus: PetStats{Attack: 1, Defense: 1}},
	{Item: "shades", Slot: PetEyes, Price: 200, Bonus: PetStats{Speed: 3}},
	{Item: "monocle", Slot: PetEyes, Price: 700, Bonus: PetStats{Attack: 3, Speed: 1}},
	{Item: "bow", Slot: PetNeck, Price: 40, Bonus: PetStats{Health: 6}},
	{Item: "scarf", Slot: PetNeck, Price: 150, Bonus: PetStats{Health: 12}},
	{Item: "medal", Slot: PetNeck, Price: 600, Bonus: PetStats{Attack: 2, Health: 8}},
	{Item: "sweater", Slot: PetBody, Price: 180, Bonus: PetStats{Health: 15}},
	{Item: "armor", Slot: PetBody, Price: 900, Bonus: PetStats{Defense: 4, Health: 10}},
	{Item: "cape", Slot: PetBody, Price: 1200, Bonus: PetStats{Attack: 4, Speed: 2}},
}

func NewPetItem(s string) (PetItem, error) {
	for _, spec := range PetItems {
		if string(spec.Item) == s {
			return spec.Item, nil
		}
	}

	return "", ErrUnknownPetItem
}

func (i PetItem) String() string {
	return string(i)
}

// Spec is the thing in the catalogue.
func (i PetItem) Spec() PetItemSpec {
	for _, spec := range PetItems {
		if spec.Item == i {
			return spec
		}
	}

	return PetItemSpec{}
}

// PetLevel is the level of experience, from 1 to MaxPetLevel.
func PetLevel(experience int64) int {
	level := 1
	for level < MaxPetLevel && experience >= PetLevelStep*int64(level+1)*int64(level) {
		level++
	}

	return level
}

// PetLevelExperience is the experience level begins with.
func PetLevelExperience(level int) int64 {
	return PetLevelStep * int64(level) * int64(level-1)
}

// PetShop is what pets cost: to adopt each kind and a meal; what a won
// fight brings and how many fights a pet has a day.
type PetShop struct {
	prices map[PetKind]int64
	cases  map[PetCase]int64
	meal   int64
	win    int64
	fights int
}

// defaultCases are what the cases cost until the administrators say.
var defaultCases = map[PetCase]int64{PetCaseBronze: 150, PetCaseSilver: 400, PetCaseGold: 1200}

// NewPetShop checks the shop: prices holds every kind of ShopPetKinds;
// the cases cost as by default until WithCases.
func NewPetShop(prices map[PetKind]int64, meal, win int64, fights int) (PetShop, error) {
	kept := make(map[PetKind]int64, len(ShopPetKinds))
	for _, k := range ShopPetKinds {
		price, ok := prices[k]
		if !ok || price < 0 || price > MaxAmount {
			return PetShop{}, ErrPetShopPrice
		}
		kept[k] = price
	}

	if meal < 0 || meal > MaxAmount || win < 0 || win > MaxAmount {
		return PetShop{}, ErrPetShopPrice
	}
	if fights < 1 || fights > MaxPetFightsADay {
		return PetShop{}, ErrPetShopFights
	}

	return PetShop{prices: kept, cases: maps.Clone(defaultCases), meal: meal, win: win, fights: fights}, nil
}

// WithCases is the shop with the cases at prices, which holds every case.
func (s PetShop) WithCases(prices map[PetCase]int64) (PetShop, error) {
	kept := make(map[PetCase]int64, len(PetCases))
	for _, c := range PetCases {
		price, ok := prices[c]
		if !ok || price < 0 || price > MaxAmount {
			return PetShop{}, ErrPetShopPrice
		}
		kept[c] = price
	}
	s.cases = kept

	return s, nil
}

// DefaultPetShop is the shop until the administrators set one.
func DefaultPetShop() PetShop {
	return PetShop{
		prices: map[PetKind]int64{PetCat: 100, PetDog: 100, PetHamster: 50, PetDragon: 500},
		cases:  maps.Clone(defaultCases),
		meal:   5,
		win:    3,
		fights: 10,
	}
}

// Price is what adopting k costs; ErrNotInPetShop for a kind only cases
// give.
func (s PetShop) Price(k PetKind) (int64, error) {
	price, ok := s.prices[k]
	if !ok {
		return 0, ErrNotInPetShop
	}

	return price, nil
}

// CasePrice is what opening c costs.
func (s PetShop) CasePrice(c PetCase) int64 {
	return s.cases[c]
}

// Cases are the prices of all cases.
func (s PetShop) Cases() map[PetCase]int64 {
	return maps.Clone(s.cases)
}

// Prices are the prices of all kinds.
func (s PetShop) Prices() map[PetKind]int64 {
	prices := make(map[PetKind]int64, len(s.prices))
	for k, p := range s.prices {
		prices[k] = p
	}

	return prices
}

func (s PetShop) Meal() int64 {
	return s.meal
}

func (s PetShop) Win() int64 {
	return s.win
}

func (s PetShop) Fights() int {
	return s.fights
}

// PetState is all a pet keeps: the feelings are what they were at
// CountedAt; FoughtOn is the day of Fights.
type PetState struct {
	ID         uuid.UUID
	Kind       PetKind
	Name       PetName
	BornAt     time.Time
	Satiety    int
	Joy        int
	CountedAt  time.Time
	PlayedAt   time.Time
	Experience int64
	Rating     int
	Wins       int
	Losses     int
	FoughtOn   time.Time
	Fights     int
	// Wardrobe are the clothes bought for the pet, Outfit those it wears:
	// one a slot.
	Wardrobe []PetItem
	Outfit   []PetItem
	// Stance is how it defends: how it fights those who set their pets
	// on it.
	Stance PetStance
	// Streak are the fights in a row, the last at StreakAt, each within
	// PetRest of the one before: they tire the pet.
	Streak   int
	StreakAt time.Time
}

// Pet is a pet of a person, a tamagotchi: it grows hungry and bored with
// time, is fed for votes, played with and fights other pets. A person
// keeps up to MaxPets of them.
type Pet struct {
	s PetState
}

// NewPet is a pet adopted at now, fed and glad.
func NewPet(kind PetKind, name PetName, now time.Time) *Pet {
	return &Pet{s: PetState{
		ID: uuid.Must(uuid.NewV7()), Kind: kind, Name: name, BornAt: now, Satiety: 80, Joy: 80, CountedAt: now, Rating: PetStartRating,
		Stance: PetDefense,
	}}
}

func RestorePet(s PetState) *Pet {
	return &Pet{s: s}
}

// State is what storage keeps.
func (p *Pet) State() PetState {
	s := p.s
	s.Wardrobe, s.Outfit = slices.Clone(s.Wardrobe), slices.Clone(s.Outfit)

	return s
}

// Wardrobe are the clothes bought for the pet, Outfit those it wears.
func (p *Pet) Wardrobe() []PetItem { return slices.Clone(p.s.Wardrobe) }
func (p *Pet) Outfit() []PetItem   { return slices.Clone(p.s.Outfit) }

func (p *Pet) ID() uuid.UUID       { return p.s.ID }
func (p *Pet) Kind() PetKind       { return p.s.Kind }
func (p *Pet) Name() PetName       { return p.s.Name }
func (p *Pet) BornAt() time.Time   { return p.s.BornAt }
func (p *Pet) Experience() int64   { return p.s.Experience }
func (p *Pet) Rating() int         { return p.s.Rating }
func (p *Pet) Wins() int           { return p.s.Wins }
func (p *Pet) Losses() int         { return p.s.Losses }
func (p *Pet) PlayedAt() time.Time { return p.s.PlayedAt }
func (p *Pet) Level() int          { return PetLevel(p.s.Experience) }
func (p *Pet) Stance() PetStance   { return p.s.Stance }

// felt is a feeling of at, which was value at CountedAt and loses a point
// each loss.
func (p *Pet) felt(value int, loss time.Duration, at time.Time) int {
	lost := int(max(at.Sub(p.s.CountedAt), 0) / loss)

	return max(value-lost, 0)
}

// SatietyAt and JoyAt are the feelings of the pet at now.
func (p *Pet) SatietyAt(now time.Time) int {
	return p.felt(p.s.Satiety, PetSatietyLoss, now)
}

func (p *Pet) JoyAt(now time.Time) int {
	return p.felt(p.s.Joy, PetJoyLoss, now)
}

// FightsAt are the fights of the day of now.
func (p *Pet) FightsAt(now time.Time) int {
	if !p.s.FoughtOn.Equal(BonusDay(now)) {
		return 0
	}

	return p.s.Fights
}

// Crit is the percent of the hits of the pet that hit twice as hard.
func (p *Pet) Crit() int {
	return critOf(p.s.Kind)
}

// PlayAt is when the pet may play again: its last game and PetPlayEvery,
// zero when it never played.
func (p *Pet) PlayAt() time.Time {
	if p.s.PlayedAt.IsZero() {
		return time.Time{}
	}

	return p.s.PlayedAt.Add(PetPlayEvery)
}

// Stats are how the pet fights at its level in its clothes: BreedStats,
// LevelStats and ClothesStats added up.
func (p *Pet) Stats() PetStats {
	return p.BreedStats().Add(p.LevelStats()).Add(p.ClothesStats())
}

// BreedStats are the stats of its kind at level 1.
func (p *Pet) BreedStats() PetStats {
	return breeds[p.s.Kind].stats
}

// LevelStats are what its levels add: less to the rarer kinds.
func (p *Pet) LevelStats() PetStats {
	grown := p.Level() - 1
	pct := growth[p.s.Kind.Rarity()]

	return PetStats{Attack: 2 * grown * pct / 100, Defense: 2 * grown * pct / 100, Health: 6 * grown * pct / 100, Speed: grown / 5}
}

// Power is how strong the pet is in one number.
func (p *Pet) Power() int {
	return p.Stats().Power()
}

// FatigueAt are the fights in a row the pet had until now: each takes
// PetFatigueStep percent of its attack, defense and health in the next.
func (p *Pet) FatigueAt(now time.Time) int {
	if p.s.StreakAt.IsZero() || now.Sub(p.s.StreakAt) >= PetRest {
		return 0
	}

	return min(p.s.Streak, MaxPetFatigue)
}

// FightStatsAt are the stats the pet fights with at now, tired by its
// fights in a row.
func (p *Pet) FightStatsAt(now time.Time) PetStats {
	return p.Stats().scaled(100 - PetFatigueStep*p.FatigueAt(now))
}

// Stood is the pet that defends in stance.
func (p *Pet) Stood(stance PetStance) *Pet {
	s := p.State()
	s.Stance = stance

	return &Pet{s: s}
}

// ClothesStats are what the clothes it wears add.
func (p *Pet) ClothesStats() PetStats {
	var sum PetStats
	for _, item := range p.s.Outfit {
		sum = sum.Add(item.Spec().Bonus)
	}

	return sum
}

// counted is the pet with its feelings counted at now.
func (p *Pet) counted(now time.Time) PetState {
	s := p.State()
	s.Satiety, s.Joy, s.CountedAt = p.SatietyAt(now), p.JoyAt(now), now

	return s
}

// Fed is the pet after a meal at now; ErrPetFull when it is not hungry.
func (p *Pet) Fed(now time.Time) (*Pet, error) {
	s := p.counted(now)
	if s.Satiety >= MaxPetFeeling {
		return nil, ErrPetFull
	}

	s.Satiety = min(s.Satiety+PetMeal, MaxPetFeeling)
	s.Experience += PetCareGain

	return &Pet{s: s}, nil
}

// Played is the pet after a game at now; ErrPetTired within PetPlayEvery
// of the last one.
func (p *Pet) Played(now time.Time) (*Pet, error) {
	if !p.s.PlayedAt.IsZero() && now.Sub(p.s.PlayedAt) < PetPlayEvery {
		return nil, ErrPetTired
	}

	s := p.counted(now)
	s.Joy = min(s.Joy+PetGame, MaxPetFeeling)
	s.Satiety = max(s.Satiety-PetGameHunger, 0)
	s.Experience += PetCareGain
	s.PlayedAt = now

	return &Pet{s: s}, nil
}

// Renamed is the pet called name.
func (p *Pet) Renamed(name PetName) *Pet {
	s := p.State()
	s.Name = name

	return &Pet{s: s}
}

// Bought is the pet with item bought and put on, instead of what it wore
// in its slot; ErrPetItemOwned when it has it.
func (p *Pet) Bought(item PetItem) (*Pet, error) {
	if slices.Contains(p.s.Wardrobe, item) {
		return nil, ErrPetItemOwned
	}

	s := p.State()
	s.Wardrobe = append(s.Wardrobe, item)

	return (&Pet{s: s}).Dressed(item, true)
}

// Dressed is the pet with item put on, instead of what it wore in its
// slot, or taken off; ErrPetItemMissing when it has none.
func (p *Pet) Dressed(item PetItem, wear bool) (*Pet, error) {
	if !slices.Contains(p.s.Wardrobe, item) {
		return nil, ErrPetItemMissing
	}

	s := p.State()
	slot := item.Spec().Slot
	s.Outfit = slices.DeleteFunc(s.Outfit, func(worn PetItem) bool { return worn.Spec().Slot == slot })
	if wear {
		s.Outfit = append(s.Outfit, item)
	}

	return &Pet{s: s}, nil
}

// CheckFightAt tells whether the pet may fight at now under shop.
func (p *Pet) CheckFightAt(shop PetShop, now time.Time) error {
	switch {
	case p.SatietyAt(now) < PetFightNeeds:
		return ErrPetHungry
	case p.JoyAt(now) < PetFightNeeds:
		return ErrPetSad
	case p.FightsAt(now) >= shop.Fights():
		return ErrPetFightsOver
	}

	return nil
}

// PetFight is a fight as the list of fights keeps it: the owners of the
// pets that fought.
type PetFight struct {
	ID         uuid.UUID
	AttackerID uuid.UUID
	DefenderID uuid.UUID
	Bout       PetBout
	At         time.Time
}

// PetShopStats are how the pets do: how many there are, the fights they
// had and the votes spent on them and won by them.
type PetShopStats struct {
	Pets   int64
	Fights int64
	Spent  int64
	Won    int64
}
