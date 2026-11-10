package backend

import (
	"context"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/pkg/errs"
	roomv1 "github.com/vexner67/freenet/room/api/room/v1"
)

type Rooms struct{ api roomv1.RoomServiceClient }

func NewRooms(api roomv1.RoomServiceClient) *Rooms { return &Rooms{api: api} }
func roomSettings(in model.RoomSettings) *roomv1.Settings {
	return &roomv1.Settings{Title: in.Title, Kind: in.Kind, Access: in.Access, Control: in.Control}
}
func toRoom(p *roomv1.Room) model.Room {
	s := p.GetSettings()
	result := model.Room{Friends: int(p.GetFriends()), ID: p.GetId(), OwnerID: p.GetOwnerId(), ClubID: p.GetClubId(), RoomSettings: model.RoomSettings{Title: s.GetTitle(), Kind: s.GetKind(), Access: s.GetAccess(), Control: s.GetControl()}, Permanent: p.GetPermanent(), CreatedAt: fromTimestamp(p.GetCreatedAt()), Playback: toRoomPlayback(p.GetPlayback()), MaxMembers: int(p.GetMaxMembers()), Members: make([]model.RoomMember, 0, len(p.GetMembers()))}
	for _, m := range p.GetMembers() {
		result.Members = append(result.Members, model.RoomMember{UserID: m.GetUserId(), JoinedAt: fromTimestamp(m.GetJoinedAt())})
	}
	return result
}
func toRoomPlayback(p *roomv1.Playback) model.RoomPlayback {
	result := model.RoomPlayback{Kind: p.GetKind(), MediaID: p.GetMediaId(), Playing: p.GetPlaying(), Position: p.GetPosition(), UpdatedAt: fromTimestamp(p.GetUpdatedAt()), Version: p.GetVersion(), Queue: make([]model.RoomQueueItem, 0, len(p.GetQueue()))}
	for _, q := range p.GetQueue() {
		result.Queue = append(result.Queue, model.RoomQueueItem{Kind: q.GetKind(), MediaID: q.GetMediaId(), Title: q.GetTitle()})
	}
	return result
}
func roomPlayback(p model.RoomPlayback) *roomv1.Playback {
	result := &roomv1.Playback{Kind: p.Kind, MediaId: p.MediaID, Playing: p.Playing, Position: p.Position, Version: p.Version}
	for _, q := range p.Queue {
		result.Queue = append(result.Queue, &roomv1.QueueItem{Kind: q.Kind, MediaId: q.MediaID, Title: q.Title})
	}
	return result
}
func toRoomChat(c *roomv1.Chat) model.RoomChat {
	return model.RoomChat{ID: c.GetId(), RoomID: c.GetRoomId(), AuthorID: c.GetAuthorId(), Text: c.GetText(), StickerID: c.GetStickerId(), CreatedAt: fromTimestamp(c.GetCreatedAt())}
}
func toRoomMedia(m *roomv1.MediaToken) model.RoomMedia {
	return model.RoomMedia{URL: m.GetUrl(), Token: m.GetToken(), ExpiresAt: fromTimestamp(m.GetExpiresAt())}
}
func (b *Rooms) Create(ctx context.Context, token string, in model.RoomDraft) (model.Room, error) {
	p, err := b.api.CreateRoom(withToken(ctx, token), &roomv1.CreateRoomRequest{Settings: roomSettings(in.RoomSettings), ClubId: in.ClubID, Permanent: in.Permanent})
	if err != nil {
		return model.Room{}, errs.Wrap("b.api.CreateRoom", fromStatus(err))
	}
	return toRoom(p.GetRoom()), nil
}
func (b *Rooms) Get(ctx context.Context, token, id string) (model.Room, error) {
	p, err := b.api.GetRoom(withToken(ctx, token), &roomv1.GetRoomRequest{RoomId: id})
	if err != nil {
		return model.Room{}, errs.Wrap("b.api.GetRoom", fromStatus(err))
	}
	return toRoom(p.GetRoom()), nil
}
func (b *Rooms) List(ctx context.Context, token string, q model.RoomQuery) (model.RoomPage, error) {
	var rows []*roomv1.Room
	var next string
	ctx = withToken(ctx, token)
	if q.OwnerID != "" {
		p, err := b.api.ListUserRooms(ctx, &roomv1.ListUserRoomsRequest{UserId: q.OwnerID, Cursor: q.Cursor, Limit: toLimit(q.Limit)})
		if err != nil {
			return model.RoomPage{}, errs.Wrap("b.api.ListUserRooms", fromStatus(err))
		}
		rows, next = p.GetRooms(), p.GetNextCursor()
	} else if q.ClubID != "" {
		p, err := b.api.ListClubRooms(ctx, &roomv1.ListClubRoomsRequest{ClubId: q.ClubID, Cursor: q.Cursor, Limit: toLimit(q.Limit)})
		if err != nil {
			return model.RoomPage{}, errs.Wrap("b.api.ListClubRooms", fromStatus(err))
		}
		rows, next = p.GetRooms(), p.GetNextCursor()
	} else {
		p, err := b.api.ListLiveRooms(ctx, &roomv1.ListLiveRoomsRequest{Cursor: q.Cursor, Limit: toLimit(q.Limit)})
		if err != nil {
			return model.RoomPage{}, errs.Wrap("b.api.ListLiveRooms", fromStatus(err))
		}
		rows, next = p.GetRooms(), p.GetNextCursor()
	}
	result := model.RoomPage{Rooms: make([]model.Room, 0, len(rows)), NextCursor: next}
	for _, r := range rows {
		result.Rooms = append(result.Rooms, toRoom(r))
	}
	return result, nil
}
func (b *Rooms) Update(ctx context.Context, token, id string, in model.RoomSettings) (model.Room, error) {
	p, err := b.api.UpdateRoom(withToken(ctx, token), &roomv1.UpdateRoomRequest{RoomId: id, Settings: roomSettings(in)})
	if err != nil {
		return model.Room{}, errs.Wrap("b.api.UpdateRoom", fromStatus(err))
	}
	return toRoom(p.GetRoom()), nil
}
func (b *Rooms) Join(ctx context.Context, token, id string) (model.RoomJoined, error) {
	p, err := b.api.JoinRoom(withToken(ctx, token), &roomv1.JoinRoomRequest{RoomId: id})
	if err != nil {
		return model.RoomJoined{}, errs.Wrap("b.api.JoinRoom", fromStatus(err))
	}
	result := model.RoomJoined{Room: toRoom(p.GetRoom()), Media: toRoomMedia(p.GetMedia()), ServerTime: fromTimestamp(p.GetServerTime()), MutedIDs: p.GetMutedIds(), Chat: make([]model.RoomChat, 0, len(p.GetChat()))}
	for _, c := range p.GetChat() {
		result.Chat = append(result.Chat, toRoomChat(c))
	}
	return result, nil
}
func (b *Rooms) Token(ctx context.Context, token, id string) (model.RoomMedia, error) {
	p, err := b.api.Token(withToken(ctx, token), &roomv1.TokenRequest{RoomId: id})
	if err != nil {
		return model.RoomMedia{}, errs.Wrap("b.api.Token", fromStatus(err))
	}
	return toRoomMedia(p.GetMedia()), nil
}
func (b *Rooms) Chat(ctx context.Context, token, id, text, sticker string) (model.RoomChat, error) {
	p, err := b.api.SendChat(withToken(ctx, token), &roomv1.SendChatRequest{RoomId: id, Text: text, StickerId: sticker})
	if err != nil {
		return model.RoomChat{}, errs.Wrap("b.api.SendChat", fromStatus(err))
	}
	return toRoomChat(p.GetChat()), nil
}
func (b *Rooms) Playback(ctx context.Context, token, id string, in model.RoomPlayback) (model.RoomPlayback, error) {
	p, err := b.api.SetPlayback(withToken(ctx, token), &roomv1.SetPlaybackRequest{RoomId: id, Playback: roomPlayback(in)})
	if err != nil {
		return model.RoomPlayback{}, errs.Wrap("b.api.SetPlayback", fromStatus(err))
	}
	return toRoomPlayback(p.GetPlayback()), nil
}
func (b *Rooms) Current(ctx context.Context, token string, ids []string) (map[string]model.RoomSummary, error) {
	result := make(map[string]model.RoomSummary)
	if len(ids) == 0 {
		return result, nil
	}
	p, err := b.api.CurrentRooms(withToken(ctx, token), &roomv1.CurrentRoomsRequest{UserIds: ids})
	if err != nil {
		return nil, errs.Wrap("b.api.CurrentRooms", fromStatus(err))
	}
	for _, r := range p.GetRooms() {
		result[r.GetUserId()] = model.RoomSummary{ID: r.GetRoom().GetId(), Title: r.GetRoom().GetSettings().GetTitle()}
	}
	return result, nil
}
func (b *Rooms) Subscribe(ctx context.Context, token string) (model.RoomEvents, error) {
	stream, err := b.api.Subscribe(withToken(ctx, token), &roomv1.SubscribeRequest{})
	if err != nil {
		return nil, errs.Wrap("b.api.Subscribe", fromStatus(err))
	}
	return func() (model.RoomEvent, error) {
		p, err := stream.Recv()
		if err != nil {
			return model.RoomEvent{}, errs.Wrap("stream.Recv", fromStatus(err))
		}
		e := model.RoomEvent{Kind: p.GetKind(), RoomID: p.GetRoomId(), UserID: p.GetUserId()}
		if p.GetRoom() != nil {
			r := toRoom(p.GetRoom())
			e.Room = &r
		}
		if p.GetChat() != nil {
			c := toRoomChat(p.GetChat())
			e.Chat = &c
		}
		if p.GetPlayback() != nil {
			v := toRoomPlayback(p.GetPlayback())
			e.Playback = &v
		}
		return e, nil
	}, nil
}

