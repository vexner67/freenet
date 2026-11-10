package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/room/internal/room"
)

func (s *Service) setPlayback(ctx context.Context, in PlaybackInput) (room.Playback, error) {
	r, err := s.member(ctx, in.RoomID, in.ActorID)
	if err != nil {
		return room.Playback{}, errs.Wrap("s.member", err)
	}
	if !r.CanControl(in.ActorID, true) {
		return room.Playback{}, room.ErrForbidden
	}
	p, err := room.NewPlayback(in.Playback, in.Version+1, s.d.Clock.Now())
	if err != nil {
		return room.Playback{}, errs.Wrap("room.NewPlayback", err)
	}
	current, err := s.d.Playback.Playback(ctx, in.RoomID)
	if err != nil {
		return room.Playback{}, errs.Wrap("s.d.Playback.Playback", err)
	}
	if current.Version() != in.Version {
		return room.Playback{}, room.ErrStalePlayback
	}

	if err = r.FollowPlayback(in.ActorID, true, p.Kind()); err != nil {
		return room.Playback{}, errs.Wrap("r.FollowPlayback", err)
	}
	if err = s.d.Rooms.Save(ctx, r); err != nil {
		return room.Playback{}, errs.Wrap("s.d.Rooms.Save", err)
	}

	if err = s.d.Playback.SetPlayback(ctx, in.RoomID, in.ActorID, in.Version, p); err != nil {
		return room.Playback{}, errs.Wrap("s.d.Playback.SetPlayback", err)
	}
	if err = s.publish(ctx, in.RoomID, Event{Kind: "room_playback", RoomID: in.RoomID, Playback: &p}); err != nil {
		return room.Playback{}, errs.Wrap("s.publish", err)
	}
	return p, nil
}

func (s *Service) SendChat(ctx context.Context, in ChatInput) (room.Chat, error) {
	if _, err := s.member(ctx, in.RoomID, in.ActorID); err != nil {
		return room.Chat{}, errs.Wrap("s.member", err)
	}
	chat, err := room.NewChat(in.RoomID, in.ActorID, in.StickerID, in.Text, s.d.Clock.Now())
	if err != nil {
		return room.Chat{}, errs.Wrap("room.NewChat", err)
	}
	if err = s.d.Chat.AppendChat(ctx, in.RoomID, chat); err != nil {
		return room.Chat{}, errs.Wrap("s.d.Chat.AppendChat", err)
	}
	members, err := s.d.Presence.Members(ctx, in.RoomID)
	if err != nil {
		return room.Chat{}, errs.Wrap("s.d.Presence.Members", err)
	}
	ids := make([]uuid.UUID, 0, len(members))
	for _, m := range members {
		ids = append(ids, m.UserID)
	}
	blocked, err := s.d.Users.Blocked(ctx, in.ActorID, ids)
	if err != nil {
		return room.Chat{}, errs.Wrap("s.d.Users.Blocked", err)
	}
	for _, m := range members {
		if !blocked[m.UserID] {
			s.d.Events.Publish(ctx, m.UserID, Event{Kind: "room_chat", RoomID: in.RoomID, UserID: in.ActorID, Chat: &chat})
		}
	}
	return chat, nil
}

func (s *Service) filteredChat(ctx context.Context, id, viewer uuid.UUID, members []Member) ([]room.Chat, []uuid.UUID, error) {
	rows, err := s.d.Chat.Chat(ctx, id)
	if err != nil {
		return nil, nil, errs.Wrap("s.d.Chat.Chat", err)
	}
	ids := make([]uuid.UUID, 0, len(rows)+len(members))
	for _, c := range rows {
		ids = append(ids, c.AuthorID())
	}
	for _, m := range members {
		ids = append(ids, m.UserID)
	}
	blocked, err := s.d.Users.Blocked(ctx, viewer, ids)
	if err != nil {
		return nil, nil, errs.Wrap("s.d.Users.Blocked", err)
	}
	result := make([]room.Chat, 0, len(rows))
	for _, c := range rows {
		if !blocked[c.AuthorID()] {
			result = append(result, c)
		}
	}
	muted := make([]uuid.UUID, 0, len(members))
	for _, m := range members {
		if blocked[m.UserID] {
			muted = append(muted, m.UserID)
		}
	}
	return result, muted, nil
}
