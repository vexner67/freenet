package model

import "time"

// BetEvent is an event the administrators ask about: people stake votes on
// its outcomes, and a stake on the winner pays its votes times the odds it
// was made at (on an event without odds those who guessed share all the
// stakes). State is "open", "closed", "resolved" or "cancelled"; Winner
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

// BetOutcome is an answer to the question of an event with its odds (in
// hundredths, 0 for an event without them), the votes and the people on
// it, and the votes of the viewer with what they pay if it wins.
type BetOutcome struct {
	Name      string
	Odds      int
	Staked    int64
	Bettors   int
	Mine      int64
	Potential int64
}

// BetEventDraft is what a new event asks; ClosesAt is zero for stakes
// until the administrators close them.
type BetEventDraft struct {
	Question string
	Outcomes []string
	Odds     []int
	ClosesAt time.Time
}
