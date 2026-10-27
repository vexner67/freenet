package user

import (
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	// MaxFirmShares is the most shares the administrators may let an
	// enterprise have: a payout times it stays within int64.
	MaxFirmShares = 1_000_000
	// MaxFirmsDirected is the most enterprises the administrators may let
	// a page direct.
	MaxFirmsDirected = 1_000
	// FirmHours are the hours an enterprise reports.
	FirmHours = 24
	// maxFirmNameLen is the longest name of an enterprise.
	maxFirmNameLen = 40
	// maxFirmUnits is the most units an order or a deal moves.
	maxFirmUnits = 1_000_000_000
)

var (
	ErrFirmName      = errors.New("the name of an enterprise must be 1 to 40 characters in one line")
	ErrFirmNotFound  = errors.New("there is no such enterprise")
	ErrNotDirector   = errors.New("only the director runs the enterprise")
	ErrTooManyFirms  = errors.New("you direct too many enterprises")
	ErrFirmTopLevel  = errors.New("the enterprise is at its top level")
	ErrFirmCash      = errors.New("the enterprise has not enough money")
	ErrFirmStaff     = errors.New("the workers sought must be from 0 to the seats of the level, the wage from half to three times the standard")
	ErrFirmGood      = errors.New("the enterprise does not deal in this good")
	ErrFirmStock     = errors.New("there is not so much of the good in the storage")
	ErrFirmStorage   = errors.New("the storage will not hold it")
	ErrFirmUnits     = errors.New("the units must be from 1")
	ErrFirmVotes     = errors.New("the votes must be from 1")
	ErrFirmAuto      = errors.New("the limit of a price must be from 0")
	ErrFirmNotYet    = errors.New("enterprises pay pages a week old with a filled profile")
	ErrFirmNoDeal    = errors.New("the state does not deal in so few units now")
	ErrFirmShortCash = errors.New("there is not a whole vote to share")
	ErrFirmWorn      = errors.New("the enterprise needs no repair")
	ErrImprovement   = errors.New("unknown improvement or it is at its top rank")
)

// FirmName is the name an owner gives an enterprise.
type FirmName string

func NewFirmName(s string) (FirmName, error) {
	s = strings.TrimSpace(s)
	if s == "" || utf8.RuneCountInString(s) > maxFirmNameLen {
		return "", ErrFirmName
	}

	for _, r := range s {
		if unicode.IsControl(r) {
			return "", ErrFirmName
		}
	}

	return FirmName(s), nil
}

func (n FirmName) String() string {
	return string(n)
}

// Auto is what an enterprise does with a good every hour by itself: buys
// from the state as much as an hour of work takes, paying at most BuyMax
// hundredths a unit, and sells what it has — to the state or, for a shop,
// to the city — taking at least SellMin.
type Auto struct {
	Buy     bool
	BuyMax  int64
	Sell    bool
	SellMin int64
}

// FirmHour is an hour of an enterprise: the units it made, bought and
// sold, and its money in hundredths — what it got for sales, paid for
// goods, in wages, in upkeep and in fees of the exchange; the workers it
// hired and those who left it for better pay.
type FirmHour struct {
	Hour      time.Time
	Made      int64
	Bought    int64
	Sold      int64
	Income    int64
	Purchases int64
	Wages     int64
	Upkeep    int64
	Fees      int64
	Hired     int64
	Left      int64
}

// Profit is what the hour earned.
func (h FirmHour) Profit() int64 {
	return h.Income - h.Purchases - h.Wages - h.Upkeep - h.Fees
}

// Firm is an enterprise: a company of shareholders run by its director.
// It seeks wanted workers and has those the people of the city gave it.
// Its cash is in hundredths of a vote and never below zero; the units of
// goods in its storage are whole, and so is what is made of them, the
// fractions of an hour carried to the next. Its machines wear out as it
// works (wear, from 0 — new — to 100), its improvements have ranks, and
// the exchange keeps its mood, the price of a share in hundredths, the
// new shares it sells (float) and an hour of the price after another.
type Firm struct {
	id         uuid.UUID
	kind       FirmKind
	name       FirmName
	directorID uuid.UUID
	foundedAt  time.Time
	level      int
	workers    int
	wanted     int
	wage       int64
	cash       int64
	stock      map[Good]int64
	carry      map[Good]float64
	auto       map[Good]Auto
	hours      []FirmHour
	wear       float64
	ranks      map[Improvement]int
	improved   int64
	idleUntil  time.Time
	mood       float64
	price      int64
	float      int64
	candles    []Candle
}

