package user

import (
	"errors"
	"time"
)

// Ore is what the mine gives, from the commonest to the rarest.
type Ore string

const (
	OreStone   Ore = "stone"
	OreCoal    Ore = "coal"
	OreIron    Ore = "iron"
	OreGold    Ore = "gold"
	OreDiamond Ore = "diamond"
	OreCrystal Ore = "crystal"
)

// Ores are all ores in order: each opens with more experience.
var Ores = []Ore{OreStone, OreCoal, OreIron, OreGold, OreDiamond, OreCrystal}

// MineOdds is the range of the roll of a mine event: a chance of 100 is
// one dig in a hundred.
const MineOdds = 10_000

// The bounds of the mine settings.
const (
	MaxOreExperience = 1_000_000_000
	MaxOreYield      = 1_000_000
	MaxOrePrice      = 1_000_000
	MaxOreGain       = 1_000_000
	MaxOreEnergy     = 100
	MaxMineEnergy    = 1_000
	MinMineRegen     = time.Second
	MaxMineRegen     = 24 * time.Hour
	MaxNuggetTimes   = 100
	MaxChestVotes    = MaxWin
	MaxDrinkEnergy   = MaxMineEnergy
)

// MineEvent is what may happen at a dig besides the ore.
type MineEvent string

const (
	MineEventNone MineEvent = ""
	// MineEventNugget multiplies the ore of the dig.
	MineEventNugget MineEvent = "nugget"
	// MineEventChest finds votes besides the ore.
	MineEventChest MineEvent = "chest"
	// MineEventCollapse buries the ore: the dig gives nothing.
	MineEventCollapse MineEvent = "collapse"
	// MineEventDrink gives energy back besides the ore.
	MineEventDrink MineEvent = "drink"
)

var (
	ErrUnknownOre      = errors.New("unknown ore")
	ErrOreLocked       = errors.New("this ore needs more experience")
	ErrNoEnergy        = errors.New("not enough energy")
	ErrNothingToSell   = errors.New("not enough ore to sell")
	ErrMineNotYet      = errors.New("ore is sold by pages a week old with a filled profile")
	ErrMineVeins       = errors.New("the mine must set every ore once, in order")
	ErrOreExperience   = errors.New("the experience of an ore must be from 0 to 1000000000, the stone from 0, each not below the one before")
	ErrOreYield        = errors.New("the yield of an ore must be from 1 to 1000000, the least not above the most")
	ErrOrePrice        = errors.New("the price of an ore must be from 0 to 1000000 hundredths of a vote")
	ErrOreGain         = errors.New("the experience of a dig must be from 0 to 1000000")
	ErrOreEnergy       = errors.New("a dig must take from 1 to 100 energy")
	ErrMineEnergy      = errors.New("the energy of a miner must be from 1 to 1000, not below a dig")
	ErrMineRegen       = errors.New("a point of energy must come back in 1 second to 24 hours")
	ErrMineChances     = errors.New("the chances of the events must be from 0 and together at most 10000")
	ErrMineNugget      = errors.New("a nugget must multiply the ore from 2 to 100 times")
	ErrMineChest       = errors.New("a chest must hold from 1 to 1000 votes, the least not above the most")
	ErrMineDrinkEnergy = errors.New("a drink must give from 1 to 1000 energy")
)

func NewOre(s string) (Ore, error) {
	for _, o := range Ores {
		if string(o) == s {
			return o, nil
		}
	}

	return "", ErrUnknownOre
}

func (o Ore) String() string {
	return string(o)
}

// OreVein is how an ore is mined: it opens at Experience; a dig takes
// Energy, gives from Least to Most of it and Gain experience; a unit sells
// for Price hundredths of a vote.
type OreVein struct {
	Ore        Ore
	Experience int64
	Least      int64
	Most       int64
	Price      int64
	Gain       int64
	Energy     int
}

