package model

import "time"

// DatingCard is how a person shows themselves in "Знакомства": the goal
// ("friendship", "chat", "love"), the gender sought (empty for anyone) and
// a text about them.
type DatingCard struct {
	UserID    string
	Goal      string
	Seek      string
	About     string
	On        bool
	UpdatedAt time.Time
}

// DatingMatch is a person who liked the viewer back.
type DatingMatch struct {
	UserID string
	At     time.Time
}

// Dating is "Знакомства" of the viewer: their card (nil before they made
// one), the cards to answer and the matches.
type Dating struct {
	Card    *DatingCard
	Deck    []DatingCard
	Matches []DatingMatch
}

// DatingCardInput is what a person tells on their card.
type DatingCardInput struct {
	Goal  string
	Seek  string
	About string
	On    bool
}