// FirmState is an enterprise as kept.
type FirmState struct {
	ID         uuid.UUID
	Kind       FirmKind
	Name       FirmName
	DirectorID uuid.UUID
	FoundedAt  time.Time
	Level      int
	Workers    int
	Wanted     int
	Wage       int64
	Cash       int64
	Stock      map[Good]int64
	Carry      map[Good]float64
	Auto       map[Good]Auto
	Hours      []FirmHour
	Wear       float64
	Ranks      map[Improvement]int
	Improved   int64
	IdleUntil  time.Time
	Mood       float64
	Price      int64
	Float      int64
	Candles    []Candle
}

func restoreFirm(s FirmState) *Firm {
	f := &Firm{
		id: s.ID, kind: s.Kind, name: s.Name, directorID: s.DirectorID, foundedAt: s.FoundedAt.UTC(),
		level: max(s.Level, 1), workers: max(s.Workers, 0), wanted: max(s.Wanted, 0), wage: s.Wage, cash: max(s.Cash, 0),
		stock: map[Good]int64{}, carry: map[Good]float64{}, auto: map[Good]Auto{},
		hours: append([]FirmHour(nil), s.Hours...), wear: min(max(s.Wear, 0), 100), ranks: map[Improvement]int{},
		improved: max(s.Improved, 0), idleUntil: s.IdleUntil.UTC(), mood: min(max(s.Mood, -maxMood), maxMood),
		price: max(s.Price, 0), float: max(s.Float, 0), candles: append([]Candle(nil), s.Candles...),
	}
	// Kept before the labour market, an enterprise sought the workers it
	// had.
	f.wanted = max(f.wanted, f.workers)
	for _, imp := range Improvements {
		if r := s.Ranks[imp]; r > 0 {
			f.ranks[imp] = min(r, MaxRank)
		}
	}

	spec, _ := FirmCatalog(s.Kind) // a kept enterprise is of a known kind
	for _, g := range spec.Uses() {
		if s.Stock[g] > 0 {
			f.stock[g] = s.Stock[g]
		}
		if s.Carry[g] > 0 {
			f.carry[g] = s.Carry[g]
		}
		if a, ok := s.Auto[g]; ok {
			f.auto[g] = a
		}
	}

	return f
}

func (f *Firm) state() FirmState {
	s := FirmState{
		ID: f.id, Kind: f.kind, Name: f.name, DirectorID: f.directorID, FoundedAt: f.foundedAt,
		Level: f.level, Workers: f.workers, Wanted: f.wanted, Wage: f.wage, Cash: f.cash,
		Stock: map[Good]int64{}, Carry: map[Good]float64{}, Auto: map[Good]Auto{},
		Hours: append([]FirmHour(nil), f.hours...), Wear: f.wear, Ranks: map[Improvement]int{}, Improved: f.improved,
		IdleUntil: f.idleUntil, Mood: f.mood, Price: f.price, Float: f.float, Candles: append([]Candle(nil), f.candles...),
	}
	for imp, r := range f.ranks {
		s.Ranks[imp] = r
	}
	// What is empty is left out, as restoreFirm leaves it: a state kept
	// and restored is the same.
	for g, u := range f.stock {
		if u > 0 {
			s.Stock[g] = u
		}
	}
	for g, c := range f.carry {
		if c > 0 {
			s.Carry[g] = c
		}
	}
	for g, a := range f.auto {
		s.Auto[g] = a
	}

	return s
}

func (f *Firm) ID() uuid.UUID         { return f.id }
func (f *Firm) Kind() FirmKind        { return f.kind }
func (f *Firm) Name() FirmName        { return f.name }
func (f *Firm) DirectorID() uuid.UUID { return f.directorID }
func (f *Firm) FoundedAt() time.Time  { return f.foundedAt }
func (f *Firm) Level() int            { return f.level }
func (f *Firm) Workers() int          { return f.workers }

// Wanted is how many workers the enterprise seeks: those it has and the
// vacancies.
func (f *Firm) Wanted() int { return f.wanted }
func (f *Firm) Wage() int64 { return f.wage }

// Cash is the money of the enterprise in hundredths of a vote, without
// what its buying orders keep.
func (f *Firm) Cash() int64 { return f.cash }

// Wear is how worn the machines are: from 0, new, to 100.
func (f *Firm) Wear() float64 { return f.wear }

// Rank is the rank of the improvement, from 0 to MaxRank.
func (f *Firm) Rank(imp Improvement) int { return f.ranks[imp] }

// IdleUntil is when the strike ends; zero or past — the workers work.
func (f *Firm) IdleUntil() time.Time { return f.idleUntil }

// Price is what a share goes for now, in hundredths of a vote: the last
// deal of the hour or, between them, the price the exchange quoted.
func (f *Firm) Price() int64 { return f.price }

// Float is the new shares the enterprise sells through the exchange.
func (f *Firm) Float() int64 { return f.float }

// Candles are the hours of the price, the oldest first.
func (f *Firm) Candles() []Candle {
	return append([]Candle(nil), f.candles...)
}

func (f *Firm) Stock(g Good) int64 { return f.stock[g] }