func (b *Rooms) Close(ctx context.Context, token, id string) error {
	_, err := b.api.CloseRoom(withToken(ctx, token), &roomv1.CloseRoomRequest{RoomId: id})
	return errs.Wrap("b.api.CloseRoom", fromStatus(err))
}

func (b *Rooms) Leave(ctx context.Context, token, id string) error {
	_, err := b.api.LeaveRoom(withToken(ctx, token), &roomv1.LeaveRoomRequest{RoomId: id})
	return errs.Wrap("b.api.LeaveRoom", fromStatus(err))
}

func (b *Rooms) Heartbeat(ctx context.Context, token, id string) error {
	_, err := b.api.Heartbeat(withToken(ctx, token), &roomv1.HeartbeatRequest{RoomId: id})
	return errs.Wrap("b.api.Heartbeat", fromStatus(err))
}

func (b *Rooms) Kick(ctx context.Context, token, id, user string) error {
	_, err := b.api.KickMember(withToken(ctx, token), &roomv1.KickMemberRequest{RoomId: id, UserId: user})
	return errs.Wrap("b.api.KickMember", fromStatus(err))
}

func (b *Rooms) Mute(ctx context.Context, token, id, user string) error {
	_, err := b.api.MuteMember(withToken(ctx, token), &roomv1.MuteMemberRequest{RoomId: id, UserId: user})
	return errs.Wrap("b.api.MuteMember", fromStatus(err))
}

func (b *Rooms) Invite(ctx context.Context, token, id, user string) error {
	_, err := b.api.Invite(withToken(ctx, token), &roomv1.InviteRequest{RoomId: id, UserId: user})
	return errs.Wrap("b.api.Invite", fromStatus(err))
}

func (b *Rooms) AuthorizeMedia(ctx context.Context, token string) error {
	_, err := b.api.AuthorizeMedia(ctx, &roomv1.AuthorizeMediaRequest{Token: token})
	return errs.Wrap("b.api.AuthorizeMedia", fromStatus(err))
}
