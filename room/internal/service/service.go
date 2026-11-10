package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/room/internal/room"
)

type Deps struct {
	MediaTokens         MediaTokens
	Observer            Observer
	Expiry              PresenceExpiry
	Rooms               RoomRepository
	Bans                BanRepository
	Presence            Presence
	Index               PresenceIndex
	SweepLock           SweepLock
	Playback            PlaybackStore
	Chat                ChatLog
	Users               Users
	Clubs               Clubs
	Media               MediaServer
	Notify              Notifier
	Events              Publisher
	Clock               Clock
	Tx                  TxManager
	MaxMembers, MaxLive int
	EmptyTTL            time.Duration
}

type Service struct{ d Deps }

func New(d Deps) *Service { return &Service{d: d} }

func (s *Service) CreateRoom(ctx context.Context, in CreateInput) (*room.Room, error) {
	in.Room.OwnerID = in.ActorID
	if in.Room.ClubID != uuid.Nil {
		access, err := s.d.Clubs.Access(ctx, in.Room.ClubID, in.ActorID)
		if err != nil {
			return nil, errs.Wrap("s.d.Clubs.Access", err)
		}
		if !access.Manager {
			return nil, room.ErrForbidden
		}
	}
	r, err := room.New(in.Room, s.d.Clock.Now())
	if err != nil {
		return nil, errs.Wrap("room.New", err)
	}
	err = s.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		if r.Permanent() {
			f := Filter{OwnerID: r.OwnerID(), PersonalOnly: true, PermanentOnly: true, Limit: 6}
			quota := 3
			if r.ClubID() != uuid.Nil {
				f.OwnerID, f.ClubID = uuid.Nil, r.ClubID()
				f.PersonalOnly = false
				quota = 5
			}
			existing, err := s.d.Rooms.List(ctx, f)
			if err != nil {
				return errs.Wrap("s.d.Rooms.List", err)
			}
			if len(existing) >= quota {
				return room.ErrTooManyRooms
			}
		}
		return errs.Wrap("s.d.Rooms.Save", s.d.Rooms.Save(ctx, r))
	})
	if err != nil {
		return nil, errs.Wrap("s.d.Tx.WithinTx", err)
	}
	return r, nil
}

func (s *Service) viewer(ctx context.Context, r *room.Room, id uuid.UUID) (room.Viewer, error) {
	v := room.Viewer{ID: id}
	if id == uuid.Nil {
		return v, nil
	}
	a, err := s.d.Users.Access(ctx, r.OwnerID(), id)
	if err != nil {
		return v, errs.Wrap("s.d.Users.Access", err)
	}
	v.Friend, v.Blocked, v.Banned = a.Friend, a.Blocked, a.Banned
	until, err := s.d.Bans.Until(ctx, r.ID(), id)
	if err != nil {
		return v, errs.Wrap("s.d.Bans.Until", err)
	}
	v.Kicked = until.After(s.d.Clock.Now())
	if r.Access() == room.AccessClub {
		a, err := s.d.Clubs.Access(ctx, r.ClubID(), id)
		if err != nil {
			return v, errs.Wrap("s.d.Clubs.Access", err)
		}
		v.ClubMember = a.Member
	}
	return v, nil
}

func (s *Service) readable(ctx context.Context, id, viewer uuid.UUID, listing bool) (*room.Room, error) {
	r, err := s.d.Rooms.Get(ctx, id)
	if err != nil {
		return nil, errs.Wrap("s.d.Rooms.Get", err)
	}
	v, err := s.viewer(ctx, r, viewer)
	if err != nil {
		return nil, errs.Wrap("s.viewer", err)
	}
	if !r.CanView(v, listing) {
		return nil, room.ErrNotFound
	}
	return r, nil
}

func (s *Service) view(ctx context.Context, r *room.Room) (View, error) {
	members, err := s.d.Presence.Members(ctx, r.ID())
	if err != nil {
		return View{}, errs.Wrap("s.d.Presence.Members", err)
	}
	playback, err := s.d.Playback.Playback(ctx, r.ID())
	if err != nil {
		return View{}, errs.Wrap("s.d.Playback.Playback", err)
	}
	return View{Room: r, Members: members, Playback: playback, MaxMembers: s.d.MaxMembers}, nil
}

func (s *Service) GetRoom(ctx context.Context, id, viewer uuid.UUID) (View, error) {
	r, err := s.readable(ctx, id, viewer, false)
	if err != nil {
		return View{}, errs.Wrap("s.readable", err)
	}
	return s.view(ctx, r)
}

// ListRooms fills a visible page before returning its cursor: private rooms
// never leak through counts, and a page of hidden rooms is not the last page.
func (s *Service) ListRooms(ctx context.Context, viewer uuid.UUID, f Filter, live bool) ([]View, uuid.UUID, error) {
	limit := f.Limit
	if limit < 1 || limit > 50 {
		limit = 20
	}
	var active map[uuid.UUID]bool
	if live {
		ids, err := s.d.Index.Live(ctx)
		if err != nil {
			return nil, uuid.Nil, errs.Wrap("s.d.Index.Live", err)
		}
		active = make(map[uuid.UUID]bool, len(ids))
		for _, id := range ids {
			active[id] = true
		}
	}
	result := make([]View, 0, limit)
	f.Limit = 50
	for {
		rows, err := s.d.Rooms.List(ctx, f)
		if err != nil {
			return nil, uuid.Nil, errs.Wrap("s.d.Rooms.List", err)
		}
		for _, r := range rows {
			f.Cursor = r.ID()
			if live && !active[r.ID()] {
				continue
			}
			v, err := s.viewer(ctx, r, viewer)
			if err != nil {
				return nil, uuid.Nil, errs.Wrap("s.viewer", err)
			}
			if !r.CanView(v, true) {
				continue
			}
			view, err := s.view(ctx, r)
			if err != nil {
				return nil, uuid.Nil, errs.Wrap("s.view", err)
			}
			if viewer != uuid.Nil {
				for _, member := range view.Members {
					if member.UserID == viewer {
						continue
					}
					relation, err := s.d.Users.Access(ctx, member.UserID, viewer)
					if err != nil {
						return nil, uuid.Nil, errs.Wrap("s.d.Users.Access", err)
					}
					if relation.Friend && !relation.Blocked && !relation.Banned {
						view.Friends++
					}
				}
			}

			result = append(result, view)
			if len(result) == limit {
				return result, f.Cursor, nil
			}
		}
		if len(rows) < f.Limit {
			return result, uuid.Nil, nil
		}
	}
}

