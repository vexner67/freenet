package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/room/internal/room"
)

const PresenceTTL = 45 * time.Second

func (s *Service) Sweep(ctx context.Context) error {
	return errs.Wrap("s.d.SweepLock.SweepLocked", s.d.SweepLock.SweepLocked(ctx, s.sweep))
}

func (s *Service) sweep(ctx context.Context) error {
	live, err := s.d.Index.Live(ctx)
	if err != nil {
		return errs.Wrap("s.d.Index.Live", err)
	}
	for _, id := range live {
		members, err := s.d.Presence.Members(ctx, id)
		if err != nil {
			return errs.Wrap("s.d.Presence.Members", err)
		}
		for _, m := range members {
			if err := s.expireMember(ctx, id, m); err != nil {
				return errs.Wrap("s.expireMember", err)
			}
		}
	}
	if err := s.closeEmpty(ctx); err != nil {
		return err
	}
	return s.observeSnapshot(ctx)
}

func (s *Service) closeEmpty(ctx context.Context) error {
	f := Filter{Limit: 50}
	for {
		rows, err := s.d.Rooms.List(ctx, f)
		if err != nil {
			return errs.Wrap("s.d.Rooms.List", err)
		}
		for _, r := range rows {
			f.Cursor = r.ID()
			if r.Permanent() {
				continue
			}
			empty, err := s.d.Index.EmptySince(ctx, r.ID())
			if err != nil {
				return errs.Wrap("s.d.Index.EmptySince", err)
			}
			if empty.IsZero() {
				members, err := s.d.Presence.Members(ctx, r.ID())
				if err != nil {
					return errs.Wrap("s.d.Presence.Members", err)
				}
				if len(members) > 0 {
					continue
				}
				empty = r.CreatedAt()
			}
			if empty.Add(s.d.EmptyTTL).After(s.d.Clock.Now()) {
				continue
			}
			err = s.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
				// Recheck under the lock shared with joins.
				members, err := s.d.Presence.Members(ctx, r.ID())
				if err != nil {
					return errs.Wrap("s.d.Presence.Members", err)
				}
				if len(members) > 0 {
					return nil
				}
				since, err := s.d.Index.EmptySince(ctx, r.ID())
				if err != nil {
					return errs.Wrap("s.d.Index.EmptySince", err)
				}
				if since.IsZero() {
					since = r.CreatedAt()
				}
				if since.Add(s.d.EmptyTTL).After(s.d.Clock.Now()) {
					return nil
				}
				return errs.Wrap("s.CloseRoom", s.CloseRoom(ctx, r.ID(), r.OwnerID()))
			})
			if err != nil {
				return errs.Wrap("s.d.Tx.WithinTx", err)
			}
		}
		if len(rows) < f.Limit {
			return nil
		}
	}
}

func (s *Service) PurgeUser(ctx context.Context, id uuid.UUID) error {
	current, err := s.d.Index.Current(ctx, []uuid.UUID{id})
	if err != nil {
		return errs.Wrap("s.d.Index.Current", err)
	}
	if r := current[id]; r != uuid.Nil {
		if err = s.LeaveRoom(ctx, r, id); err != nil {
			return errs.Wrap("s.LeaveRoom", err)
		}
	}
	if err = s.purge(ctx, Filter{OwnerID: id, Limit: 50, IncludeClosed: true}); err != nil {
		return errs.Wrap("s.purge", err)
	}
	return errs.Wrap("s.d.Bans.DeleteUser", s.d.Bans.DeleteUser(ctx, id))
}
func (s *Service) PurgeClub(ctx context.Context, id uuid.UUID) error {
	return s.purge(ctx, Filter{ClubID: id, Limit: 50, IncludeClosed: true})
}
func (s *Service) purge(ctx context.Context, f Filter) error {
	for {
		rows, err := s.d.Rooms.List(ctx, f)
		if err != nil {
			return errs.Wrap("s.d.Rooms.List", err)
		}
		for _, r := range rows {
			f.Cursor = r.ID()
			if err = s.CloseRoom(ctx, r.ID(), r.OwnerID()); err != nil {
				return errs.Wrap("s.CloseRoom", err)
			}
			if err = s.d.Rooms.Delete(ctx, r.ID()); err != nil {
				return errs.Wrap("s.d.Rooms.Delete", err)
			}
		}
		if len(rows) < f.Limit {
			return nil
		}
	}
}

func (s *Service) observeSnapshot(ctx context.Context) error {
	if s.d.Observer == nil {
		return nil
	}
	ids, err := s.d.Index.Live(ctx)
	if err != nil {
		return errs.Wrap("s.d.Index.Live", err)
	}
	kinds := map[room.Kind]int{}
	count := 0
	for _, id := range ids {
		r, err := s.d.Rooms.Get(ctx, id)
		if err != nil {
			return errs.Wrap("s.d.Rooms.Get", err)
		}
		members, err := s.d.Presence.Members(ctx, id)
		if err != nil {
			return errs.Wrap("s.d.Presence.Members", err)
		}
		if r.ClosedAt().IsZero() && len(members) > 0 {
			kinds[r.Kind()]++
			count += len(members)
		}
	}
	s.d.Observer.Snapshot(kinds, count)
	return nil
}

// Expiration and media removal share the join lock. A newly rejoined session
// must not be disconnected by the tail of an earlier expiration.
func (s *Service) expireMember(ctx context.Context, id uuid.UUID, m Member) error {
	return errs.Wrap("s.d.Tx.WithinTx", s.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		member, left, err := s.d.Expiry.Expire(ctx, id, m.UserID, s.d.Clock.Now().Add(-PresenceTTL), s.d.Clock.Now())
		if err != nil {
			return errs.Wrap("s.d.Expiry.Expire", err)
		}
		if !left {
			return nil
		}
		s.observeLeave(member)
		if err = s.d.Media.Remove(ctx, id, m.UserID); err != nil {
			return errs.Wrap("s.d.Media.Remove", err)
		}
		return errs.Wrap("s.publish", s.publish(ctx, id, Event{Kind: "room_leave", RoomID: id, UserID: m.UserID}))
	}))
}