func (f *Firm) Auto(g Good) Auto { return f.auto[g] }

// Hours are the hours reported, the oldest first; the last may be the
// hour going on, with the deals of the exchange so far.
func (f *Firm) Hours() []FirmHour {
	return append([]FirmHour(nil), f.hours...)
}

// Profit is what the enterprise earned in the last FirmHours hours.
func (f *Firm) Profit() int64 {
	var sum int64
	for _, h := range f.hours {
		sum += h.Profit()
	}

	return sum
}

func (f *Firm) spec() FirmSpec {
	spec, _ := FirmCatalog(f.kind) // a firm is of a known kind

	return spec
}

// hour is the report of the hour at, kept for FirmHours hours.
func (f *Firm) hour(at time.Time) *FirmHour {
	at = at.UTC().Truncate(time.Hour)
	for i := range f.hours {
		if f.hours[i].Hour.Equal(at) {
			return &f.hours[i]
		}
	}

	f.hours = append(f.hours, FirmHour{Hour: at})
	if over := len(f.hours) - FirmHours; over > 0 {
		f.hours = append([]FirmHour(nil), f.hours[over:]...)
	}

	return &f.hours[len(f.hours)-1]
}

// defaultAuto is what a new enterprise does by itself: buys what it works
// with at up to the price of the state with a fifth on top and sells what
// it makes for any price; a shop buys and sells about what the state asks
// with a tenth on top, so as not to sell at a loss.
func defaultAuto(spec FirmSpec, rules FirmRules) map[Good]Auto {
	auto := map[Good]Auto{}
	for _, in := range spec.Inputs {
		p, _ := rules.Good(in.Good)
		auto[in.Good] = Auto{Buy: true, BuyMax: p.Base * int64(rules.Ceiling) * 6 / 500}
	}
	if spec.Output != "" {
		auto[spec.Output] = Auto{Sell: true}
	}
	for _, g := range spec.Sells {
		p, _ := rules.Good(g)
		margin := p.Base * int64(rules.Ceiling) * 11 / 1000
		auto[g] = Auto{Buy: true, BuyMax: margin, Sell: true, SellMin: margin}
	}

	return auto
}

// CheckFirmsAt is whether the page may take votes out of the enterprises —
// dividends, a closed enterprise, shares sold — at now: pages a week old
// with a filled profile, as the daily bonus; ErrFirmNotYet.
func (u *User) CheckFirmsAt(now time.Time) error {
	if err := u.CheckBonusAt(now); err != nil {
		return ErrFirmNotYet
	}

	return nil
}

// FirmAction is what a page does with the enterprises.
type FirmAction string

const (
	FirmFound       FirmAction = "found"
	FirmRename      FirmAction = "rename"
	FirmUpgrade     FirmAction = "upgrade"
	FirmStaff       FirmAction = "staff"
	FirmDeposit     FirmAction = "deposit"
	FirmDividend    FirmAction = "dividend"
	FirmLiquidate   FirmAction = "liquidate"
	FirmAutomate    FirmAction = "auto"
	FirmOrder       FirmAction = "order"
	FirmCancel      FirmAction = "cancel"
	FirmStateBuy    FirmAction = "state_buy"
	FirmStateSell   FirmAction = "state_sell"
	FirmIssue       FirmAction = "issue"
	FirmRepair      FirmAction = "repair"
	FirmImprove     FirmAction = "improve"
	FirmTake        FirmAction = "take_contract"
	FirmDeliver     FirmAction = "deliver"
	FirmFund        FirmAction = "account_deposit"
	FirmWithdraw    FirmAction = "account_withdraw"
	FirmStockOrder  FirmAction = "stock_order"
	FirmStockCancel FirmAction = "stock_cancel"
	FirmTender      FirmAction = "tender"
	FirmTenderSell  FirmAction = "tender_sell"
	FirmTenderClose FirmAction = "tender_cancel"
)

var firmActions = []FirmAction{
	FirmFound, FirmRename, FirmUpgrade, FirmStaff, FirmDeposit, FirmDividend, FirmLiquidate, FirmAutomate,
	FirmOrder, FirmCancel, FirmStateBuy, FirmStateSell, FirmIssue, FirmRepair, FirmImprove, FirmTake, FirmDeliver,
	FirmFund, FirmWithdraw, FirmStockOrder, FirmStockCancel, FirmTender, FirmTenderSell, FirmTenderClose,
}

var ErrUnknownFirmAction = errors.New("unknown action with an enterprise")

func NewFirmAction(s string) (FirmAction, error) {
	for _, a := range firmActions {
		if string(a) == s {
			return a, nil
		}
	}

	return "", ErrUnknownFirmAction
}

// PaysOut is whether the action takes votes out of the enterprises.
func (a FirmAction) PaysOut() bool {
	return a == FirmDividend || a == FirmLiquidate || a == FirmWithdraw
}
