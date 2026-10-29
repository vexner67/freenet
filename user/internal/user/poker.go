package user

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// "Покер" of the casino is Casino Hold'em, the poker of online casinos
// played against the dealer: the player puts an ante and gets two cards,
// the dealer two face down, and three cards of the board are laid open
// (the flop). The player folds, losing the ante, or calls twice the ante;
// then the turn and the river are laid and the dealer opens. A dealer
// without a pair of fours or better does not qualify: the ante is paid by
// the table and the call comes back. Otherwise the better five of seven
// cards wins: the player's win pays the ante by the table and the call
// one to one, the dealer's takes both, and a tie gives both back.

// Card is one of the 52 cards: rank*4 + suit, the rank 0 for a two up to
// 12 for an ace, the suits spades, hearts, diamonds and clubs.
type Card int

const (
	DeckSize   = 52
	pokerCards = 9
)

const (
	cardRanks = "23456789TJQKA"
	cardSuits = "shdc"
)

// Rank is 2 for a two up to 14 for an ace.
func (c Card) Rank() int {
	return int(c)/4 + 2
}

func (c Card) Suit() int {
	return int(c) % 4
}

// String is the rank and the suit, "As" for the ace of spades, "Td" for
// the ten of diamonds.
func (c Card) String() string {
	if c < 0 || c >= DeckSize {
		return ""
	}

	return string(cardRanks[c.Rank()-2]) + string(cardSuits[c.Suit()])
}

// HandRank is how strong five cards are, from a high card up; a royal
// flush is a straight flush to the ace, but the table pays it apart.
type HandRank int

const (
	HighCard HandRank = iota
	OnePair
	TwoPair
	ThreeOfAKind
	Straight
	Flush
	FullHouse
	FourOfAKind
	StraightFlush
	RoyalFlush
)

var handNames = [...]string{"high_card", "pair", "two_pair", "three", "straight", "flush", "full_house", "four", "straight_flush", "royal_flush"}

// String is the name of the rank the site translates: "pair", "flush".
func (r HandRank) String() string {
	if r < HighCard || r > RoyalFlush {
		return ""
	}

	return handNames[r]
}

// Hand is the best five of some cards: its rank, a score that orders all
// hands (the greater wins, equal ones tie) and the five cards.
type Hand struct {
	Rank  HandRank
	Score int
	Cards [5]Card
}

// BestHand is the best five of cards (five to seven of them).
func BestHand(cards []Card) Hand {
	var (
		best  Hand
		found bool
		pick  [5]Card
	)

	var choose func(from, n int)
	choose = func(from, n int) {
		if n == 5 {
			if h := scoreFive(pick); !found || h.Score > best.Score {
				best, found = h, true
			}
			return
		}
		for i := from; i <= len(cards)-(5-n); i++ {
			pick[n] = cards[i]
			choose(i+1, n+1)
		}
	}
	choose(0, 0)

	return best
}

// scoreFive is the hand of five cards: the score is the rank and then the
// ranks of the cards that decide, most telling first, in base 16.
func scoreFive(cards [5]Card) Hand {
	var counts [15]int

	flush := true
	for _, c := range cards {
		counts[c.Rank()]++
		if c.Suit() != cards[0].Suit() {
			flush = false
		}
	}

	// The ranks that decide: by how many cards share them, then higher.
	var order []int
	for n := 4; n >= 1; n-- {
		for r := 14; r >= 2; r-- {
			if counts[r] == n {
				order = append(order, r)
			}
		}
	}

	high, straight := straightHigh(counts)

	rank := HighCard
	switch {
	case straight && flush && high == 14:
		rank = RoyalFlush
	case straight && flush:
		rank = StraightFlush
	case counts[order[0]] == 4:
		rank = FourOfAKind
	case counts[order[0]] == 3 && counts[order[1]] == 2:
		rank = FullHouse
	case flush:
		rank = Flush
	case straight:
		rank = Straight
	case counts[order[0]] == 3:
		rank = ThreeOfAKind
	case counts[order[0]] == 2 && counts[order[1]] == 2:
		rank = TwoPair
	case counts[order[0]] == 2:
		rank = OnePair
	}

	if straight {
		order = []int{high}
	}

	// A royal flush scores as the straight flush it is.
	score := int(min(rank, StraightFlush))
	for i := range 5 {
		score *= 16
		if i < len(order) {
			score += order[i]
		}
	}

	return Hand{Rank: rank, Score: score, Cards: cards}
}