// MineEvents are the chances of the events of a dig, each in MineOdds, and
// what they do: a nugget multiplies the ore NuggetTimes, a chest holds from
// ChestLeast to ChestMost votes, a drink gives DrinkEnergy back.
type MineEvents struct {
	Nugget      int
	NuggetTimes int
	Chest       int
	ChestLeast  int64
	ChestMost   int64
	Collapse    int
	Drink       int
	DrinkEnergy int
}

// Mine is the mine of the site, as the administrators set it: the veins of
// all ores, the energy a miner has at most, how fast a point of it comes
// back and the events of a dig.
type Mine struct {
	veins  []OreVein
	energy int
	regen  time.Duration
	events MineEvents
}

// NewMine checks the mine: veins are all Ores in order.
func NewMine(veins []OreVein, energy int, regen time.Duration, events MineEvents) (Mine, error) {
	if len(veins) != len(Ores) {
		return Mine{}, ErrMineVeins
	}

	for i, v := range veins {
		if err := checkVein(v, i, veins); err != nil {
			return Mine{}, err
		}
		if v.Energy > energy {
			return Mine{}, ErrMineEnergy
		}
	}

	switch {
	case energy < 1 || energy > MaxMineEnergy:
		return Mine{}, ErrMineEnergy
	case regen < MinMineRegen || regen > MaxMineRegen:
		return Mine{}, ErrMineRegen
	}

	if err := events.check(); err != nil {
		return Mine{}, err
	}

	return Mine{veins: append([]OreVein(nil), veins...), energy: energy, regen: regen, events: events}, nil
}

func checkVein(v OreVein, i int, veins []OreVein) error {
	switch {
	case v.Ore != Ores[i]:
		return ErrMineVeins
	case v.Experience < 0 || v.Experience > MaxOreExperience || (i == 0 && v.Experience != 0) ||
		(i > 0 && v.Experience < veins[i-1].Experience):
		return ErrOreExperience
	case v.Least < 1 || v.Most > MaxOreYield || v.Least > v.Most:
		return ErrOreYield
	case v.Price < 0 || v.Price > MaxOrePrice:
		return ErrOrePrice
	case v.Gain < 0 || v.Gain > MaxOreGain:
		return ErrOreGain
	case v.Energy < 1 || v.Energy > MaxOreEnergy:
		return ErrOreEnergy
	}

	return nil
}

func (e MineEvents) check() error {
	chances := []int{e.Nugget, e.Chest, e.Collapse, e.Drink}

	total := 0
	for _, c := range chances {
		if c < 0 || c > MineOdds {
			return ErrMineChances
		}

		total += c
	}

	switch {
	case total > MineOdds:
		return ErrMineChances
	case e.NuggetTimes < 2 || e.NuggetTimes > MaxNuggetTimes:
		return ErrMineNugget
	case e.ChestLeast < 1 || e.ChestMost > MaxChestVotes || e.ChestLeast > e.ChestMost:
		return ErrMineChest
	case e.DrinkEnergy < 1 || e.DrinkEnergy > MaxDrinkEnergy:
		return ErrMineDrinkEnergy
	}

	return nil
}

// DefaultMine is the mine until the administrators change it. A point of
// energy comes back in 15 minutes, so a miner has about a hundred of them
// a day; a dig of stone sells for about 3 votes, of a crystal — for 60.
func DefaultMine() Mine {
	return Mine{
		veins: []OreVein{
			{Ore: OreStone, Experience: 0, Least: 120, Most: 180, Price: 2, Gain: 10, Energy: 1},
			{Ore: OreCoal, Experience: 500, Least: 100, Most: 140, Price: 5, Gain: 15, Energy: 1},
			{Ore: OreIron, Experience: 2_000, Least: 80, Most: 120, Price: 10, Gain: 25, Energy: 1},
			{Ore: OreGold, Experience: 8_000, Least: 40, Most: 60, Price: 40, Gain: 40, Energy: 2},
			{Ore: OreDiamond, Experience: 30_000, Least: 20, Most: 30, Price: 140, Gain: 60, Energy: 2},
			{Ore: OreCrystal, Experience: 100_000, Least: 10, Most: 14, Price: 500, Gain: 100, Energy: 3},
		},
		energy: 10,
		regen:  15 * time.Minute,
		events: MineEvents{
			Nugget: 300, NuggetTimes: 3,
			Chest: 100, ChestLeast: 20, ChestMost: 200,
			Collapse: 500,
			Drink:    200, DrinkEnergy: 3,
		},
	}
}

