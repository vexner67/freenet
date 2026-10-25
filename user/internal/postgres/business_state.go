package postgres

import (
	"time"

	"github.com/vexner67/freenet/user/internal/user"
)

// storedBusiness is a business as the state column keeps it. The parts
// are the domain structs with JSON names: a struct converts to another
// with the same fields whatever their tags.
type storedBusiness struct {
	Kind        user.BusinessKind       `json:"kind"`
	Seed        uint64                  `json:"seed"`
	Level       int                     `json:"level"`
	Condition   float64                 `json:"condition"`
	Cash        float64                 `json:"cash"`
	AdUntil     time.Time               `json:"adUntil"`
	ReviewUntil time.Time               `json:"reviewUntil"`
	CountedAt   time.Time               `json:"countedAt"`
	Bankrupt    bool                    `json:"bankrupt"`
	Invested    int64                   `json:"invested"`
	Collected   int64                   `json:"collected"`
	Stock       map[user.Supply]float64 `json:"stock"`
	Prices      map[user.Offer]int64    `json:"prices"`
	Workers     []storedWorker          `json:"workers"`
	NextID      int                     `json:"nextId"`
	Window      int64                   `json:"window"`
	Hired       []int                   `json:"hired"`
	Orders      []storedOrder           `json:"orders"`
	Contracts   []storedContract        `json:"contracts"`
	Event       storedEvent             `json:"event"`
	Broken      user.Offer              `json:"broken"`
	BrokenUntil time.Time               `json:"brokenUntil"`
	Hours       []storedHour            `json:"hours"`
	Notes       []storedNote            `json:"notes"`
}

type storedWorker struct {
	ID         int             `json:"id"`
	Name       string          `json:"name"`
	Female     bool            `json:"female"`
	Profession user.Profession `json:"profession"`
	Skill      int             `json:"skill"`
	XP         float64         `json:"xp"`
	Wage       int64           `json:"wage"`
	Trait      user.Trait      `json:"trait"`
	Mood       float64         `json:"mood"`
}

type storedOrder struct {
	ID       int           `json:"id"`
	Supply   user.Supply   `json:"supply"`
	Supplier user.Supplier `json:"supplier"`
	Units    int64         `json:"units"`
	Arrives  time.Time     `json:"arrives"`
	FromTill bool          `json:"fromTill"`
}

type storedContract struct {
	Supply   user.Supply   `json:"supply"`
	Supplier user.Supplier `json:"supplier"`
	Below    int64         `json:"below"`
	Units    int64         `json:"units"`
	Stalled  bool          `json:"stalled"`
}

type storedEvent struct {
	Kind   user.EventKind `json:"kind"`
	Until  time.Time      `json:"until"`
	Offer  user.Offer     `json:"offer"`
	Worker int            `json:"worker"`
	Name   string         `json:"name"`
	Female bool           `json:"female"`
	Votes  int64          `json:"votes"`
	Units  int64          `json:"units"`
	Pay    int64          `json:"pay"`
}

type storedHour struct {
	Hour     time.Time              `json:"hour"`
	Sold     map[user.Offer]float64 `json:"sold"`
	Income   float64                `json:"income"`
	Wages    float64                `json:"wages"`
	Rent     float64                `json:"rent"`
	Supplies float64                `json:"supplies"`
	Losses   float64                `json:"losses"`
	Spoiled  float64                `json:"spoiled"`
	Unserved float64                `json:"unserved"`
}

type storedNote struct {
	At      time.Time     `json:"at"`
	Kind    user.NoteKind `json:"kind"`
	Subject string        `json:"subject"`
	Name    string        `json:"name"`
	Female  bool          `json:"female"`
	Amount  float64       `json:"amount"`
	Money   float64       `json:"money"`
}

func storeBusiness(s user.BusinessState) storedBusiness {
	return storedBusiness{
		Kind: s.Kind, Seed: s.Seed, Level: s.Level, Condition: s.Condition, Cash: s.Cash,
		AdUntil: s.AdUntil, ReviewUntil: s.ReviewUntil, CountedAt: s.CountedAt, Bankrupt: s.Bankrupt,
		Invested: s.Invested, Collected: s.Collected, Stock: s.Stock, Prices: s.Prices,
		Workers: convert(s.Workers, func(w user.Worker) storedWorker { return storedWorker(w) }),
		NextID:  s.NextID, Window: s.Window, Hired: s.Hired,
		Orders:    convert(s.Orders, func(o user.Order) storedOrder { return storedOrder(o) }),
		Contracts: convert(s.Contracts, func(c user.Contract) storedContract { return storedContract(c) }),
		Event:     storedEvent(s.Event), Broken: s.Broken, BrokenUntil: s.BrokenUntil,
		Hours: convert(s.Hours, func(h user.BusinessHour) storedHour { return storedHour(h) }),
		Notes: convert(s.Notes, func(n user.Note) storedNote { return storedNote(n) }),
	}
}

func (s storedBusiness) restore() user.BusinessState {
	return user.BusinessState{
		Kind: s.Kind, Seed: s.Seed, Level: s.Level, Condition: s.Condition, Cash: s.Cash,
		AdUntil: s.AdUntil, ReviewUntil: s.ReviewUntil, CountedAt: s.CountedAt, Bankrupt: s.Bankrupt,
		Invested: s.Invested, Collected: s.Collected, Stock: s.Stock, Prices: s.Prices,
		Workers: convert(s.Workers, func(w storedWorker) user.Worker { return user.Worker(w) }),
		NextID:  s.NextID, Window: s.Window, Hired: s.Hired,
		Orders:    convert(s.Orders, func(o storedOrder) user.Order { return user.Order(o) }),
		Contracts: convert(s.Contracts, func(c storedContract) user.Contract { return user.Contract(c) }),
		Event:     user.Event(s.Event), Broken: s.Broken, BrokenUntil: s.BrokenUntil,
		Hours: convert(s.Hours, func(h storedHour) user.BusinessHour { return user.BusinessHour(h) }),
		Notes: convert(s.Notes, func(n storedNote) user.Note { return user.Note(n) }),
	}
}

func convert[T, U any](items []T, to func(T) U) []U {
	converted := make([]U, len(items))
	for i, item := range items {
		converted[i] = to(item)
	}

	return converted
}
