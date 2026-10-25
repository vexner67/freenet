package model

// Poker is the table of "Покер": antes from Least to Most votes and the
// pays of the ante to one by the hand of the player — straight, flush, full
// house, four of a kind, straight flush, royal flush.
type Poker struct {
	Least int64
	Most  int64
	Pays  []int
}

// PokerHand is a hand of "Покер" as its player sees it; cards are "As",
// "Td". The dealer and his hand are empty until the call; Payout is what
// the hand paid back, the bets included.
type PokerHand struct {
	ID         string
	Ante       int64
	State      string
	Player     []string
	Board      []string
	Dealer     []string
	Payout     int64
	PlayerHand string
	DealerHand string
	PlayerBest []string
	DealerBest []string
	Balance    int64
}

// PokerTable is the table and the hand of the viewer waiting, if any.
type PokerTable struct {
	Poker Poker
	Dealt *PokerHand
}

// PokerStats is how "Покер" does: the hands dealt and the votes they took
// and paid.
type PokerStats struct {
	Hands int64
	Spent int64
	Won   int64
}
