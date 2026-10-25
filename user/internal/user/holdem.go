package user

import (
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// The tables of "Покер" are no-limit Texas Hold'em of people against each
// other: they buy chips for votes, which go to the Treasury, sit at a
// table, play hands with blinds and four rounds of bets, and leave with
// what is in front of them, paid back by the Treasury. The site takes no
// rake: the Treasury only keeps the chips while they are on the table.
//
// The server does not tick: the table moves on when it is read or acted
// on (HoldemTable.At), a turn past its time is checked or folded then, and
// a sweep moves on the tables nobody looks at.

const (
	MinTableSeats = 2
	MaxTableSeats = 9
	// DefaultTableSeats are the seats of a table of a quick game.
	DefaultTableSeats = 6
	MinBigBlind       = 2
	MaxBigBlind       = 1_000_000_000
	// A stack bought is from MinBuyInBlinds to MaxBuyInBlinds big blinds.
	MinBuyInBlinds = 20
	MaxBuyInBlinds = 200
	maxTableName   = 40
	// MaxTableInvites are the people a table remembers invited.
	MaxTableInvites = 50
)

const (
	// TableTurn is how long a player has to act.
	TableTurn = 30 * time.Second
	// TablePause is between hands: the result stays on the table.
	TablePause = 6 * time.Second
	// tableStart is the pause before the first hand of new players.
	tableStart = 3 * time.Second
	// tableMissed turns let pass in a row sit a player out.
	tableMissed = 2
	// TableAway is how long a player sits out before being stood up with
	// their chips.
	TableAway = 5 * time.Minute
)

// TableStakes are the big blinds of the quick games.
var TableStakes = []int64{2, 10, 50, 200, 1000}

var (
	ErrTableName        = errors.New("the name of a table must be one line of up to 40 characters")
	ErrTableSeats       = errors.New("a table has from 2 to 9 seats")
	ErrTableBlind       = errors.New("the big blind must be from 2 to 1000000000 votes")
	ErrTableStake       = errors.New("there are no quick games of this big blind")
	ErrTableBuyIn       = errors.New("the buy-in is out of the limits of the table")
	ErrTableNotFound    = errors.New("poker table not found")
	ErrTableFull        = errors.New("the poker table is full")
	ErrTableSeatTaken   = errors.New("the seat is taken")
	ErrTableSeated      = errors.New("you sit at this table already")
	ErrTableNotSeated   = errors.New("you do not sit at this table")
	ErrTableNotInvited  = errors.New("this table is for the invited only")
	ErrTableNotYourTurn = errors.New("it is not your turn")
	ErrTableAction      = errors.New("this action is not possible now")
	ErrTableRaise       = errors.New("the raise is out of the limits")
	ErrTableInvites     = errors.New("a table invites up to 50 people")
)

// TableStreet is where a hand is: waiting for players, one of the four
// rounds of bets, or over with its result shown.
type TableStreet string

const (
	StreetWaiting  TableStreet = "waiting"
	StreetPreflop  TableStreet = "preflop"
	StreetFlop     TableStreet = "flop"
	StreetTurn     TableStreet = "turn"
	StreetRiver    TableStreet = "river"
	StreetShowdown TableStreet = "showdown"
)

// Betting tells whether bets are made on the street.
func (s TableStreet) Betting() bool {
	return s == StreetPreflop || s == StreetFlop || s == StreetTurn || s == StreetRiver
}

// TableAction is what a player does on their turn: fold, check, call,
// raise (or bet) to an amount, or all in; the blinds are posted by the
// table.
type TableAction string

const (
	ActFold       TableAction = "fold"
	ActCheck      TableAction = "check"
	ActCall       TableAction = "call"
	ActRaise      TableAction = "raise"
	ActAllIn      TableAction = "allin"
	ActSmallBlind TableAction = "small_blind"
	ActBigBlind   TableAction = "big_blind"
)

// TableSeat is a seat of a table; an empty one has no UserID. Bet is what
// the player put on this street, Put — in this hand; Won is what they won
// in the last hand.
type TableSeat struct {
	UserID    uuid.UUID
	Stack     int64
	Cards     []Card
	Bet       int64
	Put       int64
	InHand    bool
	Folded    bool
	AllIn     bool
	Acted     bool
	Away      bool
	AwaySince time.Time
	Missed    int
	Leaving   bool
	Shown     bool
	Won       int64
	Last      TableAction
}

// Taken tells whether someone sits on the seat.
func (s TableSeat) Taken() bool {
	return s.UserID != uuid.Nil
}

// contends tells whether the player still plays for the pot.
func (s TableSeat) contends() bool {
	return s.InHand && !s.Folded
}

// acts tells whether the player can still bet in the hand.
func (s TableSeat) acts() bool {
	return s.contends() && !s.AllIn
}

// HoldemState is all a table is: the domain as the storage keeps it.
type HoldemState struct {
	Name     string
	OwnerID  uuid.UUID
	Private  bool
	BigBlind int64
	Players  []TableSeat
	Invited  []uuid.UUID
	Street   TableStreet
	Hand     int64
	Button   int
	Turn     int
	Deadline time.Time
	Deck     []Card
	Board    []Card
	Pot      int64
	// CurrentBet is the most put on the street; MinRaise — the least a
	// raise adds to it.
	CurrentBet int64
	MinRaise   int64
	Version    int64
	CreatedAt  time.Time
}

// HoldemTable is a table of "Покер".
type HoldemTable struct {
	id uuid.UUID
	s  HoldemState
}

// TablePayout is the chips of a player who left the table: the votes the
// Treasury pays them.
type TablePayout struct {
	UserID uuid.UUID
	Chips  int64
}

// NewTableName checks the name of a table: one line, up to 40 characters;
// empty — the site names it itself.
func NewTableName(s string) (string, error) {
	s, err := visibleText(s)
	if err != nil || utf8.RuneCountInString(s) > maxTableName || strings.ContainsAny(s, "\r\n") {
		return "", ErrTableName
	}

	return s, nil
}

// NewHoldemTable is a table of ownerID, empty until they sit;
// ErrTableName, ErrTableSeats, ErrTableBlind.
func NewHoldemTable(id, ownerID uuid.UUID, name string, private bool, seats int, bigBlind int64, now time.Time) (*HoldemTable, error) {
	name, err := NewTableName(name)
	if err != nil {
		return nil, err
	}
	if seats < MinTableSeats || seats > MaxTableSeats {
		return nil, ErrTableSeats
	}
	if bigBlind < MinBigBlind || bigBlind > MaxBigBlind {
		return nil, ErrTableBlind
	}

	return &HoldemTable{id: id, s: HoldemState{
		Name: name, OwnerID: ownerID, Private: private, BigBlind: bigBlind,
		Players: make([]TableSeat, seats), Street: StreetWaiting, Button: -1, Turn: -1,
		CreatedAt: now.UTC(),
	}}, nil
}

// RestoreHoldemTable is a table as the storage keeps it.
func RestoreHoldemTable(id uuid.UUID, s HoldemState) *HoldemTable {
	if len(s.Players) < MinTableSeats {
		s.Players = append(s.Players, make([]TableSeat, MinTableSeats-len(s.Players))...)
	}

	return &HoldemTable{id: id, s: s}
}

func (t *HoldemTable) ID() uuid.UUID {
	return t.id
}

// State is the table to keep; its slices are the table's own.
func (t *HoldemTable) State() HoldemState {
	return t.s
}

func (t *HoldemTable) Name() string {
	return t.s.Name
}

func (t *HoldemTable) OwnerID() uuid.UUID {
	return t.s.OwnerID
}

func (t *HoldemTable) Private() bool {
	return t.s.Private
}

func (t *HoldemTable) BigBlind() int64 {
	return t.s.BigBlind
}

func (t *HoldemTable) SmallBlind() int64 {
	return t.s.BigBlind / 2
}

func (t *HoldemTable) MinBuyIn() int64 {
	return t.s.BigBlind * MinBuyInBlinds
}

func (t *HoldemTable) MaxBuyIn() int64 {
	return t.s.BigBlind * MaxBuyInBlinds
}

func (t *HoldemTable) Seats() []TableSeat {
	return t.s.Players
}

func (t *HoldemTable) Street() TableStreet {
	return t.s.Street
}

func (t *HoldemTable) Hand() int64 {
	return t.s.Hand
}

func (t *HoldemTable) Button() int {
	return t.s.Button
}

// Turn is the seat to act, -1 for none.
func (t *HoldemTable) Turn() int {
	return t.s.Turn
}

// Deadline is when the turn passes or the next hand is dealt; zero for
// none.
func (t *HoldemTable) Deadline() time.Time {
	return t.s.Deadline
}

func (t *HoldemTable) Board() []Card {
	return t.s.Board
}

// Pot is all the chips in the middle: those of past streets and the bets
// of this one.
func (t *HoldemTable) Pot() int64 {
	pot := t.s.Pot
	for _, p := range t.s.Players {
		pot += p.Bet
	}

	return pot
}

func (t *HoldemTable) CurrentBet() int64 {
	return t.s.CurrentBet
}

func (t *HoldemTable) Version() int64 {
	return t.s.Version
}

func (t *HoldemTable) Invited() []uuid.UUID {
	return t.s.Invited
}

func (t *HoldemTable) CreatedAt() time.Time {
	return t.s.CreatedAt
}

// Players is how many sit at the table.
func (t *HoldemTable) Players() int {
	n := 0
	for _, p := range t.s.Players {
		if p.Taken() {
			n++
		}
	}

	return n
}

// Empty tells whether nobody sits at the table: it is closed then.
func (t *HoldemTable) Empty() bool {
	return t.Players() == 0
}

// SeatOf is the seat of userID, -1 for none.
func (t *HoldemTable) SeatOf(userID uuid.UUID) int {
	if userID == uuid.Nil {
		return -1
	}

	return slices.IndexFunc(t.s.Players, func(p TableSeat) bool { return p.UserID == userID })
}

// IsInvited tells whether userID may sit at the table: anyone at a public
// one, the owner and the invited at a private one.
func (t *HoldemTable) IsInvited(userID uuid.UUID) bool {
	return !t.s.Private || userID == t.s.OwnerID || slices.Contains(t.s.Invited, userID)
}

// Due is when the table has to move on without anyone looking: the turn
// or the pause passing, or a player away for long enough; zero for never.
func (t *HoldemTable) Due() time.Time {
	due := t.s.Deadline
	for _, p := range t.s.Players {
		if !p.Taken() || !p.Away || p.InHand {
			continue
		}
		if at := p.AwaySince.Add(TableAway); due.IsZero() || at.Before(due) {
			due = at
		}
	}

	return due
}

// Sit puts userID on the seat (-1 — the first free one) with buyIn chips
// they paid; ErrTableSeated, ErrTableNotInvited, ErrTableBuyIn,
// ErrTableFull, ErrTableSeatTaken.
func (t *HoldemTable) Sit(userID uuid.UUID, seat int, buyIn int64, now time.Time) error {
	if t.SeatOf(userID) >= 0 {
		return ErrTableSeated
	}
	if !t.IsInvited(userID) {
		return ErrTableNotInvited
	}
	if buyIn < t.MinBuyIn() || buyIn > t.MaxBuyIn() {
		return ErrTableBuyIn
	}

	if seat < 0 {
		seat = slices.IndexFunc(t.s.Players, func(p TableSeat) bool { return !p.Taken() })
		if seat < 0 {
			return ErrTableFull
		}
	}
	if seat >= len(t.s.Players) {
		return ErrTableSeatTaken
	}
	if t.s.Players[seat].Taken() {
		return ErrTableSeatTaken
	}

	t.s.Players[seat] = TableSeat{UserID: userID, Stack: buyIn}
	t.wake(now)
	t.s.Version++

	return nil
}

// wake sets the next hand to come soon on a table waiting for players.
func (t *HoldemTable) wake(now time.Time) {
	if t.s.Street == StreetWaiting && len(t.eligible()) >= 2 {
		t.s.Deadline = now.UTC().Add(tableStart)
	}
}

// Back brings userID back from sitting out; ErrTableNotSeated.
func (t *HoldemTable) Back(userID uuid.UUID, now time.Time) error {
	i := t.SeatOf(userID)
	if i < 0 {
		return ErrTableNotSeated
	}

	p := &t.s.Players[i]
	p.Away, p.Missed, p.AwaySince = false, 0, time.Time{}
	t.wake(now)
	t.s.Version++

	return nil
}

// Leave stands userID up: in a hand they fold first, what they put stays
// in the pot; the payout is their chips; ErrTableNotSeated.
func (t *HoldemTable) Leave(userID uuid.UUID, now time.Time) (TablePayout, error) {
	i := t.SeatOf(userID)
	if i < 0 {
		return TablePayout{}, ErrTableNotSeated
	}

	payout := t.standUp(i, now.UTC())
	t.s.Version++

	return payout, nil
}

// standUp takes the player of seat i away with their chips; one in a hand
// folds and keeps the seat, empty, until the hand is over.
func (t *HoldemTable) standUp(i int, now time.Time) TablePayout {
	p := &t.s.Players[i]
	payout := TablePayout{UserID: p.UserID, Chips: p.Stack}

	if !p.InHand || !t.s.Street.Betting() {
		t.s.Players[i] = TableSeat{}

		return payout
	}

	p.Stack, p.Leaving, p.Folded, p.Last = 0, true, true, ActFold
	if t.s.Turn == i {
		t.next(now)
	} else if t.alone() {
		t.next(now)
	}

	return payout
}

// Invite lets userIDs sit at a private table and shows it to them;
// only who sits at the table or owns it invites; ErrTableNotSeated,
// ErrTableInvites.
func (t *HoldemTable) Invite(byID uuid.UUID, userIDs []uuid.UUID) error {
	if byID != t.s.OwnerID && t.SeatOf(byID) < 0 {
		return ErrTableNotSeated
	}

	invited := slices.Clone(t.s.Invited)
	for _, id := range userIDs {
		if id != byID && id != uuid.Nil && !slices.Contains(invited, id) {
			invited = append(invited, id)
		}
	}
	if len(invited) > MaxTableInvites {
		return ErrTableInvites
	}

	t.s.Invited = invited
	t.s.Version++

	return nil
}

// eligible are the seats of players who get cards in the next hand.
func (t *HoldemTable) eligible() []int {
	var seats []int
	for i, p := range t.s.Players {
		if p.Taken() && !p.Leaving && !p.Away && p.Stack > 0 {
			seats = append(seats, i)
		}
	}

	return seats
}

// At moves the table on to now: turns past their time are checked or
// folded (two in a row sit the player out), hands are dealt with decks
// of shuffle, and players away too long stand up. The payouts are the
// chips of those stood up; changed tells whether the table moved.
func (t *HoldemTable) At(now time.Time, shuffle func() []Card) (payouts []TablePayout, changed bool) {
	now = now.UTC()

	// A turn passes into the next: a hand of players all away ends in a
	// few hundred steps at most.
	for range 1000 {
		if t.s.Deadline.IsZero() || now.Before(t.s.Deadline) {
			break
		}

		at := t.s.Deadline
		if t.s.Street.Betting() && t.s.Turn >= 0 {
			t.timeout(at)
		} else {
			payouts = append(payouts, t.deal(at, shuffle)...)
		}
		changed = true
	}

	for i, p := range t.s.Players {
		if p.Taken() && p.Away && !p.InHand && !now.Before(p.AwaySince.Add(TableAway)) {
			payouts = append(payouts, t.standUp(i, now))
			changed = true
		}
	}

	if changed {
		t.s.Version++
	}

	return payouts, changed
}

// timeout lets the turn pass: a check when it costs nothing, else a fold.
func (t *HoldemTable) timeout(at time.Time) {
	p := &t.s.Players[t.s.Turn]
	p.Missed++
	if p.Missed >= tableMissed {
		p.Away, p.AwaySince = true, at
	}

	if p.Bet >= t.s.CurrentBet {
		p.Acted, p.Last = true, ActCheck
	} else {
		p.Folded, p.Last = true, ActFold
	}

	t.next(at)
}

// deal starts a hand at now: the seats emptied by the last one are freed
// (their payouts — of the broke, nothing), the button moves, the blinds
// are posted and two cards dealt each.
func (t *HoldemTable) deal(now time.Time, shuffle func() []Card) []TablePayout {
	var payouts []TablePayout
	for i, p := range t.s.Players {
		if p.Taken() && (p.Leaving || p.Stack == 0) {
			if p.Stack > 0 {
				payouts = append(payouts, TablePayout{UserID: p.UserID, Chips: p.Stack})
			}
			t.s.Players[i] = TableSeat{}
		}
	}

	for i := range t.s.Players {
		p := &t.s.Players[i]
		*p = TableSeat{UserID: p.UserID, Stack: p.Stack, Away: p.Away, AwaySince: p.AwaySince, Missed: p.Missed}
	}
	t.s.Board, t.s.Deck, t.s.Pot, t.s.Turn = nil, nil, 0, -1

	seats := t.eligible()
	if len(seats) < 2 {
		t.s.Street, t.s.Deadline = StreetWaiting, time.Time{}

		return payouts
	}

	t.s.Hand++
	t.s.Street = StreetPreflop
	t.s.Deck = shuffle()
	for _, i := range seats {
		t.s.Players[i].InHand = true
	}

	t.s.Button = t.after(t.s.Button, func(p TableSeat) bool { return p.InHand })
	small := t.after(t.s.Button, func(p TableSeat) bool { return p.InHand })
	if len(seats) == 2 {
		small = t.s.Button
	}
	big := t.after(small, func(p TableSeat) bool { return p.InHand })

	t.put(small, t.SmallBlind())
	t.s.Players[small].Last = ActSmallBlind
	t.put(big, t.s.BigBlind)
	t.s.Players[big].Last = ActBigBlind
	t.s.CurrentBet, t.s.MinRaise = t.s.BigBlind, t.s.BigBlind

	for range 2 {
		for i := t.after(t.s.Button, nil); ; i = t.after(i, nil) {
			if t.s.Players[i].InHand {
				t.s.Players[i].Cards = append(t.s.Players[i].Cards, t.draw())
			}
			if i == t.s.Button {
				break
			}
		}
	}

	t.s.Turn = big
	t.next(now)

	return payouts
}

// draw takes the top card of the deck.
func (t *HoldemTable) draw() Card {
	c := t.s.Deck[0]
	t.s.Deck = t.s.Deck[1:]

	return c
}

// after is the first seat after i, going round, that ok takes (nil — any
// taken one); i itself last.
func (t *HoldemTable) after(i int, ok func(TableSeat) bool) int {
	n := len(t.s.Players)
	for k := 1; k <= n; k++ {
		j := ((i+k)%n + n) % n
		if p := t.s.Players[j]; p.Taken() && (ok == nil || ok(p)) {
			return j
		}
	}

	return i
}

// put moves chips of seat i into its bet, all in once the stack is out.
func (t *HoldemTable) put(i int, chips int64) {
	p := &t.s.Players[i]
	chips = min(chips, p.Stack)
	p.Stack -= chips
	p.Bet += chips
	p.Put += chips
	if p.Stack == 0 {
		p.AllIn = true
	}
	t.s.CurrentBet = max(t.s.CurrentBet, p.Bet)
}

// Act is the move of userID on their turn; amount is what a raise takes
// the bet to. ErrTableNotSeated, ErrTableNotYourTurn, ErrTableAction,
// ErrTableRaise.
func (t *HoldemTable) Act(userID uuid.UUID, action TableAction, amount int64, now time.Time) error {
	i := t.SeatOf(userID)
	if i < 0 {
		return ErrTableNotSeated
	}
	if !t.s.Street.Betting() || t.s.Turn != i {
		return ErrTableNotYourTurn
	}

	p := &t.s.Players[i]
	toCall := t.s.CurrentBet - p.Bet

	switch action {
	case ActFold:
		p.Folded = true
	case ActCheck:
		if toCall > 0 {
			return ErrTableAction
		}
	case ActCall:
		if toCall <= 0 {
			return ErrTableAction
		}
		t.put(i, toCall)
	case ActAllIn:
		if p.Bet+p.Stack <= t.s.CurrentBet {
			t.put(i, p.Stack)
			break
		}
		t.raise(i, p.Bet+p.Stack)
	case ActRaise:
		least, most := t.RaiseLimits(i)
		if most == 0 || amount < least || amount > most {
			return ErrTableRaise
		}
		t.raise(i, amount)
	default:
		return ErrTableAction
	}

	p.Acted, p.Missed, p.Last = true, 0, action
	t.next(now.UTC())
	t.s.Version++

	return nil
}

// raise takes the bet of seat i to to; a full raise opens the round to
// the others again.
func (t *HoldemTable) raise(i int, to int64) {
	by := to - t.s.CurrentBet
	t.put(i, to-t.s.Players[i].Bet)

	if by < t.s.MinRaise {
		return
	}

	t.s.MinRaise = by
	for j := range t.s.Players {
		if j != i {
			t.s.Players[j].Acted = false
		}
	}
}

// RaiseLimits are the least and the most seat i can raise (or bet) to,
// zeros when it cannot: the stack is too short to more than call.
func (t *HoldemTable) RaiseLimits(i int) (least, most int64) {
	if i < 0 || i >= len(t.s.Players) {
		return 0, 0
	}

	p := t.s.Players[i]
	most = p.Bet + p.Stack
	if most <= t.s.CurrentBet || t.othersActing(i) == 0 {
		return 0, 0
	}

	least = min(t.s.CurrentBet+max(t.s.MinRaise, t.s.BigBlind), most)

	return least, most
}

// othersActing are the players but i who can still bet.
func (t *HoldemTable) othersActing(i int) int {
	n := 0
	for j, p := range t.s.Players {
		if j != i && p.acts() {
			n++
		}
	}

	return n
}

// ToCall is what seat i has to put to call.
func (t *HoldemTable) ToCall(i int) int64 {
	if i < 0 || i >= len(t.s.Players) {
		return 0
	}

	p := t.s.Players[i]

	return min(max(t.s.CurrentBet-p.Bet, 0), p.Stack)
}

// owes tells whether seat i has to act before the round is over: it can
// bet and has to call, or has not acted while someone can still answer.
func (t *HoldemTable) owes(i int) bool {
	p := t.s.Players[i]
	if !p.acts() {
		return false
	}

	return p.Bet < t.s.CurrentBet || (!p.Acted && t.othersActing(i) > 0)
}

// alone tells whether one player is left in the hand.
func (t *HoldemTable) alone() bool {
	n := 0
	for _, p := range t.s.Players {
		if p.contends() {
			n++
		}
	}

	return n <= 1
}

// next passes the turn, ends the round or the hand at now.
func (t *HoldemTable) next(now time.Time) {
	if t.alone() {
		t.award(now)

		return
	}

	for j := t.after(t.s.Turn, nil); ; j = t.after(j, nil) {
		if t.owes(j) {
			t.s.Turn, t.s.Deadline = j, now.Add(TableTurn)

			return
		}
		if j == t.s.Turn {
			break
		}
	}

	t.endRound(now)
}

// endRound gathers the bets and lays the next street; without two players
// to bet the board is run out to the showdown.
func (t *HoldemTable) endRound(now time.Time) {
	for t.s.Street.Betting() {
		for i := range t.s.Players {
			p := &t.s.Players[i]
			t.s.Pot += p.Bet
			p.Bet = 0
			if p.acts() {
				p.Acted = false
				if p.Last != ActFold {
					p.Last = ""
				}
			}
		}
		t.s.CurrentBet, t.s.MinRaise = 0, t.s.BigBlind

		switch t.s.Street {
		case StreetPreflop:
			t.s.Street = StreetFlop
			t.s.Board = append(t.s.Board, t.draw(), t.draw(), t.draw())
		case StreetFlop:
			t.s.Street = StreetTurn
			t.s.Board = append(t.s.Board, t.draw())
		case StreetTurn:
			t.s.Street = StreetRiver
			t.s.Board = append(t.s.Board, t.draw())
		default:
			t.showdown(now)

			return
		}

		acting := 0
		for _, p := range t.s.Players {
			if p.acts() {
				acting++
			}
		}
		if acting >= 2 {
			t.s.Turn = t.s.Button
			for j := t.after(t.s.Button, nil); ; j = t.after(j, nil) {
				if t.owes(j) {
					t.s.Turn, t.s.Deadline = j, now.Add(TableTurn)

					return
				}
				if j == t.s.Button {
					break
				}
			}
		}
	}
}

// award gives all the chips to the one player left: nobody shows.
func (t *HoldemTable) award(now time.Time) {
	pot := t.Pot()
	for i := range t.s.Players {
		p := &t.s.Players[i]
		p.Bet = 0
		if p.contends() {
			p.Stack += pot
			p.Won = pot
		}
	}
	t.over(now)
}

// over ends the hand: the result stays on the table for the pause.
func (t *HoldemTable) over(now time.Time) {
	t.s.Pot, t.s.CurrentBet, t.s.Turn = 0, 0, -1
	t.s.Street, t.s.Deadline = StreetShowdown, now.Add(TablePause)
}

// showdown opens the hands and pays the pots: the main one and a side pot
// for every all-in level, each to the best hands that played for it, odd
// chips to the first winner after the button.
func (t *HoldemTable) showdown(now time.Time) {
	scores := make(map[int]int)
	var levels []int64
	for i, p := range t.s.Players {
		if p.contends() {
			p.Shown = true
			t.s.Players[i] = p
			scores[i] = BestHand(append(slices.Clone(p.Cards), t.s.Board...)).Score
			if !slices.Contains(levels, p.Put) {
				levels = append(levels, p.Put)
			}
		}
	}
	slices.Sort(levels)

	var prev int64
	for _, level := range levels {
		var pot int64
		for _, p := range t.s.Players {
			pot += min(p.Put, level) - min(p.Put, prev)
		}
		prev = level

		var winners []int
		best := -1
		for j := t.after(t.s.Button, nil); ; j = t.after(j, nil) {
			if p := t.s.Players[j]; p.contends() && p.Put >= level {
				switch s := scores[j]; {
				case s > best:
					best, winners = s, []int{j}
				case s == best:
					winners = append(winners, j)
				}
			}
			if j == t.s.Button {
				break
			}
		}
		t.pay(winners, pot)
	}

	// What folded players put above every level of those who showed goes
	// to the one who put the most.
	var rest int64
	for _, p := range t.s.Players {
		rest += max(p.Put-prev, 0)
	}
	if rest > 0 {
		top := -1
		for i, p := range t.s.Players {
			if p.contends() && (top < 0 || p.Put > t.s.Players[top].Put) {
				top = i
			}
		}
		t.pay([]int{top}, rest)
	}

	t.over(now)
}

// pay splits chips among winners in their order, the odd ones to the
// first.
func (t *HoldemTable) pay(winners []int, chips int64) {
	if len(winners) == 0 || chips == 0 {
		return
	}

	share := chips / int64(len(winners))
	odd := chips % int64(len(winners))
	for k, i := range winners {
		won := share
		if int64(k) < odd {
			won++
		}
		t.s.Players[i].Stack += won
		t.s.Players[i].Won += won
	}
}

// HandOf is the best hand seat i shows of its cards and the board; zero
// without cards.
func (t *HoldemTable) HandOf(i int) (Hand, bool) {
	p := t.s.Players[i]
	if len(p.Cards) < 2 || len(t.s.Board) < 3 {
		return Hand{}, false
	}

	return BestHand(append(slices.Clone(p.Cards), t.s.Board...)), true
}
