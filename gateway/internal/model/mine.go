package model

// OreVein is how an ore is mined: it opens at Experience; a dig takes
// Energy, gives from Least to Most of it and Gain experience; a unit sells
// for Price hundredths of a vote.
type OreVein struct {
	Ore        string
	Experience int64
	Least      int64
	Most       int64
	Price      int64
	Gain       int64
	Energy     int
}

// MineEvents are the chances of the events of a dig, each of 10000, and
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

// MineSettings are the mine as the administrators set it: all ores in
// order, the most energy of a miner, how many seconds a point of it takes
// to come back and the events.
type MineSettings struct {
	Veins        []OreVein
	Energy       int
	RegenSeconds int
	Events       MineEvents
}

// Miner is a page in the mine now: its energy, how many seconds the next
// point takes (0 — full), its experience, the ore it has not sold, its digs
// and the votes it took out of the mine.
type Miner struct {
	Energy            int
	NextEnergySeconds int
	Experience        int64
	Backpack          map[string]int64
	Digs              int64
	Earned            int64
}

// Mine is the mine as a miner sees it.
type Mine struct {
	Settings MineSettings
	Miner    Miner
}

// MineDig is what a dig gave: units of ore, the event ("" — none), the
// votes of a chest with the balance after them, the experience, and the
// mine after it.
type MineDig struct {
	Units      int64
	Event      string
	Votes      int64
	Balance    int64
	Experience int64
	Mine       Mine
}

// MineSale is what a sale gave: units of ore, votes, the balance and the
// mine after it.
type MineSale struct {
	Units   int64
	Votes   int64
	Balance int64
	Mine    Mine
}

// MineStats is how the mine does: digs made, votes paid for ore and found
// in chests, and pages that dug.
type MineStats struct {
	Digs   int64
	Sold   int64
	Chests int64
	Miners int64
}
