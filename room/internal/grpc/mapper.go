package grpc

import (
	"errors"
	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	roomv1 "github.com/vexner67/freenet/room/api/room/v1"
	"github.com/vexner67/freenet/room/internal/room"
	"github.com/vexner67/freenet/room/internal/service"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func fromProtoID(value string) (uuid.UUID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return uuid.Nil, errors.Join(room.ErrInvalidID, errs.Wrap("uuid.Parse", err))
	}
	if id == uuid.Nil {
		return uuid.Nil, room.ErrInvalidID
	}
	return id, nil
}
func fromProtoOptionalID(value string) (uuid.UUID, error) {
	if value == "" {
		return uuid.Nil, nil
	}
	return fromProtoID(value)
}
func toProtoID(id uuid.UUID) string {
	if id == uuid.Nil {
		return ""
	}
	return id.String()
}
func fromProtoSettings(p *roomv1.Settings) (room.Input, error) {
	title, err := room.NewTitle(p.GetTitle())
	if err != nil {
		return room.Input{}, errs.Wrap("room.NewTitle", err)
	}
	kind, err := room.NewKind(p.GetKind())
	if err != nil {
		return room.Input{}, errs.Wrap("room.NewKind", err)
	}
	access, err := room.NewAccess(p.GetAccess())
	if err != nil {
		return room.Input{}, errs.Wrap("room.NewAccess", err)
	}
	control, err := room.NewControl(p.GetControl())
	if err != nil {
		return room.Input{}, errs.Wrap("room.NewControl", err)
	}
	return room.Input{Title: title, Kind: kind, Access: access, Control: control}, nil
}
func toProtoRoom(view service.View) *roomv1.Room {
	r := view.Room
	if r == nil {
		return nil
	}
	p := &roomv1.Room{Id: r.ID().String(), OwnerId: r.OwnerID().String(), ClubId: toProtoID(r.ClubID()), Settings: &roomv1.Settings{Title: r.Title().String(), Kind: r.Kind().String(), Access: r.Access().String(), Control: r.Control().String()}, Permanent: r.Permanent(), CreatedAt: timestamppb.New(r.CreatedAt()), Playback: toProtoPlayback(view.Playback), Friends: int32(view.Friends), MaxMembers: int32(view.MaxMembers)}
	for _, m := range view.Members {
		p.Members = append(p.Members, &roomv1.Member{UserId: m.UserID.String(), JoinedAt: timestamppb.New(m.JoinedAt)})
	}
	return p
}
func toProtoPlayback(p room.Playback) *roomv1.Playback {
	result := &roomv1.Playback{Kind: string(p.Kind()), MediaId: p.MediaID(), Playing: p.Playing(), Position: p.Position(), Version: p.Version()}
	if !p.UpdatedAt().IsZero() {
		result.UpdatedAt = timestamppb.New(p.UpdatedAt())
	}
	for _, q := range p.Queue() {
		result.Queue = append(result.Queue, &roomv1.QueueItem{Kind: string(q.Kind()), MediaId: q.MediaID(), Title: q.Title()})
	}
	return result
}
func fromProtoPlayback(p *roomv1.Playback) (room.PlaybackInput, error) {
	in := room.PlaybackInput{Kind: room.PlaybackKind(p.GetKind()), MediaID: p.GetMediaId(), Playing: p.GetPlaying(), Position: p.GetPosition()}
	if len(p.GetQueue()) > room.MaxQueue {
		return room.PlaybackInput{}, room.ErrInvalidPlayback
	}
	for _, q := range p.GetQueue() {
		item, err := room.NewQueueItem(room.PlaybackKind(q.GetKind()), q.GetMediaId(), q.GetTitle())
		if err != nil {
			return room.PlaybackInput{}, errs.Wrap("room.NewQueueItem", err)
		}
		in.Queue = append(in.Queue, item)
	}
	return in, nil
}
func toProtoChat(c room.Chat) *roomv1.Chat {
	return &roomv1.Chat{Id: c.ID().String(), RoomId: c.RoomID().String(), AuthorId: c.AuthorID().String(), Text: c.Text().String(), StickerId: toProtoID(c.StickerID()), CreatedAt: timestamppb.New(c.CreatedAt())}
}
func toProtoMedia(m service.MediaToken) *roomv1.MediaToken {
	return &roomv1.MediaToken{Url: m.URL, Token: m.Token, ExpiresAt: timestamppb.New(m.ExpiresAt)}
}
func toProtoEvent(e service.Event) *roomv1.SubscribeResponse {
	p := &roomv1.SubscribeResponse{Kind: e.Kind, RoomId: toProtoID(e.RoomID), UserId: toProtoID(e.UserID)}
	if e.Room != nil {
		p.Room = toProtoRoom(service.View{Room: e.Room})
	}
	if e.Chat != nil {
		p.Chat = toProtoChat(*e.Chat)
	}
	if e.Playback != nil {
		p.Playback = toProtoPlayback(*e.Playback)
	}
	return p
}
func toProtoRooms(rows []service.View) []*roomv1.Room {
	result := make([]*roomv1.Room, 0, len(rows))
	for _, row := range rows {
		result = append(result, toProtoRoom(row))
	}
	return result
}
