package model

import "time"

// World is one generated map of "Мир": Status is "generating", "ready",
// "open", "ended" or "failed"; one world at a time is open, and the game
// is played in it.
type World struct {
	ID        string
	Seed      uint64
	Grid      int
	Status    string
	CreatedAt time.Time
	OpenedAt  time.Time
	EndedAt   time.Time
}

// WorldRules are the rules of the game the administrators set; the
// sums are votes, Income is a share of the population a province pays.
type WorldRules struct {
	FoundCost       int64
	FoundTreasury   int64
	FoundLands      int
	CapitalDistance int
	ProtectionDays  int
	ColonizeCost    int64
	ColonizeHours   int
	Income          int64
}

// Flag is the flag of a state: a pattern of up to three colors of the
// palette and an emblem in the middle or the canton.
type Flag struct {
	Pattern     string
	Colors      []int
	Emblem      string
	EmblemColor int
	EmblemAt    string
}

// State is a state of the open world. Treasury and Income are in
// hundredths of a vote (Income an hour); Lands, Population (thousands)
// and Citizens are counted from the game.
type State struct {
	ID             string
	Name           string
	Adjective      string
	Motto          string
	Color          int
	Flag           Flag
	Government     string
	RulerID        string
	Capital        int
	Treasury       int64
	FoundedAt      time.Time
	ProtectedUntil time.Time
	ClubID         string
	Lands          int
	Population     int
	Citizens       int
	Income         int64
}

// Colony is a province being settled, the state's from Until on.
type Colony struct {
	Province int
	StateID  string
	Until    time.Time
}

// Standing is where the viewer is in the world: a citizen of StateID in
// Role, or one who applied to AppliedTo.
type Standing struct {
	StateID   string
	Role      string
	AppliedTo string
}

// WorldMap is the open world as the map shows it: Owners has a number a
// province, 0 for none and the index in States plus one for a state.
type WorldMap struct {
	World    World
	Rules    WorldRules
	Hour     time.Time
	States   []State
	Owners   []int
	Colonies []Colony
	Viewer   Standing
}

// Atlas is what the map knows of its provinces: their names, the land
// and its riches, and who borders whom.
type Atlas struct {
	Width     int
	Height    int
	Provinces []Province
}

type Province struct {
	ID         int
	Sea        bool
	Name       string
	Terrain    string
	Biome      string
	Resources  []string
	CenterX    int
	CenterY    int
	Area       int
	Coastal    bool
	Culture    int
	Population int
	FreeCity   bool
	Links      []ProvinceLink
}

// ProvinceLink is a border: over a river, or a crossing by sea.
type ProvinceLink struct {
	To    int
	River bool
	Sea   bool
}

type Citizen struct {
	UserID   string
	Role     string
	JoinedAt time.Time
}

type CitizenshipApplication struct {
	UserID string
	At     time.Time
}

// BudgetHour is what came into and went out of a treasury in an hour, in
// hundredths of a vote; spending is negative.
type BudgetHour struct {
	Hour         time.Time
	Income       int64
	Donations    int64
	Founding     int64
	Colonization int64
}

// StateView is the page of a state: the applications only for those who
// answer them, ColonyCost what the next colony costs, in hundredths.
type StateView struct {
	State        State
	Lands        []int
	Citizens     []Citizen
	Colonies     []Colony
	Applications []CitizenshipApplication
	ColonyCost   int64
	Budget       []BudgetHour
	Viewer       Standing
}

// WorldEvent is a line of the news of the world.
type WorldEvent struct {
	ID       string
	Kind     string
	StateID  string
	UserID   string
	Province int
	Name     string
	Role     string
	At       time.Time
}

// Founding is what a new state is founded with.
type Founding struct {
	Name       string
	Adjective  string
	Motto      string
	Color      int
	Flag       Flag
	Government string
	Capital    int
}

// StateLook is what the ruler may change of a state.
type StateLook struct {
	Adjective string
	Motto     string
	Color     int
	Flag      Flag
}