// straightHigh is the highest card of a straight in counts, the five for
// the wheel from the ace up to the five.
func straightHigh(counts [15]int) (int, bool) {
	for high := 14; high >= 6; high-- {
		if counts[high] == 1 && counts[high-1] == 1 && counts[high-2] == 1 && counts[high-3] == 1 && counts[high-4] == 1 {
			return high, true
		}
	}
	if counts[14] == 1 && counts[2] == 1 && counts[3] == 1 && counts[4] == 1 && counts[5] == 1 {
		return 5, true
	}

	return 0, false
}

// Qualifies tells whether the dealer plays with a hand: a pair of fours
// or better.
func (h Hand) Qualifies() bool {
	if h.Rank > OnePair {
		return true
	}

	return h.Rank == OnePair && h.Score>>16&0xf >= 4
}

// The pays of the ante, "to one", by the hand of the player: a royal
// flush, a straight flush, four of a kind, a full house, a flush and the
// rest — a straight or less.
const PokerPays = 6

// The bounds of a pay, to one.
const (
	MinPokerPay = 1
	MaxPokerPay = MaxMultiplier
)

var (
	ErrPokerLimits   = errors.New("poker antes must be at least 1 vote, the least not above the most")
	ErrPokerPays     = errors.New("the pays of poker must be from 1 to 10000 to one")
	ErrPokerAnte     = errors.New("the ante is out of the limits of poker")
	ErrPokerNotFound = errors.New("poker hand not found")
	ErrPokerOver     = errors.New("the poker hand is over")
	ErrPokerOpen     = errors.New("a poker hand is under way")
)

// Poker is the table of "Покер": the antes from Least to Most votes and
// the pays of the ante.
type Poker struct {
	least, most int64
	pays        [PokerPays]int
}

// NewPoker checks the table; ErrPokerLimits, ErrPokerPays.
func NewPoker(least, most int64, pays [PokerPays]int) (Poker, error) {
	// The call is twice the ante, so the ante stays a third of a bet.
	if least < 1 || most > MaxBet/3 || least > most {
		return Poker{}, ErrPokerLimits
	}
	for _, p := range pays {
		if p < MinPokerPay || p > MaxPokerPay {
			return Poker{}, ErrPokerPays
		}
	}

	return Poker{least: least, most: most, pays: pays}, nil
}

// DefaultPoker is the table of online casinos until the administrators
// change it: antes of 1 to 100 votes, paid 100, 20, 10, 3, 2 and 1 to
// one — about 97% of what is bet back calling every hand, near 99%
// folding the worst.
func DefaultPoker() Poker {
	return Poker{least: 1, most: 100, pays: [PokerPays]int{100, 20, 10, 3, 2, 1}}
}

func (p Poker) Least() int64 {
	return p.least
}

func (p Poker) Most() int64 {
	return p.most
}

func (p Poker) Pays() [PokerPays]int {
	return p.pays
}

// Pay is what the ante is paid to one for a hand of the player.
func (p Poker) Pay(r HandRank) int {
	switch r {
	case RoyalFlush:
		return p.pays[0]
	case StraightFlush:
		return p.pays[1]
	case FourOfAKind:
		return p.pays[2]
	case FullHouse:
		return p.pays[3]
	case Flush:
		return p.pays[4]
	default:
		return p.pays[5]
	}
}

// PokerState is where a hand is: dealt and waiting for the player, or
// over — folded, won, lost, tied, or won as the dealer did not qualify.
type PokerState string

const (
	PokerDealt       PokerState = "dealt"
	PokerFolded      PokerState = "folded"
	PokerWon         PokerState = "won"
	PokerLost        PokerState = "lost"
	PokerTied        PokerState = "tied"
	PokerUnqualified PokerState = "unqualified"
)

// PokerHand is one hand of "Покер": the cards are the player's two, the
// dealer's two and the board's five, drawn at the deal.
type PokerHand struct {
	id        uuid.UUID
	userID    uuid.UUID
	ante      int64
	cards     [pokerCards]Card
	state     PokerState
	payout    int64
	startedAt time.Time
}

