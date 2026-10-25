package user

import (
	"math"
	"slices"
	"time"
)

// NoteKind is what a note of a business tells.
type NoteKind string

const (
	NoteDelivery  NoteKind = "delivery"
	NoteOrdered   NoteKind = "ordered"
	NoteUnpaid    NoteKind = "unpaid"
	NoteHired     NoteKind = "hired"
	NoteFired     NoteKind = "fired"
	NoteQuit      NoteKind = "quit"
	NoteSkill     NoteKind = "skill"
	NoteBreakdown NoteKind = "breakdown"
	NoteRepaired  NoteKind = "repaired"
	NoteTheft     NoteKind = "theft"
	NoteReview    NoteKind = "review"
	NoteFine      NoteKind = "fine"
	NoteBribe     NoteKind = "bribe"
	NoteCaught    NoteKind = "caught"
	NoteBigOrder  NoteKind = "bigorder"
	NoteRaise     NoteKind = "raise"
	NoteBankrupt  NoteKind = "bankrupt"
)

// The moods of the workers: overworkMood an hour is lost when a fifth
// more customers come than the workers of a profession serve, unpaidMood
// while the till is in debt.
const (
	overworkLoad = 1.2
	overworkMood = 1.5
	unpaidMood   = 2
)

// The memory of a business: the notes and the hours it keeps.
const (
	MaxBusinessNotes = 60
	MaxBusinessHours = 24
)

// Note is what happened to a business at At: Subject is the supply,
// offer or profession it is about, Name the worker, Amount the units (or
// the new skill, or 1 for a yes), Money the hundredths of a vote — or,
// for a delivery, the units lost.
type Note struct {
	At      time.Time
	Kind    NoteKind
	Subject string
	Name    string
	Female  bool
	Amount  float64
	Money   float64
}

func (b *Business) note(n Note) {
	b.notes = append(b.notes, n)
	if extra := len(b.notes) - MaxBusinessNotes; extra > 0 {
		b.notes = slices.Delete(b.notes, 0, extra)
	}
}

// BusinessHour is what a business did in an hour of the clock: the sales
// of each offer, the income, the wages, the rent, the supplies its
// contracts bought, the fines and thefts — money in hundredths of a vote —
// and the units spoiled and the customers who left with nothing.
type BusinessHour struct {
	Hour     time.Time
	Sold     map[Offer]float64
	Income   float64
	Wages    float64
	Rent     float64
	Supplies float64
	Losses   float64
	Spoiled  float64
	Unserved float64
}

func cloneHours(hours []BusinessHour) []BusinessHour {
	cloned := make([]BusinessHour, len(hours))
	for i, h := range hours {
		sold := make(map[Offer]float64, len(h.Sold))
		for k, v := range h.Sold {
			sold[k] = v
		}

		h.Sold = sold
		cloned[i] = h
	}

	return cloned
}

// hourAt is the hour of the clock t falls in, the hours past the last
// MaxBusinessHours forgotten.
func (b *Business) hourAt(t time.Time) *BusinessHour {
	start := t.Truncate(time.Hour)
	if n := len(b.hours); n > 0 && !b.hours[n-1].Hour.Before(start) {
		return &b.hours[n-1]
	}

	b.hours = append(b.hours, BusinessHour{Hour: start, Sold: map[Offer]float64{}})
	if extra := len(b.hours) - MaxBusinessHours; extra > 0 {
		b.hours = slices.Delete(b.hours, 0, extra)
	}

	return &b.hours[len(b.hours)-1]
}

// OfferRate is an offer of a business now: the customers an hour, those
// its workers serve, and whether its equipment is broken.
type OfferRate struct {
	Offer    Offer
	Demand   float64
	Served   float64
	Capacity float64
	Broken   bool
}

// BusinessRates are how a business runs now, an hour: its offers, the
// supplies they use, how loaded the workers of each profession are (1 —
// fully, more — too much), and the wages and the rent, in hundredths of
// a vote.
type BusinessRates struct {
	Offers []OfferRate
	Use    map[Supply]float64
	Load   map[Profession]float64
	Wages  float64
	Rent   float64
}

