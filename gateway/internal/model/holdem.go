package model

// HoldemSeat is a seat of a poker table as the viewer sees it; UserID is
// empty on a free seat, Cards are the viewer's own and those shown.
type HoldemSeat struct {
	UserID   string
	Stack    int64
	Bet      int64
	Cards    []string
	HasCards bool
	Folded   bool
	AllIn    bool
	Away     bool
	Leaving  bool
	Last     string
	Won      int64
	Hand     string
	Best     []string
}

// HoldemTable is a poker table of people: no-limit Texas Hold'em. You is
// the seat of the viewer, -1 for none; ToCall and the raise limits are
// theirs.
type HoldemTable struct {
	ID         string
	Name       string
	Private    bool
	OwnerID    string
	BigBlind   int64
	SmallBlind int64
	MinBuyIn   int64
	MaxBuyIn   int64
	Street     string
	Hand       int64
	Button     int
	Turn       int
	TurnMs     int64
	Board      []string
	Pot        int64
	CurrentBet int64
	Version    int64
	Seats      []HoldemSeat
	Players    int
	You        int
	ToCall     int64
	RaiseLeast int64
	RaiseMost  int64
	Invited    bool
}

// NewHoldemTable is what a table is opened with.
type NewHoldemTable struct {
	Name     string
	Private  bool
	Seats    int
	BigBlind int64
	BuyIn    int64
}
