package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
)

// Sizes of the sync of feed sources: how many clubs are listed at once
// and how many members one stretch tells (the most post takes).
const (
	syncClubs = 100
	syncChunk = 1000
)

// MemberBook reads the memberships as the news of post need them, by ID.
type MemberBook interface {
	// ClubIDs are up to limit clubs with IDs above after, ascending.
	ClubIDs(ctx context.Context, after uuid.UUID, limit int) ([]uuid.UUID, error)
	// MemberIDs are up to limit members of the club with IDs above
	// after, ascending.
	MemberIDs(ctx context.Context, clubID, after uuid.UUID, limit int) ([]uuid.UUID, error)
	ClubLock
}

// ClubLock keeps, until the transaction of ctx ends, everyone else from
// telling post who is in the club: what is told of its members is then
// told in the order it became true.
type ClubLock interface {
	LockClub(ctx context.Context, clubID uuid.UUID) error
}

// Rosters records, in the transaction of ctx, a stretch of who is in the
// club for post to make its copy match: the IDs above after, up to the
// last of ids, or to the end when last.
type Rosters interface {
	Roster(ctx context.Context, clubID, after uuid.UUID, ids []uuid.UUID, last bool) error
}

// ReaderSyncService tells post, now and then, who is in every club. Post
// hears of each member who joins and leaves as it happens; a word lost on
// the way (a record the outbox dropped) would leave its copy wrong for
// good, and this mends it.
type ReaderSyncService struct {
	book    MemberBook
	tx      TxManager
	rosters Rosters
}

func NewReaderSyncService(book MemberBook, tx TxManager, rosters Rosters) *ReaderSyncService {
	return &ReaderSyncService{
		book:    book,
		tx:      tx,
		rosters: rosters,
	}
}

// SyncAll tells the members of every club and returns how many clubs
// that was.
func (s *ReaderSyncService) SyncAll(ctx context.Context) (int, error) {
	total, after := 0, uuid.Nil

	for {
		clubs, err := s.book.ClubIDs(ctx, after, syncClubs)
		if err != nil {
			return total, errs.Wrap("s.book.ClubIDs", err)
		}

		for _, id := range clubs {
			if err = s.sync(ctx, id); err != nil {
				return total, errs.Wrap("s.sync", err)
			}

			total++
		}

		if len(clubs) < syncClubs {
			return total, nil
		}

		after = clubs[len(clubs)-1]
	}
}

// sync tells the members of one club, in one transaction and under the
// lock of the club: a member who joins meanwhile is told after the
// stretch, not before it, where the stretch would undo it.
func (s *ReaderSyncService) sync(ctx context.Context, clubID uuid.UUID) error {
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.book.LockClub(ctx, clubID); err != nil {
			return errs.Wrap("s.book.LockClub", err)
		}

		after := uuid.Nil

		for {
			ids, err := s.book.MemberIDs(ctx, clubID, after, syncChunk)
			if err != nil {
				return errs.Wrap("s.book.MemberIDs", err)
			}

			last := len(ids) < syncChunk
			if err = s.rosters.Roster(ctx, clubID, after, ids, last); err != nil {
				return errs.Wrap("s.rosters.Roster", err)
			}

			if last {
				return nil
			}

			after = ids[len(ids)-1]
		}
	})

	return errs.Wrap("s.tx.WithinTx", err)
}
