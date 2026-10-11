package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
)

const (
	// remindBatch is how many events one transaction reminds of.
	remindBatch = 20
	// MaxReminded bounds whom one event reminds: its newest members.
	MaxReminded = 1000
)

// DueEvents finds the events to remind of.
type DueEvents interface {
	// TakeDue marks up to limit events that start after now and not later
	// than until as reminded of, and returns them; an event is taken once,
	// and again after its start is changed.
	TakeDue(ctx context.Context, now, until time.Time, limit int) ([]uuid.UUID, error)
	// ComerIDs returns up to limit members of the event, the newest first,
	// whether they come for sure or only might.
	ComerIDs(ctx context.Context, clubID uuid.UUID, limit int) ([]uuid.UUID, error)
}

// Reminder records, in the transaction that takes the event, that the
// notification service tells a member the event starts soon.
type Reminder interface {
	Reminded(ctx context.Context, clubID, userID uuid.UUID) error
}

// ReminderService tells the members of an event that it starts soon,
// openvk's "В ближайшее время состоится событие": once, ahead of its
// start.
type ReminderService struct {
	events   DueEvents
	tx       TxManager
	reminder Reminder
	ahead    time.Duration
	now      func() time.Time
}

// NewReminderService reminds of the events that start within ahead; now
// is the clock.
func NewReminderService(events DueEvents, tx TxManager, reminder Reminder, ahead time.Duration, now func() time.Time) *ReminderService {
	return &ReminderService{
		events:   events,
		tx:       tx,
		reminder: reminder,
		ahead:    ahead,
		now:      now,
	}
}

// RemindDue reminds of every event that is due, a batch per transaction,
// and returns how many events it reminded of.
func (s *ReminderService) RemindDue(ctx context.Context) (int, error) {
	total := 0

	for {
		var taken int

		err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
			now := s.now()

			ids, err := s.events.TakeDue(ctx, now, now.Add(s.ahead), remindBatch)
			if err != nil {
				return errs.Wrap("s.events.TakeDue", err)
			}

			for _, id := range ids {
				if err = s.remind(ctx, id); err != nil {
					return errs.Wrap("s.remind", err)
				}
			}

			taken = len(ids)

			return nil
		})
		if err != nil {
			return total, errs.Wrap("s.tx.WithinTx", err)
		}

		total += taken
		if taken < remindBatch {
			return total, nil
		}
	}
}

func (s *ReminderService) remind(ctx context.Context, clubID uuid.UUID) error {
	comers, err := s.events.ComerIDs(ctx, clubID, MaxReminded)
	if err != nil {
		return errs.Wrap("s.events.ComerIDs", err)
	}

	for _, userID := range comers {
		if err = s.reminder.Reminded(ctx, clubID, userID); err != nil {
			return errs.Wrap("s.reminder.Reminded", err)
		}
	}

	return nil
}
