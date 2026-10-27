package user

import (
	"encoding/binary"
	"hash/fnv"
	"slices"
	"time"

	"github.com/google/uuid"
)

// Luck is a number from 0 to 1 for what happens in the hour at to id, the
// draw named by what: the same arguments, the same number.
type Luck func(at time.Time, what uint64, id uuid.UUID) float64

// HashLuck is the luck of the city: a hash of its arguments, so that the
// hours counted again come out as before.
func HashLuck(at time.Time, what uint64, id uuid.UUID) float64 {
	h := fnv.New64a()
	var buf [16]byte
	binary.BigEndian.PutUint64(buf[:8], uint64(at.Unix()))
	binary.BigEndian.PutUint64(buf[8:], what)
	_, _ = h.Write(buf[:]) // a hash never fails to write
	_, _ = h.Write(id[:])  // a hash never fails to write
	v := h.Sum64()
	v ^= v >> 33 // mixes the high bits into the low ones, as FNV leaves them weak
	v *= 0xff51afd7ed558ccd
	v ^= v >> 33

	return float64(v>>11) / float64(1<<53)
}

// CalmLuck is luck with nothing happening: no events, no noise; only the
// city, ordering more often than not, orders every hour.
func CalmLuck(time.Time, uint64, uuid.UUID) float64 {
	return 0.5
}

// The draws of an hour.
const (
	drawSector uint64 = iota + 1
	drawSectorKind
	drawFirm
	drawFirmKind
	drawStrike
	drawMood
	drawMoodTwice
	drawMoodThrice
	drawContract
	drawContractGood
	drawContractSize
	drawContractPrice
)

// EventKind is what happens in the city: to all enterprises of a kind, or
// to one.
type EventKind string

const (
	EventDrought    EventKind = "drought"
	EventHarvest    EventKind = "harvest"
	EventVein       EventKind = "vein"
	EventCollapse   EventKind = "collapse"
	EventGusher     EventKind = "gusher"
	EventStorm      EventKind = "storm"
	EventWildfire   EventKind = "wildfire"
	EventBlackout   EventKind = "blackout"
	EventInvention  EventKind = "invention"
	EventFair       EventKind = "fair"
	EventCrisis     EventKind = "crisis"
	EventBoom       EventKind = "boom"
	EventBreakdown  EventKind = "breakdown"
	EventInspection EventKind = "inspection"
	EventAward      EventKind = "award"
	EventStrike     EventKind = "strike"
	EventTender     EventKind = "tender"
	EventTakeover   EventKind = "takeover"
)

// eventSpec is what an event does: for its hours the enterprises of kinds
// (none — all) make output times what they would and the city pays the
// shops city times its price; the mood of their shares changes by mood
// at once. An event of one enterprise wears its machines by wear, fines
// fine percent of its cash, or stops its workers for its hours.
type eventSpec struct {
	kind   EventKind
	kinds  []FirmKind
	output float64
	city   float64
	mood   float64
	hours  int
	wear   float64
	fine   int64
	idle   bool
}

var sectorEvents = []eventSpec{
	{kind: EventDrought, kinds: []FirmKind{FirmFarm}, output: 0.6, city: 1, mood: -0.12, hours: 12},
	{kind: EventHarvest, kinds: []FirmKind{FirmFarm}, output: 1.4, city: 1, mood: 0.1, hours: 12},
	{kind: EventVein, kinds: []FirmKind{FirmMine}, output: 1.5, city: 1, mood: 0.12, hours: 12},
	{kind: EventCollapse, kinds: []FirmKind{FirmMine}, output: 0.5, city: 1, mood: -0.15, hours: 6},
	{kind: EventGusher, kinds: []FirmKind{FirmOilrig}, output: 1.5, city: 1, mood: 0.12, hours: 12},
	{kind: EventStorm, kinds: []FirmKind{FirmOilrig}, output: 0.5, city: 1, mood: -0.15, hours: 6},
	{kind: EventWildfire, kinds: []FirmKind{FirmForestry}, output: 0.5, city: 1, mood: -0.12, hours: 12},
	{kind: EventBlackout, kinds: []FirmKind{FirmMill, FirmSteelworks, FirmChemworks, FirmSawmill}, output: 0.6, city: 1, mood: -0.08, hours: 6},
	{kind: EventInvention, kinds: []FirmKind{FirmJoinery, FirmElectroplant}, output: 1.3, city: 1, mood: 0.1, hours: 12},
	{kind: EventFair, kinds: []FirmKind{FirmShop}, output: 1, city: 1.3, mood: 0.1, hours: 12},
	{kind: EventCrisis, output: 1, city: 0.8, mood: -0.08, hours: 12},
	{kind: EventBoom, output: 1, city: 1.15, mood: 0.08, hours: 12},
}

var firmEvents = []eventSpec{
	{kind: EventBreakdown, output: 1, city: 1, mood: -0.06, wear: 25},
	{kind: EventInspection, output: 1, city: 1, mood: -0.05, fine: 5},
	{kind: EventAward, output: 1, city: 1, mood: 0.12},
}

