package model

import "time"

// BetEvent is an event the administrators ask about: people stake votes on
// its outcomes, and those who guessed share all the stakes once it is
// resolved. State is "open", "closed", "resolved" or "cancelled"; Winner
// is the winning outcome, -1 unless resolved; ClosesAt is zero for never.
type BetEvent struct {
	ID        string
	Question  string
	Outcomes  []BetOutcome
	State     string
	Open      bool
	ClosesAt  time.Time
	Winner    int
	CreatedAt time.Time
	// People staked on any outcome; Payout is what the stakes of the
	// viewer paid back once the event was over.
	People int
	Payout int64
}

// BetOutcome is an answer to the question of an event with the votes and
// the people on it, and the votes of the viewer.
type BetOutcome struct {
	Name    string
	Staked  int64
	Bettors int
	Mine    int64
}

// BetEventDraft is what a new event asks; ClosesAt is zero for stakes
// until the administrators close them.
type BetEventDraft struct {
	Question string
	Outcomes []string
	ClosesAt time.Time
}
