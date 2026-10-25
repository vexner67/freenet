package user

import (
	"errors"
	"math"
	"math/rand/v2"
	"time"
)

// EventKind is something that happens to a business and waits for the
// owner: until Until, then the default answer applies.
type EventKind string

const (
	// EventInspection is an inspector: pay the fine, or a bribe of half
	// of it that may be caught.
	EventInspection EventKind = "inspection"
	// EventBreakdown is the equipment of an offer broken: it serves half
	// until mended.
	EventBreakdown EventKind = "breakdown"
	// EventBigOrder is a customer who buys many sales at once, dearer.
	EventBigOrder EventKind = "bigorder"
	// EventRaise is a worker asking for a raise.
	EventRaise EventKind = "raise"
)

// The answers to the events.
const (
	OptionFine    = "fine"
	OptionBribe   = "bribe"
	OptionRepair  = "repair"
	OptionLeave   = "leave"
	OptionAccept  = "accept"
	OptionDecline = "decline"
)

// The odds of the events: one an eventHours hours or so, the owner has a
// day to answer, a bribe is caught in bribeCaught percents.
const (
	eventHours   = 50
	eventWait    = 24 * time.Hour
	bribeCaught  = 30
	reviewHours  = 24
	reviewBoost  = 1.3
	bigOrderRate = 1.3
)

var (
	ErrNoEvent       = errors.New("nothing waits for an answer")
	ErrUnknownOption = errors.New("unknown answer")
	ErrEventStock    = errors.New("there are not enough supplies for the order")
)

// Event is what waits for the owner: the offer of a breakdown or a big
// order, the worker asking for a raise, the votes of a fine or a repair,
// the units of a big order and what it pays in hundredths of a vote.
type Event struct {
	Kind   EventKind
	Until  time.Time
	Offer  Offer
	Worker int
	Name   string
	Female bool
	Votes  int64
	Units  int64
	Pay    int64
}

// Options are the answers to e, the default first.
func (e Event) Options() []string {
	switch e.Kind {
	case EventInspection:
		return []string{OptionFine, OptionBribe}
	case EventBreakdown:
		return []string{OptionLeave, OptionRepair}
	case EventBigOrder, EventRaise:
		return []string{OptionDecline, OptionAccept}
	}

	return nil
}

// fine is the fine of an inspection: a twentieth of the price of the
// business, half with an accountant.
func (b Business) fine(p BusinessPlan) int64 {
	fine := max(p.Price/20, 1)
	if b.skillOf(ProfAccountant) > 0 {
		fine = max(fine/2, 1)
	}

	return fine
}

func breakdownCost(p BusinessPlan) int64 {
	return max(p.Price/40, 1)
}

// happen is whatever happens to b in the hours of a step at t: an event
// an eventHours hours or so, none while one waits.
func (b *Business) happen(spec KindSpec, p BusinessPlan, t time.Time, hours float64, hour *BusinessHour) {
	rng := rand.New(rand.NewPCG(b.seed^0xA5A5A5A5A5A5A5A5, uint64(t.UnixNano()))) //nolint:gosec // the game, not secrets
	if b.event.Kind != "" || rng.Float64() >= hours/eventHours {
		return
	}

	offer := spec.Offers[rng.IntN(len(spec.Offers))]
	until := t.Add(eventWait)

	switch rng.IntN(6) {
	case 0:
		b.event = Event{Kind: EventInspection, Until: until, Votes: b.fine(p)}
	case 1:
		if b.broken != "" {
			return
		}

		b.broken = offer.Offer
		if skill := b.skillOf(ProfTechnician); skill > 0 {
			b.brokenUntil = t.Add(time.Duration(12 / float64(skill) * float64(time.Hour)))
			b.note(Note{At: t, Kind: NoteBreakdown, Subject: string(offer.Offer)})

			return
		}

		b.event = Event{Kind: EventBreakdown, Until: until, Offer: offer.Offer, Votes: breakdownCost(p)}
	case 2:
		units := int64(math.Ceil(offer.Demand * float64(b.level) * 4))
		b.event = Event{
			Kind: EventBigOrder, Until: until, Offer: offer.Offer, Units: units,
			Pay: int64(math.Round(float64(units) * float64(offer.Fair) * bigOrderRate)),
		}
	case 3:
		if len(b.workers) == 0 {
			return
		}

		w := b.workers[rng.IntN(len(b.workers))]
		b.event = Event{Kind: EventRaise, Until: until, Worker: w.ID, Name: w.Name, Female: w.Female}
	case 4:
		if b.skillOf(ProfGuard) > 0 || b.cash <= 0 {
			return
		}

		stolen := b.cash * float64(10+rng.IntN(21)) / 100
		b.cash -= stolen
		hour.Losses += stolen
		b.note(Note{At: t, Kind: NoteTheft, Money: stolen})
	default:
		b.reviewUntil = t.Add(reviewHours * time.Hour)
		b.note(Note{At: t, Kind: NoteReview})
	}
}

