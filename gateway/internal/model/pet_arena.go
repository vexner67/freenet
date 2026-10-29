package model

import "time"

// PetArena is what the pets do together, as the caller sees it: the boss
// and the tournament of the week, the tournament of the last (nil without
// one), the places of the season as they stand and of those closed, the
// tasks of the day with the days in a row all were done, the badges and
// the pets up for sale.
type PetArena struct {
	Boss         PetBoss
	Tournament   PetTournament
	Last         *PetTournament
	SeasonEndsAt time.Time
	Top          []PetSeasonPlace
	Seasons      []PetSeason
	Quests       []PetQuest
	Streak       int
	Achievements []PetAchievement
	Market       []PetListing
}

// PetBoss is the boss of a week: its health, who finished it and when,
// the people who hit it by damage and the votes it brings, LastHit more
// to the one who finishes it.
type PetBoss struct {
	Kind      string
	Health    int
	MaxHealth int
	KilledBy  string
	KilledAt  time.Time
	Hits      []PetBossHit
	Stats     PetPower
	Prize     int64
	LastHit   int64
}

type PetBossHit struct {
	UserID string
	Damage int
	Hits   int
}

// PetTournament is the tournament of a week: Places are the indexes of
// the entrants, the winner first; Bets and Pool the stakes, BetPetID and
// BetVotes the bet of the caller.
type PetTournament struct {
	Week     time.Time
	StartsAt time.Time
	Held     bool
	Entrants []PetEntrant
	Rounds   [][]PetMatch
	Places   []int
	Bets     int
	Pool     int64
	BetPetID string
	BetVotes int64
}

type PetEntrant struct {
	OwnerID string
	PetID   string
	Kind    string
	Name    string
	Level   int
	Power   int
}

// PetMatch is a fight of a tournament between the entrants A and B, -1
// for none; Bout is nil for a pet alone.
type PetMatch struct {
	A, B   int
	Winner int
	Bout   *PetFight
}

type PetSeasonPlace struct {
	OwnerID string
	PetID   string
	Kind    string
	Name    string
	Rating  int
	Prize   int64
}

type PetSeason struct {
	Season time.Time
	Places []PetSeasonPlace
}

type PetQuest struct {
	Kind     string
	Goal     int
	Progress int
}

type PetAchievement struct {
	ID    string
	GotAt time.Time
}

type PetListing struct {
	OwnerID string
	Pet     Pet
}

// PetArenaAction is something done on the arena: Votes are the stake of a
// bet or the price of a sale.
type PetArenaAction struct {
	Action string
	PetID  string
	Votes  int64
}

// PetArenaDone is the arena after an action, the hit of the boss and the
// votes after a payment, 0 when nothing was paid.
type PetArenaDone struct {
	Arena   PetArena
	Bout    *PetFight
	Balance int64
}
