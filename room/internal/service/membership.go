package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/room/internal/room"
)

func (s *Service) joinRoom(ctx context.Context, id, actor uuid.UUID) (Joined, error) {
	r, err := s.d.Rooms.Get(ctx, id)
	if err != nil {
		return Joined{}, errs.Wrap("s.d.Rooms.Get", err)
	}
	viewer, err := s.viewer(ctx, r, actor)
	if err != nil {
		return Joined{}, errs.Wrap("s.viewer", err)
	}
	if err = r.CanEnter(viewer); err != nil {
		return Joined{}, errs.Wrap("r.CanEnter", err)
	}
	media, err := s.d.Media.Token(ctx, id, actor)
	if err != nil {
		return Joined{}, errs.Wrap("s.d.Media.Token", err)
	}
	change, err := s.d.Presence.Join(ctx, id, actor, s.d.Clock.Now(), s.d.MaxMembers, s.d.MaxLive)
	if err != nil {
		return Joined{}, errs.Wrap("s.d.Presence.Join", err)
	}
	if change.PreviousRoom != uuid.Nil {
		s.observeLeave(change.PreviousMember)
		if err = s.d.Media.Remove(ctx, change.PreviousRoom, actor); err != nil {
			return Joined{}, errs.Wrap("s.d.Media.Remove", err)
		}
		if err = s.publish(ctx, change.PreviousRoom, Event{Kind: "room_leave", RoomID: change.PreviousRoom, UserID: actor}); err != nil {
			return Joined{}, errs.Wrap("s.publish", err)
		}
	}
	if change.PreviousRoom != uuid.Nil {
		s.d.Events.Publish(ctx, actor, Event{Kind: "room_leave", RoomID: change.PreviousRoom, UserID: actor})
	}
	if change.Joined {
		if err = s.publish(ctx, id, Event{Kind: "room_join", RoomID: id, UserID: actor}); err != nil {
			return Joined{}, errs.Wrap("s.publish", err)
		}
	}
	view, err := s.view(ctx, r)
	if err != nil {
		return Joined{}, errs.Wrap("s.view", err)
	}
	chat, muted, err := s.filteredChat(ctx, id, actor, view.Members)
	if err != nil {
		return Joined{}, errs.Wrap("s.filteredChat", err)
	}
	return Joined{View: view, Chat: chat, MutedIDs: muted, Media: media, ServerTime: s.d.Clock.Now()}, nil
}

func (s *Service) leaveRoom(ctx context.Context, id, actor uuid.UUID) error {
	member, left, err := s.d.Presence.Leave(ctx, id, actor, s.d.Clock.Now())
	if err != nil {
		return errs.Wrap("s.d.Presence.Leave", err)
	}
	if !left {
		return nil
	}
	s.observeLeave(member)
	if err = s.d.Media.Remove(ctx, id, actor); err != nil {
		return errs.Wrap("s.d.Media.Remove", err)
	}
	return errs.Wrap("s.publish", s.publish(ctx, id, Event{Kind: "room_leave", RoomID: id, UserID: actor}))
}

func (s *Service) Heartbeat(ctx context.Context, id, actor uuid.UUID) error {
	if _, err := s.member(ctx, id, actor); err != nil {
		return errs.Wrap("s.member", err)
	}
	return errs.Wrap("s.d.Presence.Heartbeat", s.d.Presence.Heartbeat(ctx, id, actor, s.d.Clock.Now()))
}

func (s *Service) Token(ctx context.Context, id, actor uuid.UUID) (MediaToken, error) {
	if _, err := s.member(ctx, id, actor); err != nil {
		return MediaToken{}, errs.Wrap("s.member", err)
	}
	return s.d.Media.Token(ctx, id, actor)
}

func (s *Service) kickMember(ctx context.Context, in MemberInput) error {
	r, err := s.d.Rooms.Get(ctx, in.RoomID)
	if err != nil {
		return errs.Wrap("s.d.Rooms.Get", err)
	}
	until, err := r.Kick(in.ActorID, in.MemberID, s.d.Clock.Now())
	if err != nil {
		return errs.Wrap("r.Kick", err)
	}
	if err = s.d.Bans.SaveBan(ctx, in.RoomID, in.MemberID, until); err != nil {
		return errs.Wrap("s.d.Bans.SaveBan", err)
	}
	return errs.Wrap("s.d.Media.Remove", s.d.Media.Remove(ctx, in.RoomID, in.MemberID))
}

func (s *Service) MuteMember(ctx context.Context, in MemberInput) error {
	r, err := s.member(ctx, in.RoomID, in.MemberID)
	if err != nil {
		return errs.Wrap("s.member", err)
	}
	if r.OwnerID() != in.ActorID {
		return room.ErrForbidden
	}
	return errs.Wrap("s.d.Media.Mute", s.d.Media.Mute(ctx, in.RoomID, in.MemberID))
}

func (s *Service) invite(ctx context.Context, in MemberInput) error {
	r, err := s.member(ctx, in.RoomID, in.ActorID)
	if err != nil {
		return errs.Wrap("s.member", err)
	}
	v, err := s.viewer(ctx, r, in.MemberID)
	if err != nil {
		return errs.Wrap("s.viewer", err)
	}
	if err = r.CanEnter(v); err != nil {
		return errs.Wrap("r.CanEnter", err)
	}
	allowed, err := s.d.Users.CanInvite(ctx, in.ActorID, in.MemberID)
	if err != nil {
		return errs.Wrap("s.d.Users.CanInvite", err)
	}
	if !allowed {
		return room.ErrForbidden
	}
	return errs.Wrap("s.d.Notify.Invite", s.d.Notify.Invite(ctx, in.ActorID, in.MemberID, in.RoomID, r.Title().String()))
}

func (s *Service) observeLeave(member Member) {
	if s.d.Observer != nil && !member.JoinedAt.IsZero() {
		s.d.Observer.Leave(s.d.Clock.Now().Sub(member.JoinedAt))
	}
}