func (s *Service) updateRoom(ctx context.Context, in UpdateInput) (View, error) {
	r, err := s.d.Rooms.Get(ctx, in.RoomID)
	if err != nil {
		return View{}, errs.Wrap("s.d.Rooms.Get", err)
	}
	if err = r.Update(in.ActorID, in.Room); err != nil {
		return View{}, errs.Wrap("r.Update", err)
	}
	playback, err := s.d.Playback.Playback(ctx, in.RoomID)
	if err != nil {
		return View{}, errs.Wrap("s.d.Playback.Playback", err)
	}
	if !r.AcceptsPlayback(playback.Kind()) {
		cleared, err := room.NewPlayback(room.PlaybackInput{}, playback.Version()+1, s.d.Clock.Now())
		if err != nil {
			return View{}, errs.Wrap("room.NewPlayback", err)
		}
		if err = s.d.Playback.ResetPlayback(ctx, in.RoomID, playback.Version(), cleared); err != nil {
			return View{}, errs.Wrap("s.d.Playback.ResetPlayback", err)
		}
		if err = s.publish(ctx, in.RoomID, Event{Kind: "room_playback", RoomID: in.RoomID, Playback: &cleared}); err != nil {
			return View{}, errs.Wrap("s.publish", err)
		}
	}
	if err = s.d.Rooms.Save(ctx, r); err != nil {
		return View{}, errs.Wrap("s.d.Rooms.Save", err)
	}
	if err = s.publish(ctx, in.RoomID, Event{Kind: "room_update", RoomID: in.RoomID, Room: r}); err != nil {
		return View{}, errs.Wrap("s.publish", err)
	}
	return s.view(ctx, r)
}

func (s *Service) closeRoom(ctx context.Context, id, actor uuid.UUID) error {
	r, err := s.d.Rooms.Get(ctx, id)
	if err != nil {
		return errs.Wrap("s.d.Rooms.Get", err)
	}
	if err = r.Close(actor, s.d.Clock.Now()); err != nil {
		return errs.Wrap("r.Close", err)
	}
	if err = s.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.d.Rooms.Save(ctx, r); err != nil {
			return errs.Wrap("s.d.Rooms.Save", err)
		}
		return errs.Wrap("s.d.Notify.Revoke", s.d.Notify.Revoke(ctx, id))
	}); err != nil {
		return errs.Wrap("s.d.Tx.WithinTx", err)
	}
	members, err := s.d.Index.Close(ctx, id, s.d.Clock.Now())
	if err != nil {
		return errs.Wrap("s.d.Index.Close", err)
	}
	for _, member := range members {
		s.observeLeave(member)
		s.d.Events.Publish(ctx, member.UserID, Event{Kind: "room_closed", RoomID: id})
	}
	return errs.Wrap("s.d.Media.Delete", s.d.Media.Delete(ctx, id))
}

func (s *Service) publish(ctx context.Context, id uuid.UUID, event Event) error {
	members, err := s.d.Presence.Members(ctx, id)
	if err != nil {
		return errs.Wrap("s.d.Presence.Members", err)
	}
	for _, member := range members {
		s.observeLeave(member)
		s.d.Events.Publish(ctx, member.UserID, event)
	}
	return nil
}

func (s *Service) member(ctx context.Context, id, actor uuid.UUID) (*room.Room, error) {
	r, err := s.readable(ctx, id, actor, false)
	if err != nil {
		return nil, errs.Wrap("s.readable", err)
	}
	current, err := s.d.Index.Current(ctx, []uuid.UUID{actor})
	if err != nil {
		return nil, errs.Wrap("s.d.Index.Current", err)
	}
	if current[actor] != id {
		return nil, room.ErrNotMember
	}
	return r, nil
}

func (s *Service) CurrentRooms(ctx context.Context, viewer uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]View, error) {
	current, err := s.d.Index.Current(ctx, ids)
	if err != nil {
		return nil, errs.Wrap("s.d.Index.Current", err)
	}
	result := make(map[uuid.UUID]View)
	for userID, roomID := range current {
		v, err := s.GetRoom(ctx, roomID, viewer)
		if errors.Is(err, room.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, errs.Wrap("s.GetRoom", err)
		}
		result[userID] = v
	}
	return result, nil
}

// AuthorizeMedia guards each signalling connection. Self-hosted LiveKit does
// not revoke cached JWTs on removal, so a valid signature alone is insufficient.
func (s *Service) AuthorizeMedia(ctx context.Context, token string) error {
	id, user, err := s.d.MediaTokens.Verify(token)
	if err != nil {
		return errs.Wrap("s.d.MediaTokens.Verify", err)
	}
	_, err = s.member(ctx, id, user)
	return errs.Wrap("s.member", err)
}
