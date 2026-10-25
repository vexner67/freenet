package user_test

import (
	"errors"
	"math/rand/v2"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/user"
)

// cards are the cards of "As Kd 2c".
func cards(t *testing.T, s string) []user.Card {
	t.Helper()

	var out []user.Card
	for _, name := range strings.Fields(s) {
		rank := strings.IndexByte("23456789TJQKA", name[0])
		suit := strings.IndexByte("shdc", name[1])
		if rank < 0 || suit < 0 {
			t.Fatalf("bad card %q", name)
		}
		out = append(out, user.Card(rank*4+suit))
	}

	return out
}

func TestCard_String(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"2s", "Th", "Ad", "Kc"} {
		if got := cards(t, name)[0].String(); got != name {
			t.Errorf("String() = %q, want %q", got, name)
		}
	}
}

func TestBestHand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		cards string
		want  user.HandRank
	}{
		{"As Ks Qs Js Ts 2d 3c", user.RoyalFlush},
		{"9h 8h 7h 6h 5h Ad Ac", user.StraightFlush},
		{"Ah 2h 3h 4h 5h Kd Kc", user.StraightFlush},
		{"7s 7h 7d 7c 2s 3d 9c", user.FourOfAKind},
		{"7s 7h 7d 2c 2s 3d 9c", user.FullHouse},
		{"As 9s 7s 4s 2s Kd Kc", user.Flush},
		{"Ad 2s 3c 4h 5d 9c Kc", user.Straight},
		{"Td Js Qc Kh Ad 2c 2h", user.Straight},
		{"7s 7h 7d 2c 4s 9d Kc", user.ThreeOfAKind},
		{"7s 7h 2d 2c 4s 9d Kc", user.TwoPair},
		{"7s 7h 2d 3c 4s 9d Kc", user.OnePair},
		{"7s 8h 2d 3c 4s Jd Kc", user.HighCard},
	}
	for _, tt := range tests {
		if got := user.BestHand(cards(t, tt.cards)).Rank; got != tt.want {
			t.Errorf("BestHand(%s) = %v, want %v", tt.cards, got, tt.want)
		}
	}
}

func TestBestHand_Order(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		better, worse string
	}{
		{"kicker", "As Ah Kd 9c 5s", "Ac Ad Qh 9s 5d"},
		{"two pair", "Ks Kh 3d 3c 2s", "Qs Qh Jd Jc As"},
		{"wheel is the lowest straight", "2s 3h 4d 5c 6s", "As 2h 3d 4c 5s"},
		{"full house by three", "3s 3h 3d 2c 2s", "2h 2d 2s Ac Ad"},
		{"flush by its top", "Ks 9s 7s 4s 2s", "Qh Jh 9h 8h 6h"},
	}
	for _, tt := range tests {
		b, w := user.BestHand(cards(t, tt.better)), user.BestHand(cards(t, tt.worse))
		if b.Score <= w.Score {
			t.Errorf("%s: %s scores %d, not above %s (%d)", tt.name, tt.better, b.Score, tt.worse, w.Score)
		}
	}

	a, b := user.BestHand(cards(t, "As Kh 9d 5c 3s")), user.BestHand(cards(t, "Ad Kc 9h 5s 3d"))
	if a.Score != b.Score {
		t.Errorf("the same ranks score %d and %d", a.Score, b.Score)
	}
}

func TestHand_Qualifies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		cards string
		want  bool
	}{
		{"4s 4h Kd 9c 2s", true},
		{"3s 3h Kd 9c 2s", false},
		{"As Kh Qd 9c 2s", false},
		{"3s 3h 2d 2c 9s", true},
	}
	for _, tt := range tests {
		if got := user.BestHand(cards(t, tt.cards)).Qualifies(); got != tt.want {
			t.Errorf("Qualifies(%s) = %v, want %v", tt.cards, got, tt.want)
		}
	}
}

func TestNewPoker(t *testing.T) {
	t.Parallel()

	pays := [user.PokerPays]int{100, 20, 10, 3, 2, 1}
	tests := []struct {
		name        string
		least, most int64
		pays        [user.PokerPays]int
		want        error
	}{
		{name: "fine", least: 1, most: 100, pays: pays},
		{name: "zero least", least: 0, most: 100, pays: pays, want: user.ErrPokerLimits},
		{name: "least above most", least: 10, most: 5, pays: pays, want: user.ErrPokerLimits},
		{name: "too big", least: 1, most: user.MaxBet, pays: pays, want: user.ErrPokerLimits},
		{name: "zero pay", least: 1, most: 100, pays: [user.PokerPays]int{100, 20, 10, 3, 2, 0}, want: user.ErrPokerPays},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if _, err := user.NewPoker(tt.least, tt.most, tt.pays); !errors.Is(err, tt.want) {
				t.Errorf("NewPoker() error = %v, want %v", err, tt.want)
			}
		})
	}
}

