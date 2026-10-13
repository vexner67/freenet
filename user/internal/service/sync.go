package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
)

// Sizes of the sync of feed sources: how many readers are listed at once
// and how many follows one stretch tells (the most post takes).
const (
	syncReaders = 100
	syncChunk   = 1000
)

// FollowBook reads the follows as the news of post need them, by ID.
type FollowBook interface {
	// Readers are up to limit users with IDs above after, ascending.
	Readers(ctx context.Context, after uuid.UUID, limit int) ([]uuid.UUID, error)
	// Followees are up to limit of those readerID follows with IDs above
	// after, ascending.
	Followees(ctx context.Context, readerID, after uuid.UUID, limit int) ([]uuid.UUID, error)
	FollowLock
}

// FollowLock keeps, until the transaction of ctx ends, everyone else from
// telling post of the follows of readerID: what is told of them is then
// told in the order it became true.
type FollowLock interface {
	LockReader(ctx context.Context, readerID uuid.UUID) error
}

// Sources records, in the transaction of ctx, a stretch of whom readerID
// follows for post to make its copy match: the IDs above after, up to the
// last of ids, or to the end when last.
type Sources interface {
	Following(ctx context.Context, readerID, after uuid.UUID, ids []uuid.UUID, last bool) error
}

// SourceSyncService tells post, now and then, whom everybody follows.
// Post hears of each follow and its end as they happen; a word lost on
// the way (a record the outbox dropped) would leave its copy wrong for
// good, and this mends it.
type SourceSyncService struct {
	book    FollowBook
	tx      TxManager
	sources Sources
}

func NewSourceSyncService(book FollowBook, tx TxManager, sources Sources) *SourceSyncService {
	return &SourceSyncService{
		book:    book,
		tx:      tx,
		sources: sources,
	}
}

// SyncAll tells the follows of every user and returns how many users
// that was.
func (s *SourceSyncService) SyncAll(ctx context.Context) (int, error) {
	total, after := 0, uuid.Nil

	for {
		readers, err := s.book.Readers(ctx, after, syncReaders)
		if err != nil {
			return total, errs.Wrap("s.book.Readers", err)
		}

		for _, id := range readers {
			if err = s.sync(ctx, id); err != nil {
				return total, errs.Wrap("s.sync", err)
			}

			total++
		}

		if len(readers) < syncReaders {
			return total, nil
		}

		after = readers[len(readers)-1]
	}
}

// sync tells the follows of one reader, in one transaction and under the
// lock of the reader: a follow made meanwhile is told after the stretch,
// not before it, where the stretch would undo it.
func (s *SourceSyncService) sync(ctx context.Context, readerID uuid.UUID) error {
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.book.LockReader(ctx, readerID); err != nil {
			return errs.Wrap("s.book.LockReader", err)
		}

		after := uuid.Nil

		for {
			ids, err := s.book.Followees(ctx, readerID, after, syncChunk)
			if err != nil {
				return errs.Wrap("s.book.Followees", err)
			}

			last := len(ids) < syncChunk
			if err = s.sources.Following(ctx, readerID, after, ids, last); err != nil {
				return errs.Wrap("s.sources.Following", err)
			}

			if last {
				return nil
			}

			after = ids[len(ids)-1]
		}
	})

	return errs.Wrap("s.tx.WithinTx", err)
}