// Rates are how b runs at t, the supplies not counted: customers come the
// fewer the dearer an offer is than fair, the worse the premises, and the
// more with an advertisement, a marketer and a good review; the workers
// of a profession share themselves between its offers by their customers.
func (b Business) Rates(p BusinessPlan, r BusinessRules, t time.Time) BusinessRates {
	spec, err := BusinessCatalog(b.kind)
	if err != nil || b.bankrupt {
		return BusinessRates{Use: map[Supply]float64{}, Load: map[Profession]float64{}}
	}

	boost := float64(b.level) * float64(p.Demand) / 100 * (0.4 + 0.6*b.condition/FullCondition)
	boost *= 1 + 0.04*float64(b.skillOf(ProfMarketer))
	if t.Before(b.adUntil) {
		boost *= 1 + float64(p.AdBoost)/100
	}
	if t.Before(b.reviewUntil) {
		boost *= reviewBoost
	}

	capacity := map[Profession]float64{}
	for _, w := range b.workers {
		if ps, err := spec.Profession(w.Profession); err == nil {
			capacity[w.Profession] += ps.Serve * w.rate()
		}
	}

	demands := map[Profession]float64{}
	rates := BusinessRates{Use: map[Supply]float64{}, Load: map[Profession]float64{}, Rent: float64(p.Rent) * float64(b.level)}
	for _, o := range spec.Offers {
		demand := o.Demand * boost * max(0, 2-float64(b.prices[o.Offer])/float64(o.Fair))
		demands[o.Profession] += demand
		rates.Offers = append(rates.Offers, OfferRate{Offer: o.Offer, Demand: demand, Broken: b.broken == o.Offer})
	}

	for i, o := range spec.Offers {
		rate := &rates.Offers[i]
		if total := demands[o.Profession]; total > 0 {
			rate.Capacity = capacity[o.Profession] * rate.Demand / total
		}
		if rate.Broken {
			rate.Capacity /= 2
		}

		rate.Served = min(rate.Demand, rate.Capacity)
		for _, in := range o.Recipe {
			rates.Use[in.Supply] += rate.Served * float64(in.Units)
		}
	}

	for prof, demand := range demands {
		switch {
		case capacity[prof] > 0:
			rates.Load[prof] = demand / capacity[prof]
		case demand > 0:
			rates.Load[prof] = math.Inf(1)
		}
	}

	rates.Wages = float64(b.Wages())

	return rates
}

// At is b at now: the hours since it was counted, an hour after another.
// The supplies come, the contracts order, customers buy what the workers
// serve and the supplies make, the supplies spoil, the wages and the rent
// are paid from the till, the premises wear out, the workers learn and
// change their mood, and things happen. The till keeps no more than it
// holds, the rest is lost; a debt past the bound makes the business
// bankrupt, and then it stops.
func (b Business) At(p BusinessPlan, r BusinessRules, now time.Time) Business {
	now = now.UTC()
	if b.bankrupt || !now.After(b.countedAt) {
		return b
	}

	spec, err := BusinessCatalog(b.kind)
	if err != nil {
		return b
	}

	step := time.Hour
	if elapsed := now.Sub(b.countedAt); elapsed > step*maxBusinessSimSteps {
		step = elapsed / maxBusinessSimSteps
	}

	b = b.clone()
	for t := b.countedAt; t.Before(now) && !b.bankrupt; {
		dt := min(step, now.Sub(t))
		b.step(spec, p, r, t, dt.Hours())
		t = t.Add(dt)
	}

	if !b.bankrupt {
		b.deliver(spec, now)
		b.expire(spec, now, b.hourAt(now))
	}
	b.countedAt = now

	return b
}