// Deal is a hand of ante votes with the cards of a shuffled deck, the
// first nine of it; ErrPokerAnte.
func (p Poker) Deal(id, userID uuid.UUID, ante int64, deck []Card, now time.Time) (*PokerHand, error) {
	if ante < p.least || ante > p.most {
		return nil, ErrPokerAnte
	}

	h := &PokerHand{id: id, userID: userID, ante: ante, state: PokerDealt, startedAt: now.UTC()}
	copy(h.cards[:], deck)

	return h, nil
}

// RestorePokerHand is a hand as the storage keeps it.
func RestorePokerHand(id, userID uuid.UUID, ante int64, cards [pokerCards]Card, state PokerState, payout int64, startedAt time.Time) *PokerHand {
	return &PokerHand{id: id, userID: userID, ante: ante, cards: cards, state: state, payout: payout, startedAt: startedAt.UTC()}
}

func (h *PokerHand) ID() uuid.UUID {
	return h.id
}

func (h *PokerHand) UserID() uuid.UUID {
	return h.userID
}

func (h *PokerHand) Ante() int64 {
	return h.ante
}

// Cards are all nine cards: not to be shown while the hand is dealt.
func (h *PokerHand) Cards() [pokerCards]Card {
	return h.cards
}

func (h *PokerHand) State() PokerState {
	return h.state
}

func (h *PokerHand) Payout() int64 {
	return h.payout
}

func (h *PokerHand) StartedAt() time.Time {
	return h.startedAt
}

// Over tells whether the hand is decided.
func (h *PokerHand) Over() bool {
	return h.state != PokerDealt
}

// Called tells whether the player called: what the hand cost is the ante
// and twice it then.
func (h *PokerHand) Called() bool {
	return h.state != PokerDealt && h.state != PokerFolded
}

// Owned tells whether the hand is userID's; one not theirs is not found.
func (h *PokerHand) Owned(userID uuid.UUID) bool {
	return h.userID == userID
}

// Player are the two cards of the player.
func (h *PokerHand) Player() []Card {
	return h.cards[0:2]
}

// Dealer are the two cards of the dealer: none while the hand is dealt
// or was folded — the dealer does not open then.
func (h *PokerHand) Dealer() []Card {
	if !h.Called() {
		return nil
	}

	return h.cards[2:4]
}

// Board are the cards laid open: the flop while the hand is dealt or was
// folded, all five once called.
func (h *PokerHand) Board() []Card {
	if !h.Called() {
		return h.cards[4:7]
	}

	return h.cards[4:9]
}

// PlayerHand is the best hand of the player of the cards open.
func (h *PokerHand) PlayerHand() Hand {
	return BestHand(append(append([]Card{}, h.Player()...), h.Board()...))
}

// DealerHand is the best hand of the dealer, once open.
func (h *PokerHand) DealerHand() (Hand, bool) {
	if !h.Called() {
		return Hand{}, false
	}

	return BestHand(append(append([]Card{}, h.Dealer()...), h.Board()...)), true
}

// Fold gives the hand up, the ante lost; ErrPokerOver.
func (h *PokerHand) Fold() (*PokerHand, error) {
	if h.Over() {
		return nil, ErrPokerOver
	}

	folded := *h
	folded.state = PokerFolded

	return &folded, nil
}

// Call plays the hand to the end by the table of p: the call costs twice
// the ante, and Payout is all the hand gives back, the ante and the call
// with them; ErrPokerOver.
func (h *PokerHand) Call(p Poker) (*PokerHand, error) {
	if h.Over() {
		return nil, ErrPokerOver
	}

	called := *h
	called.state = PokerLost

	player := BestHand(append(append([]Card{}, h.cards[0:2]...), h.cards[4:9]...))
	dealer := BestHand(append(append([]Card{}, h.cards[2:4]...), h.cards[4:9]...))
	ante, call := h.ante, 2*h.ante
	won := ante + MulDiv(ante, int64(p.Pay(player.Rank)), 1)

	switch {
	case !dealer.Qualifies():
		called.state, called.payout = PokerUnqualified, won+call
	case player.Score > dealer.Score:
		called.state, called.payout = PokerWon, won+2*call
	case player.Score == dealer.Score:
		called.state, called.payout = PokerTied, ante+call
	}

	return &called, nil
}

// PokerStats are how the hands of "Покер" do: how many were dealt, the
// votes bet on them (antes and calls) and paid back.
type PokerStats struct {
	Hands int64
	Spent int64
	Won   int64
}