func (m Mine) Veins() []OreVein {
	return append([]OreVein(nil), m.veins...)
}

// Energy is the most energy a miner has.
func (m Mine) Energy() int {
	return m.energy
}

// Regen is how long a point of energy takes to come back.
func (m Mine) Regen() time.Duration {
	return m.regen
}

func (m Mine) Events() MineEvents {
	return m.events
}

// Vein is how ore is mined.
func (m Mine) Vein(ore Ore) (OreVein, error) {
	for _, v := range m.veins {
		if v.Ore == ore {
			return v, nil
		}
	}

	return OreVein{}, ErrUnknownOre
}

// Event is the event a roll in [0, MineOdds) falls on.
func (m Mine) Event(roll int64) MineEvent {
	for _, e := range []struct {
		event  MineEvent
		chance int
	}{
		{MineEventNugget, m.events.Nugget},
		{MineEventChest, m.events.Chest},
		{MineEventCollapse, m.events.Collapse},
		{MineEventDrink, m.events.Drink},
	} {
		if roll < int64(e.chance) {
			return e.event
		}

		roll -= int64(e.chance)
	}

	return MineEventNone
}

// Miner is a page in the mine: its energy as counted at a time — it comes
// back as time goes, — its experience, the ore in its backpack, how many
// digs it made and how many votes it took out of the mine.
type Miner struct {
	energy     int
	countedAt  time.Time
	experience int64
	backpack   map[Ore]int64
	digs       int64
	earned     int64
}

// NewMiner is a page that comes to the mine for the first time: with all
// the energy.
func NewMiner(mine Mine, now time.Time) Miner {
	return Miner{energy: mine.energy, countedAt: now.UTC(), backpack: map[Ore]int64{}}
}

// RestoreMiner is a miner as kept.
func RestoreMiner(energy int, countedAt time.Time, experience int64, backpack map[Ore]int64, digs, earned int64) Miner {
	kept := make(map[Ore]int64, len(backpack))
	for o, units := range backpack {
		if units > 0 {
			kept[o] = units
		}
	}

	return Miner{energy: energy, countedAt: countedAt.UTC(), experience: experience, backpack: kept, digs: digs, earned: earned}
}

// Energy is the energy as counted at CountedAt.
func (m Miner) Energy() int {
	return m.energy
}

func (m Miner) CountedAt() time.Time {
	return m.countedAt
}

func (m Miner) Experience() int64 {
	return m.experience
}

// Backpack is the ore mined and not sold yet.
func (m Miner) Backpack() map[Ore]int64 {
	backpack := make(map[Ore]int64, len(m.backpack))
	for o, units := range m.backpack {
		backpack[o] = units
	}

	return backpack
}

func (m Miner) Digs() int64 {
	return m.digs
}

// Earned is the votes the miner took out of the mine: sales and chests.
func (m Miner) Earned() int64 {
	return m.earned
}

// At is the miner at now: the energy that came back since it was counted.
// A full miner is counted at now, a miner short of energy — at the moment
// its last point came back, so the part of the next one is not lost.
func (m Miner) At(mine Mine, now time.Time) Miner {
	now = now.UTC()
	if m.energy >= mine.energy {
		m.energy, m.countedAt = max(m.energy, mine.energy), now

		return m
	}
	if now.Before(m.countedAt) {
		return m
	}

	points := now.Sub(m.countedAt) / mine.regen
	if int64(points) >= int64(mine.energy-m.energy) {
		m.energy, m.countedAt = mine.energy, now

		return m
	}

	m.energy += int(points)
	m.countedAt = m.countedAt.Add(points * mine.regen)

	return m
}