// Decided is b with its event answered at now.
func (b Business) Decided(spec KindSpec, option string, now time.Time) (Business, error) {
	if err := b.checkOpen(); err != nil {
		return Business{}, err
	}
	if b.event.Kind == "" {
		return Business{}, ErrNoEvent
	}

	b = b.clone()
	if err := b.answer(spec, option, now.UTC(), nil); err != nil {
		return Business{}, err
	}

	return b, nil
}

// answer answers the event at at: the money comes out of and into the
// till.
func (b *Business) answer(spec KindSpec, option string, at time.Time, hour *BusinessHour) error {
	e := b.event
	known := false
	for _, o := range e.Options() {
		known = known || o == option
	}
	if !known {
		return ErrUnknownOption
	}

	var losses float64
	switch {
	case e.Kind == EventInspection && option == OptionFine:
		losses = float64(e.Votes) * 100
		b.note(Note{At: at, Kind: NoteFine, Money: losses})
	case e.Kind == EventInspection:
		losses = float64(e.Votes) * 50
		rng := rand.New(rand.NewPCG(b.seed, uint64(e.Until.UnixNano()))) //nolint:gosec // the game, not secrets
		if rng.IntN(100) < bribeCaught {
			losses += float64(e.Votes) * 200
			b.note(Note{At: at, Kind: NoteCaught, Money: losses})
		} else {
			b.note(Note{At: at, Kind: NoteBribe, Money: losses})
		}
	case e.Kind == EventBreakdown && option == OptionRepair:
		losses = float64(e.Votes) * 100
		b.broken, b.brokenUntil = "", time.Time{}
		b.note(Note{At: at, Kind: NoteRepaired, Subject: string(e.Offer), Money: losses})
	case e.Kind == EventBigOrder && option == OptionAccept:
		if err := b.fulfil(spec, e); err != nil {
			return err
		}
		b.note(Note{At: at, Kind: NoteBigOrder, Subject: string(e.Offer), Amount: float64(e.Units), Money: float64(e.Pay)})
	case e.Kind == EventRaise:
		b.raiseAnswered(e, option == OptionAccept, at)
	}

	b.cash -= losses
	if hour != nil {
		hour.Losses += losses
	}
	b.event = Event{}

	return nil
}

// fulfil sells the big order: its supplies come out of the storage, its
// pay into the till.
func (b *Business) fulfil(spec KindSpec, e Event) error {
	offer, err := spec.Offer(e.Offer)
	if err != nil {
		return err
	}

	for _, in := range offer.Recipe {
		if b.stock[in.Supply] < float64(in.Units*e.Units) {
			return ErrEventStock
		}
	}
	for _, in := range offer.Recipe {
		b.stock[in.Supply] -= float64(in.Units * e.Units)
	}

	b.cash += float64(e.Pay)

	return nil
}

func (b *Business) raiseAnswered(e Event, accepted bool, at time.Time) {
	i, err := b.worker(e.Worker)
	if err != nil {
		return
	}

	w := &b.workers[i]
	if accepted {
		w.Wage = min(w.Wage+(w.Wage+4)/5, MaxBusinessPrice)
		w.Mood = min(w.Mood+20, 100)
	} else {
		w.Mood = max(w.Mood-30, 0)
	}

	b.note(Note{At: at, Kind: NoteRaise, Name: w.Name, Female: w.Female, Amount: boolAmount(accepted)})
}

func boolAmount(ok bool) float64 {
	if ok {
		return 1
	}

	return 0
}

// expire gives the event the default answer when its day is over by at.
func (b *Business) expire(spec KindSpec, at time.Time, hour *BusinessHour) {
	if b.event.Kind == "" || b.event.Until.After(at) {
		return
	}

	_ = b.answer(spec, b.event.Options()[0], b.event.Until, hour) // the default answers are always known
}
