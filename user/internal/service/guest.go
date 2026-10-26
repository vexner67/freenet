package service

import (
	"context"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/user/internal/user"
)

// MaxNewGuests is the most new guests counted: the menu shows "99+".
const MaxNewGuests = 100

// defaultGuests is how many guests a page holds when the caller does not
// say.
const defaultGuests = 30

// GuestKey is where a page of guests goes on from: the visit of the last
// guest shown.
type GuestKey struct {
	VisitedAt time.Time
	GuestID   uuid.UUID
}

// GuestRepository keeps who visited whose page and when they last did.
type GuestRepository interface {
	// Visit writes down the visit of guestID to the page of ownerID at
	// at, in place of the one before.
	Visit(ctx context.Context, ownerID, guestID uuid.UUID, at time.Time) error
	// Guests are the guests of ownerID who visited after since, the last
	// visit first, past after (nil: from the last).
	Guests(ctx context.Context, ownerID uuid.UUID, since time.Time, after *GuestKey, limit int) ([]user.Guest, error)
	// CountSince counts the guests who visited after since, at most
	// limit.
	CountSince(ctx context.Context, ownerID uuid.UUID, since time.Time, limit int) (int, error)
	// SeenAt is when ownerID last looked at their guests, zero if never.
	SeenAt(ctx context.Context, ownerID uuid.UUID) (time.Time, error)
	MarkSeen(ctx context.Context, ownerID uuid.UUID, at time.Time) error
}

// GuestPage is one screen of guests: SeenAt is when the owner had looked
// at them before, so newer visits are new; NextCursor is "" on the last.
type GuestPage struct {
	Guests     []user.Guest
	SeenAt     time.Time
	NextCursor string
}

// GuestService is "Гости": who visited a page in the last GuestsFor.
type GuestService struct {
	userRepo UserRepository
	guests   GuestRepository
}

func NewGuestService(users UserRepository, guests GuestRepository) *GuestService {
	return &GuestService{userRepo: users, guests: guests}
}

// Visit writes down a visit of guestID to the page of nickname at now,
// unless it leaves no trace; user.ErrNotFound.
func (s *GuestService) Visit(ctx context.Context, guestID uuid.UUID, nickname user.Nickname, now time.Time) error {
	owner, err := s.userRepo.GetByNickname(ctx, nickname)
	if err != nil {
		return errs.Wrap("s.userRepo.GetByNickname", err)
	}

	guest, err := s.userRepo.GetByID(ctx, guestID)
	if err != nil {
		return errs.Wrap("s.userRepo.GetByID", err)
	}

	if !guest.LeavesTraceOn(owner, now) {
		return nil
	}

	return errs.Wrap("s.guests.Visit", s.guests.Visit(ctx, owner.ID(), guestID, now.Truncate(time.Microsecond)))
}

// Guests are the guests of ownerID at now from cursor; the first page
// marks them seen.
func (s *GuestService) Guests(ctx context.Context, ownerID uuid.UUID, cursor string, limit int, now time.Time) (GuestPage, error) {
	var after *GuestKey
	if cursor != "" {
		micros, id, ok := splitCursor(cursor)
		if !ok {
			return GuestPage{}, ErrCursor
		}
		after = &GuestKey{VisitedAt: time.UnixMicro(micros).UTC(), GuestID: id}
	}

	if limit <= 0 {
		limit = defaultGuests
	}
	limit = min(limit, user.MaxGuests)

	guests, err := s.guests.Guests(ctx, ownerID, now.Add(-user.GuestsFor), after, limit+1)
	if err != nil {
		return GuestPage{}, errs.Wrap("s.guests.Guests", err)
	}

	var page GuestPage
	if len(guests) > limit {
		guests = guests[:limit]
		last := guests[limit-1]
		page.NextCursor = strconv.FormatInt(last.VisitedAt.UnixMicro(), 10) + "_" + last.GuestID.String()
	}
	page.Guests = guests

	if page.SeenAt, err = s.guests.SeenAt(ctx, ownerID); err != nil {
		return GuestPage{}, errs.Wrap("s.guests.SeenAt", err)
	}

	if after == nil {
		if err = s.guests.MarkSeen(ctx, ownerID, now.Truncate(time.Microsecond)); err != nil {
			return GuestPage{}, errs.Wrap("s.guests.MarkSeen", err)
		}
	}

	return page, nil
}

// NewGuests counts the guests of ownerID since they last looked, at most
// MaxNewGuests.
func (s *GuestService) NewGuests(ctx context.Context, ownerID uuid.UUID, now time.Time) (int, error) {
	seen, err := s.guests.SeenAt(ctx, ownerID)
	if err != nil {
		return 0, errs.Wrap("s.guests.SeenAt", err)
	}

	since := now.Add(-user.GuestsFor)
	if seen.After(since) {
		since = seen
	}

	n, err := s.guests.CountSince(ctx, ownerID, since, MaxNewGuests)

	return n, errs.Wrap("s.guests.CountSince", err)
}
