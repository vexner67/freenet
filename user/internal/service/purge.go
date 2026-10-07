package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
)

// purgeBatch is how many pages one transaction erases.
const purgeBatch = 100

// DueUserDeleter deletes up to limit pages due to be erased by now, with
// their follows and blocks, and returns whose they were.
type DueUserDeleter interface {
	DeleteDue(ctx context.Context, now time.Time, limit int) ([]uuid.UUID, error)
}

// Eraser records, in the transaction of the deletion, that the other
// services erase what they keep of the user; they do it afterwards,
// retried while they are down (outbox).
type Eraser interface {
	UserErased(ctx context.Context, userID uuid.UUID) error
}

// PurgeService erases the pages their owners deleted and did not restore
// in time: here at once, in the other services soon after.
type PurgeService struct {
	users  DueUserDeleter
	tx     TxManager
	eraser Eraser
	now    func() time.Time
}

func NewPurgeService(users DueUserDeleter, tx TxManager, eraser Eraser) *PurgeService {
	return &PurgeService{
		users:  users,
		tx:     tx,
		eraser: eraser,
		now:    time.Now,
	}
}

// PurgeDue erases every page that is due, a batch per transaction, and
// returns how many it erased.
func (s *PurgeService) PurgeDue(ctx context.Context) (int, error) {
	total := 0

	for {
		var erased int

		err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
			ids, err := s.users.DeleteDue(ctx, s.now(), purgeBatch)
			if err != nil {
				return errs.Wrap("s.users.DeleteDue", err)
			}

			for _, id := range ids {
				if err = s.eraser.UserErased(ctx, id); err != nil {
					return errs.Wrap("s.eraser.UserErased", err)
				}
			}

			erased = len(ids)

			return nil
		})
		if err != nil {
			return total, errs.Wrap("s.tx.WithinTx", err)
		}

		total += erased
		if erased < purgeBatch {
			return total, nil
		}
	}
}