// step is the hours of b from t.
func (b *Business) step(spec KindSpec, p BusinessPlan, r BusinessRules, t time.Time, hours float64) {
	hour := b.hourAt(t)

	b.deliver(spec, t)
	b.expire(spec, t, hour)
	if !b.brokenUntil.IsZero() && !b.brokenUntil.After(t) {
		b.note(Note{At: b.brokenUntil, Kind: NoteRepaired, Subject: string(b.broken)})
		b.broken, b.brokenUntil = "", time.Time{}
	}
	b.runContracts(spec, r, t, hour)

	rates := b.Rates(p, r, t)
	wanted := b.sell(spec, rates, hours, hour)
	b.spoil(spec, hours, hour)

	wages, rent := rates.Wages*hours, rates.Rent*hours
	hour.Wages += wages
	hour.Rent += rent
	b.cash = min(b.cash-wages-rent, b.TillAt(p))

	wear := float64(p.Wear) * max(1-0.15*float64(b.skillOf(ProfTechnician)), 0)
	b.condition = max(b.condition-wear*hours, 0)

	b.work(spec, rates, wanted, t, hours)
	b.happen(spec, p, t, hours, hour)

	if b.cash < -b.DebtAt(p) {
		b.bankrupt = true
		b.event = Event{}
		b.note(Note{At: t, Kind: NoteBankrupt})
	}
}

// sell sells the hours of the offers, in order, while the supplies last,
// and tells the customers of each profession the supplies would serve:
// with nothing to sell, the workers are not busy.
func (b *Business) sell(spec KindSpec, rates BusinessRates, hours float64, hour *BusinessHour) map[Profession]float64 {
	wanted := map[Profession]float64{}
	for i, o := range spec.Offers {
		rate := rates.Offers[i]
		sold, could := rate.Served*hours, rate.Demand*hours
		for _, in := range o.Recipe {
			sold = min(sold, b.stock[in.Supply]/float64(in.Units))
			could = min(could, b.stock[in.Supply]/float64(in.Units))
		}
		sold = max(sold, 0)
		wanted[o.Profession] += max(could, 0)

		for _, in := range o.Recipe {
			b.stock[in.Supply] -= sold * float64(in.Units)
		}

		income := sold * float64(b.prices[o.Offer])
		b.cash += income
		hour.Income += income
		hour.Sold[o.Offer] += sold
		hour.Unserved += rate.Demand*hours - sold
	}

	return wanted
}

// spoil spoils the supplies that spoil: a part of the stock an hour.
func (b *Business) spoil(spec KindSpec, hours float64, hour *BusinessHour) {
	for _, s := range spec.Supplies {
		if s.Spoil == 0 || b.stock[s.Supply] <= 0 {
			continue
		}

		lost := b.stock[s.Supply] * min(hours/float64(s.Spoil), 1)
		b.stock[s.Supply] -= lost
		hour.Spoiled += lost
	}
}

// work is the hours of the workers: they learn, their moods drift to
// calm, fall from more customers than they serve and from an unpaid till,
// and the gloomiest quit.
func (b *Business) work(spec KindSpec, rates BusinessRates, wanted map[Profession]float64, t time.Time, hours float64) {
	busy := map[Profession]bool{}
	capacity := map[Profession]float64{}
	for i, o := range spec.Offers {
		capacity[o.Profession] += rates.Offers[i].Capacity * hours
	}
	for prof, w := range wanted {
		busy[prof] = w > capacity[prof]*overworkLoad
	}

	kept := b.workers[:0]
	for _, w := range b.workers {
		learn := hours
		if w.Trait == TraitStudious {
			learn *= 2
		}
		if next := w.NextSkillHours(); next > 0 {
			if w.XP += learn; w.XP >= next {
				w.Skill++
				w.XP = 0
				b.note(Note{At: t, Kind: NoteSkill, Subject: string(w.Profession), Name: w.Name, Female: w.Female, Amount: float64(w.Skill)})
			}
		}

		drift := min(math.Abs(calmMood-w.Mood), hours)
		if w.Mood > calmMood {
			drift = -drift
		}
		w.Mood += drift
		if busy[w.Profession] && w.Trait != TraitCalm {
			w.Mood -= overworkMood * hours
		}
		if b.cash < 0 {
			w.Mood -= unpaidMood * hours
		}
		w.Mood = min(max(w.Mood, 0), 100)

		if w.Mood < quittingMood {
			b.note(Note{At: t, Kind: NoteQuit, Subject: string(w.Profession), Name: w.Name, Female: w.Female})

			continue
		}

		kept = append(kept, w)
	}

	b.workers = kept
}
