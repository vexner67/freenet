package world

import (
	"time"

	"github.com/google/uuid"
)

// EventKind is what happened in a world.
type EventKind string

const (
	EventFounded   EventKind = "founded"
	EventJoined    EventKind = "joined"
	EventLeft      EventKind = "left"
	EventExpelled  EventKind = "expelled"
	EventAppointed EventKind = "appointed"
	// EventSucceeded: the ruler gave the state to another or left the
	// site, and another rules.
	EventSucceeded EventKind = "succeeded"
	EventColonized EventKind = "colonized"
	EventDissolved EventKind = "dissolved"
)

// Event is a piece of the news of a world. Name is the name of the state
// then, so that the news read the same after it is gone; Role is the
// office given.
type Event struct {
	ID       uuid.UUID
	Kind     EventKind
	StateID  uuid.UUID
	UserID   uuid.UUID
	Province int
	Name     string
	Role     Role
	At       time.Time
}