// NextEnergy is how long the next point of energy takes at now, 0 when
// the miner is full.
func (m Miner) NextEnergy(mine Mine, now time.Time) time.Duration {
	m = m.At(mine, now)
	if m.energy >= mine.energy {
		return 0
	}

	return mine.regen - now.UTC().Sub(m.countedAt)
}

// MineRolls are the random numbers of a dig: Event in [0, MineOdds), Yield
// in [0, Most-Least] of the ore and Chest in [0, ChestMost-ChestLeast].
type MineRolls struct {
	Event int64
	Yield int64
	Chest int64
}

// Haul is what a dig gave: the ore and how much of it, the event, the
// votes of a chest and the experience.
type Haul struct {
	Ore        Ore
	Units      int64
	Event      MineEvent
	Votes      int64
	Experience int64
}

// Dig is the miner after digging ore at now and what it gave;
// ErrUnknownOre, ErrOreLocked, ErrNoEnergy.
func (m Miner) Dig(mine Mine, ore Ore, now time.Time, rolls MineRolls) (Miner, Haul, error) {
	vein, err := mine.Vein(ore)
	if err != nil {
		return Miner{}, Haul{}, err
	}
	if m.experience < vein.Experience {
		return Miner{}, Haul{}, ErrOreLocked
	}

	m = m.At(mine, now)
	if m.energy < vein.Energy {
		return Miner{}, Haul{}, ErrNoEnergy
	}

	full := m.energy >= mine.energy
	m.energy -= vein.Energy
	if full {
		// The energy starts to come back from now.
		m.countedAt = now.UTC()
	}

	haul := Haul{Ore: ore, Event: mine.Event(rolls.Event), Units: vein.Least + rolls.Yield, Experience: vein.Gain}

	switch haul.Event {
	case MineEventNugget:
		haul.Units *= int64(mine.events.NuggetTimes)
	case MineEventChest:
		haul.Votes = mine.events.ChestLeast + rolls.Chest
	case MineEventCollapse:
		haul.Units, haul.Experience = 0, 0
	case MineEventDrink:
		m.energy = min(m.energy+mine.events.DrinkEnergy, mine.energy)
		if m.energy >= mine.energy {
			m.countedAt = now.UTC()
		}
	case MineEventNone:
	}

	m.backpack = m.Backpack()
	if haul.Units > 0 {
		m.backpack[ore] += haul.Units
	}
	m.experience += haul.Experience
	m.earned += haul.Votes
	m.digs++

	return m, haul, nil
}

// SaleValue is what units of ore sell for: whole votes, the rest of a vote
// is lost.
func (v OreVein) SaleValue(units int64) int64 {
	return units * v.Price / 100
}

// Sell is the miner after selling all of ore and the votes it got;
// ErrUnknownOre, ErrNothingToSell when that is not a whole vote.
func (m Miner) Sell(mine Mine, ore Ore) (Miner, int64, int64, error) {
	vein, err := mine.Vein(ore)
	if err != nil {
		return Miner{}, 0, 0, err
	}

	units := m.backpack[ore]

	votes := vein.SaleValue(units)
	if votes < 1 {
		return Miner{}, 0, 0, ErrNothingToSell
	}

	m.backpack = m.Backpack()
	delete(m.backpack, ore)
	m.earned += votes

	return m, units, votes, nil
}

// CheckSellAt is ErrMineNotYet unless u may sell ore at now: the same
// pages as take the daily bonus, so new pages do not farm votes.
func (u *User) CheckSellAt(now time.Time) error {
	if err := u.CheckBonusAt(now); err != nil {
		return ErrMineNotYet
	}

	return nil
}

// MineStats is how the mine does: digs made, votes paid for ore and found
// in chests, and how many pages dig.
type MineStats struct {
	Digs   int64
	Sold   int64
	Chests int64
	Miners int64
}
