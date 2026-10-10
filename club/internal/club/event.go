package club

import (
	"errors"
	"time"
)

var (
	ErrInvalidKind     = errors.New("invalid club kind")
	ErrNoStart         = errors.New("an event needs the time it starts")
	ErrEndsBeforeStart = errors.New("an event ends after it starts")
)

// Kind is what a club is: a group, or an event ("встреча" of openvk),
// which is a group with the time it starts and people who come to it.
type Kind string

const (
	KindGroup Kind = "group"
	KindEvent Kind = "event"
)

func NewKind(s string) (Kind, error) {
	switch k := Kind(s); k {
	case KindGroup, KindEvent:
		return k, nil
	default:
		return "", ErrInvalidKind
	}
}

func (k Kind) String() string {
	return string(k)
}

// Schedule is when an event starts and, if End is not zero, ends. A group
// has the zero schedule.
type Schedule struct {
	Start time.Time
	End   time.Time
}

// NewSchedule reads the schedule of an event: it has a start and, if an
// end, a later one.
func NewSchedule(start, end time.Time) (Schedule, error) {
	if start.IsZero() {
		return Schedule{}, ErrNoStart
	}

	if !end.IsZero() && !end.After(start) {
		return Schedule{}, ErrEndsBeforeStart
	}

	// PostgreSQL keeps microseconds; trim so a schedule reads back equal.
	s := Schedule{Start: start.UTC().Truncate(time.Microsecond)}
	if !end.IsZero() {
		s.End = end.UTC().Truncate(time.Microsecond)
	}

	return s, nil
}

// NewEvent creates an event: a club of its kind with a schedule,
// otherwise as New makes it.
func NewEvent(name Name, description Description, schedule Schedule, now time.Time) (*Club, error) {
	if schedule.Start.IsZero() {
		return nil, ErrNoStart
	}

	c, err := New(name, description, now)
	if err != nil {
		return nil, err
	}

	c.kind = KindEvent
	c.schedule = schedule

	return c, nil
}

func (c *Club) Kind() Kind {
	return c.kind
}

func (c *Club) IsEvent() bool {
	return c.kind == KindEvent
}

func (c *Club) Schedule() Schedule {
	return c.schedule
}

// Maybes counts the members who only might come to the event ("Возможно
// придут"); the others of Members are sure to ("Точно придут").
func (c *Club) Maybes() int {
	return c.maybes
}

// Started: the event has begun, which openvk takes for over.
func (c *Club) Started(now time.Time) bool {
	return c.IsEvent() && !now.Before(c.schedule.Start)
}

// JoinsAsMaybe tells how someone who asks to join as maybe is counted:
// only an event that has not started takes "Возможно приду", as in
// openvk; anything else is joined for sure.
func (c *Club) JoinsAsMaybe(maybe bool, now time.Time) bool {
	return maybe && c.IsEvent() && !c.Started(now)
}
