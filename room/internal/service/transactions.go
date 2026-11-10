package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/room/internal/room"
)

// Membership and durable access changes share the repository transaction's
// lock. A join cannot race a ban or a room's closure across replicas.
func (s *Service) JoinRoom(ctx context.Context, id, actor uuid.UUID) (Joined, error) {
	var result Joined
	err := s.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		var err error
		result, err = s.joinRoom(ctx, id, actor)
		return errs.Wrap("s.joinRoom", err)
	})
	if s.d.Observer != nil {
		s.d.Observer.Join(err)
	}
	return result, errs.Wrap("s.d.Tx.WithinTx", err)
}
func (s *Service) KickMember(ctx context.Context, in MemberInput) error {
	if err := s.d.Tx.WithinTx(ctx, func(ctx context.Context) error { return errs.Wrap("s.kickMember", s.kickMember(ctx, in)) }); err != nil {
		return errs.Wrap("s.d.Tx.WithinTx", err)
	}
	s.d.Events.Publish(ctx, in.MemberID, Event{Kind: "room_kick", RoomID: in.RoomID})
	return errs.Wrap("s.LeaveRoom", s.LeaveRoom(ctx, in.RoomID, in.MemberID))
}

func (s *Service) CloseRoom(ctx context.Context, id, actor uuid.UUID) error {
	return errs.Wrap("s.d.Tx.WithinTx", s.d.Tx.WithinTx(ctx, func(ctx context.Context) error { return errs.Wrap("s.closeRoom", s.closeRoom(ctx, id, actor)) }))
}
func (s *Service) UpdateRoom(ctx context.Context, in UpdateInput) (View, error) {
	var result View
	err := s.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		var err error
		result, err = s.updateRoom(ctx, in)
		return errs.Wrap("s.updateRoom", err)
	})
	return result, errs.Wrap("s.d.Tx.WithinTx", err)
}

func (s *Service) Invite(ctx context.Context, in MemberInput) error {
	return errs.Wrap("s.d.Tx.WithinTx", s.d.Tx.WithinTx(ctx, func(ctx context.Context) error { return errs.Wrap("s.invite", s.invite(ctx, in)) }))
}

func (s *Service) SetPlayback(ctx context.Context, in PlaybackInput) (room.Playback, error) {
	var result room.Playback
	err := s.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		var err error
		result, err = s.setPlayback(ctx, in)
		return errs.Wrap("s.setPlayback", err)
	})
	if err == nil && s.d.Observer != nil {
		s.d.Observer.Playback()
	}
	return result, errs.Wrap("s.d.Tx.WithinTx", err)
}

func (s *Service) LeaveRoom(ctx context.Context, id, actor uuid.UUID) error {
	return errs.Wrap("s.d.Tx.WithinTx", s.d.Tx.WithinTx(ctx, func(ctx context.Context) error { return errs.Wrap("s.leaveRoom", s.leaveRoom(ctx, id, actor)) }))
}
