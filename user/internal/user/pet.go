package user

import (
	"errors"
	"math"
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
)

// PetKinds are all kinds in the order the site shows them.
var PetKinds = []PetKind{PetCat, PetDog, PetHamster, PetDragon}

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
	// MaxPetTurns is the most hits a fight lasts.
	MaxPetTurns = 12
	// PetDiceCount is the random numbers a fight may take.
	PetDiceCount = MaxPetTurns * 3
	// PetDiceRange is the range of each of them.
	PetDiceRange = 1 << 30
	// MaxPetNameLength is in characters.
	MaxPetNameLength = 20
	// MaxPetFights are the fights a list of them holds.
	MaxPetFights = 20
	// MaxPetFightsADay is the most fights a day the shop may allow.
	MaxPetFightsADay = 1_000
)

var (
	ErrUnknownPetKind = errors.New("unknown pet")
	ErrPetNameEmpty   = errors.New("the name of a pet is empty")
	ErrPetNameTooLong = errors.New("the name of a pet is too long")
	ErrPetNameLines   = errors.New("the name of a pet is one line")
	ErrPetNotFound    = errors.New("pet not found")
	ErrPetExists      = errors.New("you have a pet already")
	ErrPetFull        = errors.New("the pet is not hungry")
	ErrPetTired       = errors.New("the pet has played recently")
	ErrPetHungry      = errors.New("the pet is too hungry to fight")
	ErrPetSad         = errors.New("the pet is too sad to fight")
	ErrPetFightsOver  = errors.New("the pet has fought enough today")
	ErrPetFightSelf   = errors.New("a pet does not fight itself")
	ErrNoPetOpponent  = errors.New("there is nobody to fight")
	ErrPetShopPrice   = errors.New("the prices of the pet shop must be from 0")
	ErrPetShopFights  = errors.New("the fights of a day must be from 1 to 1000")
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

// breeds are the stats of each kind at level 1.
var breeds = map[PetKind]PetStats{
	PetCat:     {Attack: 6, Defense: 4, Health: 40, Speed: 14},
	PetDog:     {Attack: 7, Defense: 6, Health: 46, Speed: 6},
	PetHamster: {Attack: 5, Defense: 4, Health: 36, Speed: 10},
	PetDragon:  {Attack: 8, Defense: 7, Health: 52, Speed: 8},
}

// critOf is the chance in a hundred of a kind to hit twice as hard.
func critOf(k PetKind) int {
	if k == PetHamster {
		return 20
	}

	return 5
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
	meal   int64
	win    int64
	fights int
}

// NewPetShop checks the shop: prices holds every kind.
func NewPetShop(prices map[PetKind]int64, meal, win int64, fights int) (PetShop, error) {
	kept := make(map[PetKind]int64, len(PetKinds))
	for _, k := range PetKinds {
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

	return PetShop{prices: kept, meal: meal, win: win, fights: fights}, nil
}

// DefaultPetShop is the shop until the administrators set one.
func DefaultPetShop() PetShop {
	return PetShop{
		prices: map[PetKind]int64{PetCat: 100, PetDog: 100, PetHamster: 50, PetDragon: 500},
		meal:   5,
		win:    3,
		fights: 10,
	}
}

// Price is what adopting k costs.
func (s PetShop) Price(k PetKind) int64 {
	return s.prices[k]
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
}

// Pet is the pet of a person, a tamagotchi: it grows hungry and bored
// with time, is fed for votes, played with and fights other pets.
type Pet struct {
	s PetState
}

// NewPet is a pet adopted at now, fed and glad.
func NewPet(kind PetKind, name PetName, now time.Time) *Pet {
	return &Pet{s: PetState{
		Kind: kind, Name: name, BornAt: now, Satiety: 80, Joy: 80, CountedAt: now, Rating: PetStartRating,
	}}
}

func RestorePet(s PetState) *Pet {
	return &Pet{s: s}
}

// State is what storage keeps.
func (p *Pet) State() PetState {
	return p.s
}

func (p *Pet) Kind() PetKind       { return p.s.Kind }
func (p *Pet) Name() PetName       { return p.s.Name }
func (p *Pet) BornAt() time.Time   { return p.s.BornAt }
func (p *Pet) Experience() int64   { return p.s.Experience }
func (p *Pet) Rating() int         { return p.s.Rating }
func (p *Pet) Wins() int           { return p.s.Wins }
func (p *Pet) Losses() int         { return p.s.Losses }
func (p *Pet) PlayedAt() time.Time { return p.s.PlayedAt }
func (p *Pet) Level() int          { return PetLevel(p.s.Experience) }

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

// Stats are how the pet fights at its level.
func (p *Pet) Stats() PetStats {
	s := breeds[p.s.Kind]
	grown := p.Level() - 1

	return PetStats{
		Attack:  s.Attack + grown,
		Defense: s.Defense + grown,
		Health:  s.Health + 4*grown,
		Speed:   s.Speed + grown/5,
	}
}

// counted is the pet with its feelings counted at now.
func (p *Pet) counted(now time.Time) PetState {
	s := p.s
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
	s := p.s
	s.Name = name

	return &Pet{s: s}
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

// PetDice are the random numbers a fight takes, each in [0, PetDiceRange);
// a fight that runs out of them reads zeros.
type PetDice []int64

// next is the next number of the dice in [0, n).
func (d *PetDice) next(n int) int {
	if len(*d) == 0 || n <= 0 {
		return 0
	}

	v := (*d)[0]
	*d = (*d)[1:]

	return int(v % int64(n))
}

// PetHit is a hit of a fight: who hit, whether the other dodged or it was
// twice as hard, the damage and the health both are left with after it.
type PetHit struct {
	ByAttacker     bool
	Dodged         bool
	Crit           bool
	Damage         int
	AttackerHealth int
	DefenderHealth int
}

// PetFighter is a pet as a fight remembers it.
type PetFighter struct {
	Kind   PetKind
	Name   PetName
	Level  int
	Health int
}

// PetBout is a fight: who fought, how it went and what it changed.
type PetBout struct {
	Attacker PetFighter
	Defender PetFighter
	Hits     []PetHit
	Won      bool
	// Rating is the points the attacker won (above zero) or lost; the
	// defender changes by as much the other way.
	Rating int
}

// strike is a hit of a with stats as against d with stats.
func strike(as, ds PetStats, kind PetKind, dice *PetDice) (damage int, dodged, crit bool) {
	// High numbers dodge and crit, so dice of zeros never do.
	if dice.next(100) >= 100-ds.Speed {
		return 0, true, false
	}

	damage = max(as.Attack*2-ds.Defense+dice.next(as.Attack+1), 1)
	if dice.next(100) >= 100-critOf(kind) {
		return damage * 2, false, true
	}

	return damage, false, false
}

// Fought is the fight of the pet against defender at now: the pet and
// the defender after it and how it went. The faster pet hits first, then
// they take turns until one falls or MaxPetTurns pass, when the one with
// the greater share of its health left wins — the defender on a tie.
func (p *Pet) Fought(defender *Pet, shop PetShop, now time.Time, dice PetDice) (*Pet, *Pet, PetBout, error) {
	if err := p.CheckFightAt(shop, now); err != nil {
		return nil, nil, PetBout{}, err
	}

	as, ds := p.Stats(), defender.Stats()
	bout := PetBout{
		Attacker: PetFighter{Kind: p.s.Kind, Name: p.s.Name, Level: p.Level(), Health: as.Health},
		Defender: PetFighter{Kind: defender.s.Kind, Name: defender.s.Name, Level: defender.Level(), Health: ds.Health},
	}

	ah, dh := as.Health, ds.Health
	byAttacker := as.Speed >= ds.Speed
	for turn := 0; turn < MaxPetTurns && ah > 0 && dh > 0; turn++ {
		hit := PetHit{ByAttacker: byAttacker}
		if byAttacker {
			hit.Damage, hit.Dodged, hit.Crit = strike(as, ds, p.s.Kind, &dice)
			dh = max(dh-hit.Damage, 0)
		} else {
			hit.Damage, hit.Dodged, hit.Crit = strike(ds, as, defender.s.Kind, &dice)
			ah = max(ah-hit.Damage, 0)
		}
		hit.AttackerHealth, hit.DefenderHealth = ah, dh
		bout.Hits = append(bout.Hits, hit)
		byAttacker = !byAttacker
	}

	switch {
	case dh == 0:
		bout.Won = true
	case ah == 0:
		bout.Won = false
	default:
		bout.Won = ah*ds.Health > dh*as.Health
	}

	bout.Rating = eloChange(p.s.Rating, defender.s.Rating, bout.Won)

	a := p.counted(now)
	a.Satiety = max(a.Satiety-PetFightTakes, 0)
	a.Joy = max(a.Joy-PetFightTakes, 0)
	a.Fights = p.FightsAt(now) + 1
	a.FoughtOn = BonusDay(now)
	a.Rating += bout.Rating

	d := defender.s
	d.Rating -= bout.Rating
	d.Experience += PetDefendGain

	if bout.Won {
		a.Wins++
		a.Experience += PetWinGain
		d.Losses++
	} else {
		a.Losses++
		a.Experience += PetLossGain
		d.Wins++
	}

	return &Pet{s: a}, &Pet{s: d}, bout, nil
}

// eloChange is the points a pet of rating a wins or loses against one of
// rating b: at least one either way.
func eloChange(a, b int, won bool) int {
	expected := 1 / (1 + math.Pow(10, float64(b-a)/400))

	score := 0.0
	if won {
		score = 1
	}

	change := int(math.Round(PetEloFactor * (score - expected)))
	switch {
	case won && change < 1:
		return 1
	case !won && change > -1:
		return -1
	}

	return change
}

// PetFight is a fight as the list of fights keeps it.
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