// deal is a hand of ante 10 with the player's cards, the dealer's and the
// board's.
func deal(t *testing.T, player, dealer, board string) *user.PokerHand {
	t.Helper()

	deck := cards(t, player+" "+dealer+" "+board)
	h, err := user.DefaultPoker().Deal(uuid.New(), uuid.New(), 10, deck, time.Now())
	if err != nil {
		t.Fatalf("Deal() error = %v", err)
	}

	return h
}

func TestPokerHand_Call(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                  string
		player, dealer, board string
		state                 user.PokerState
		payout                int64
	}{
		// The ante paid 1 to 1 (20) and the call 1 to 1 (40).
		{"won with a pair", "As Ah", "Ks Kh", "2d 7c 9s Jd 3h", user.PokerWon, 60},
		// A flush pays the ante 2 to 1.
		{"won with a flush", "As 2s", "Kd Kh", "5s 7s 9s Jd 3h", user.PokerWon, 70},
		{"lost", "Ks Kh", "As Ah", "2d 7c 9s Jd 3h", user.PokerLost, 0},
		// The dealer's pair of threes: the ante paid, the call back.
		{"unqualified", "Ks Qh", "3s 3h", "2d 7c 9s Jd 5h", user.PokerUnqualified, 40},
		{"tied", "Ks Qh", "Kd Qc", "As Ad 9s 9d 5h", user.PokerTied, 30},
		// A royal flush pays the ante 100 to 1 though the dealer did
		// not qualify.
		{"royal", "As Ks", "2h 3c", "Qs Js Ts 7d 8c", user.PokerUnqualified, 1030},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h, err := deal(t, tt.player, tt.dealer, tt.board).Call(user.DefaultPoker())
			if err != nil {
				t.Fatalf("Call() error = %v", err)
			}
			if h.State() != tt.state || h.Payout() != tt.payout {
				t.Errorf("Call() = %s, %d; want %s, %d", h.State(), h.Payout(), tt.state, tt.payout)
			}
			if len(h.Dealer()) != 2 || len(h.Board()) != 5 {
				t.Errorf("a called hand shows %d cards of the dealer and %d of the board", len(h.Dealer()), len(h.Board()))
			}
			if _, err := h.Call(user.DefaultPoker()); !errors.Is(err, user.ErrPokerOver) {
				t.Errorf("Call() again error = %v, want ErrPokerOver", err)
			}
		})
	}
}

func TestPokerHand_Fold(t *testing.T) {
	t.Parallel()

	h := deal(t, "As Ah", "Ks Kh", "2d 7c 9s Jd 3h")
	if len(h.Dealer()) != 0 || len(h.Board()) != 3 {
		t.Fatalf("a dealt hand shows %d cards of the dealer and %d of the board", len(h.Dealer()), len(h.Board()))
	}

	folded, err := h.Fold()
	if err != nil {
		t.Fatalf("Fold() error = %v", err)
	}
	if folded.State() != user.PokerFolded || folded.Payout() != 0 || len(folded.Dealer()) != 0 || len(folded.Board()) != 3 {
		t.Errorf("Fold() = %s, %d with %d cards of the dealer", folded.State(), folded.Payout(), len(folded.Dealer()))
	}
	if _, err := folded.Fold(); !errors.Is(err, user.ErrPokerOver) {
		t.Errorf("Fold() again error = %v, want ErrPokerOver", err)
	}
}

func TestPoker_Deal(t *testing.T) {
	t.Parallel()

	if _, err := user.DefaultPoker().Deal(uuid.New(), uuid.New(), 101, make([]user.Card, user.DeckSize), time.Now()); !errors.Is(err, user.ErrPokerAnte) {
		t.Errorf("Deal() error = %v, want ErrPokerAnte", err)
	}
}

// TestPoker_Return plays many hands, calling every one, to see the
// default table gives back less than all that is bet and more than 95%
// (about 97%; folding the worst hands brings it near 99%).
func TestPoker_Return(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewPCG(1, 2))
	deck := make([]user.Card, user.DeckSize)
	for i := range deck {
		deck[i] = user.Card(i)
	}

	var bet, back int64
	for range 200_000 {
		r.Shuffle(len(deck), func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })

		h, err := user.DefaultPoker().Deal(uuid.Nil, uuid.Nil, 1, deck, time.Time{})
		if err != nil {
			t.Fatalf("Deal() error = %v", err)
		}
		if h, err = h.Call(user.DefaultPoker()); err != nil {
			t.Fatalf("Call() error = %v", err)
		}
		bet += 3
		back += h.Payout()
	}

	if ret := float64(back) / float64(bet); ret < 0.95 || ret >= 1 {
		t.Errorf("the table gives back %.4f of what is bet", ret)
	}
}