var strikeEvent = eventSpec{kind: EventStrike, output: 1, city: 1, mood: -0.1, hours: 4, idle: true}

// tenderEvent is an offer to buy out the shareholders of an enterprise,
// news that lifts its shares; takeoverEvent — the one who made it came to
// direct it.
var (
	tenderEvent   = eventSpec{kind: EventTender, output: 1, city: 1, mood: 0.05}
	takeoverEvent = eventSpec{kind: EventTakeover, output: 1, city: 1}
)

const (
	// sectorChance is the chance of an event of a kind in an hour, while
	// fewer than maxSectorEvents go on.
	sectorChance    = 0.06
	maxSectorEvents = 2
	// firmChance is the chance of an event of an enterprise in an hour.
	firmChance = 0.015
	// strikeChance is the chance of a strike in an hour of an enterprise
	// paying below the standard, at half the standard; less paying more.
	strikeChance = 0.06
	// newsKept is how many events the city remembers.
	newsKept = 40
)

// Event is what happened in the city at Start, going on till End: to the
// enterprises of its kind or to the enterprise FirmID.
type Event struct {
	ID     uuid.UUID
	Kind   EventKind
	FirmID uuid.UUID
	Start  time.Time
	End    time.Time
}

func eventOf(kind EventKind) (eventSpec, bool) {
	for _, list := range [][]eventSpec{sectorEvents, firmEvents, {strikeEvent, tenderEvent, takeoverEvent}} {
		if i := slices.IndexFunc(list, func(e eventSpec) bool { return e.kind == kind }); i >= 0 {
			return list[i], true
		}
	}

	return eventSpec{}, false
}

// touches is whether the event of spec touches an enterprise of kind.
func (e eventSpec) touches(kind FirmKind) bool {
	return len(e.kinds) == 0 || slices.Contains(e.kinds, kind)
}

// Kinds are the enterprises an event of all of a kind touches; none — all
// of them, or the one of the event.
func (e Event) Kinds() []FirmKind {
	spec, _ := eventOf(e.Kind)

	return append([]FirmKind(nil), spec.kinds...)
}

// News are the events of the city, the latest first.
func (c *City) News() []Event {
	news := append([]Event(nil), c.events...)
	slices.Reverse(news)

	return news
}

// goes is whether the event goes on in the hour at.
func (e Event) goes(at time.Time) bool {
	return !at.Before(e.Start) && at.Before(e.End)
}

// effects are what the sector events going on in the hour at do to an
// enterprise of kind: its output and the price the city pays.
func (c *City) effects(kind FirmKind, at time.Time) (output, city float64) {
	output, city = 1, 1
	for _, e := range c.events {
		spec, ok := eventOf(e.Kind)
		if !ok || e.FirmID != uuid.Nil || !e.goes(at) || !spec.touches(kind) {
			continue
		}

		output *= spec.output
		city *= spec.city
	}

	return output, city
}

// happen draws the events of the hour at: one for a kind, now and then,
// and those of single enterprises.
func (c *City) happen(rules FirmRules, at time.Time) {
	going := 0
	for _, e := range c.events {
		if e.FirmID == uuid.Nil && e.goes(at) {
			going++
		}
	}

	if going < maxSectorEvents && c.luck(at, drawSector, uuid.Nil) < sectorChance {
		spec := sectorEvents[int(c.luck(at, drawSectorKind, uuid.Nil)*float64(len(sectorEvents)))%len(sectorEvents)]
		c.begin(spec, uuid.Nil, at)
		for _, f := range c.firms {
			if spec.touches(f.kind) {
				f.mood += spec.mood
			}
		}
	}

	for _, f := range c.firms {
		c.befall(rules, f, at)
	}
}

// befall draws what happens to the enterprise in the hour at.
func (c *City) befall(rules FirmRules, f *Firm, at time.Time) {
	if f.wage < rules.Wage && f.workers > 0 && at.After(f.idleUntil) {
		chance := strikeChance * float64(rules.Wage-f.wage) / float64(rules.Wage) * 2
		if c.luck(at, drawStrike, f.id) < chance {
			e := c.begin(strikeEvent, f.id, at)
			f.idleUntil = e.End
			f.mood += strikeEvent.mood
		}
	}

	if c.luck(at, drawFirm, f.id) >= firmChance {
		return
	}

	spec := firmEvents[int(c.luck(at, drawFirmKind, f.id)*float64(len(firmEvents)))%len(firmEvents)]
	c.begin(spec, f.id, at)
	f.mood += spec.mood
	f.wear = min(f.wear+spec.wear, 100)
	f.cash -= f.cash * spec.fine / 100
}

func (c *City) begin(spec eventSpec, firmID uuid.UUID, at time.Time) Event {
	id, err := uuid.NewV7()
	if err != nil {
		id = uuid.New()
	}

	e := Event{ID: id, Kind: spec.kind, FirmID: firmID, Start: at, End: at.Add(time.Duration(spec.hours) * time.Hour)}
	c.events = append(c.events, e)
	if over := len(c.events) - newsKept; over > 0 {
		c.events = append([]Event(nil), c.events[over:]...)
	}

	return e
}
