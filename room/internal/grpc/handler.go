package grpc

import (
	"context"
	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/jwtauth"
	roomv1 "github.com/vexner67/freenet/room/api/room/v1"
	"github.com/vexner67/freenet/room/internal/room"
	"github.com/vexner67/freenet/room/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Handler struct {
	roomv1.UnimplementedRoomServiceServer
	d Deps
}

func NewHandler(d Deps) *Handler { return &Handler{d: d} }
func viewer(ctx context.Context) uuid.UUID {
	id, _ := jwtauth.IdentityFromContext(ctx)
	return id.UserID
}
func (h *Handler) CreateRoom(ctx context.Context, req *roomv1.CreateRoomRequest) (*roomv1.CreateRoomResponse, error) {
	in, err := fromProtoSettings(req.GetSettings())
	if err != nil {
		return nil, err
	}
	clubID, err := fromProtoOptionalID(req.GetClubId())
	if err != nil {
		return nil, err
	}
	in.ClubID, in.Permanent = clubID, req.GetPermanent()
	result, err := h.d.Editor.CreateRoom(ctx, service.CreateInput{ActorID: viewer(ctx), Room: in})
	if err != nil {
		return nil, errs.Wrap("h.d.Editor.CreateRoom", err)
	}
	return &roomv1.CreateRoomResponse{Room: toProtoRoom(service.View{Room: result, MaxMembers: h.d.MaxMembers})}, nil
}
func (h *Handler) GetRoom(ctx context.Context, req *roomv1.GetRoomRequest) (*roomv1.GetRoomResponse, error) {
	id, err := fromProtoID(req.GetRoomId())
	if err != nil {
		return nil, err
	}
	result, err := h.d.Reader.GetRoom(ctx, id, viewer(ctx))
	if err != nil {
		return nil, errs.Wrap("h.d.Reader.GetRoom", err)
	}
	return &roomv1.GetRoomResponse{Room: toProtoRoom(result)}, nil
}
func (h *Handler) ListLiveRooms(ctx context.Context, req *roomv1.ListLiveRoomsRequest) (*roomv1.ListLiveRoomsResponse, error) {
	cursor, err := fromProtoOptionalID(req.GetCursor())
	if err != nil {
		return nil, err
	}
	rows, next, err := h.d.Reader.ListRooms(ctx, viewer(ctx), service.Filter{Cursor: cursor, Limit: int(req.GetLimit())}, true)
	if err != nil {
		return nil, errs.Wrap("h.d.Reader.ListRooms", err)
	}
	return &roomv1.ListLiveRoomsResponse{Rooms: toProtoRooms(rows), NextCursor: toProtoID(next)}, nil
}
func (h *Handler) ListUserRooms(ctx context.Context, req *roomv1.ListUserRoomsRequest) (*roomv1.ListUserRoomsResponse, error) {
	id, err := fromProtoID(req.GetUserId())
	if err != nil {
		return nil, err
	}
	cursor, err := fromProtoOptionalID(req.GetCursor())
	if err != nil {
		return nil, err
	}
	rows, next, err := h.d.Reader.ListRooms(ctx, viewer(ctx), service.Filter{OwnerID: id, PersonalOnly: true, PermanentOnly: true, Cursor: cursor, Limit: int(req.GetLimit())}, false)
	if err != nil {
		return nil, errs.Wrap("h.d.Reader.ListRooms", err)
	}
	return &roomv1.ListUserRoomsResponse{Rooms: toProtoRooms(rows), NextCursor: toProtoID(next)}, nil
}
func (h *Handler) ListClubRooms(ctx context.Context, req *roomv1.ListClubRoomsRequest) (*roomv1.ListClubRoomsResponse, error) {
	id, err := fromProtoID(req.GetClubId())
	if err != nil {
		return nil, err
	}
	cursor, err := fromProtoOptionalID(req.GetCursor())
	if err != nil {
		return nil, err
	}
	rows, next, err := h.d.Reader.ListRooms(ctx, viewer(ctx), service.Filter{ClubID: id, PermanentOnly: true, Cursor: cursor, Limit: int(req.GetLimit())}, false)
	if err != nil {
		return nil, errs.Wrap("h.d.Reader.ListRooms", err)
	}
	return &roomv1.ListClubRoomsResponse{Rooms: toProtoRooms(rows), NextCursor: toProtoID(next)}, nil
}
func (h *Handler) UpdateRoom(ctx context.Context, req *roomv1.UpdateRoomRequest) (*roomv1.UpdateRoomResponse, error) {
	id, err := fromProtoID(req.GetRoomId())
	if err != nil {
		return nil, err
	}
	in, err := fromProtoSettings(req.GetSettings())
	if err != nil {
		return nil, err
	}
	result, err := h.d.Editor.UpdateRoom(ctx, service.UpdateInput{ActorID: viewer(ctx), RoomID: id, Room: in})
	if err != nil {
		return nil, errs.Wrap("h.d.Editor.UpdateRoom", err)
	}
	return &roomv1.UpdateRoomResponse{Room: toProtoRoom(result)}, nil
}
func (h *Handler) JoinRoom(ctx context.Context, req *roomv1.JoinRoomRequest) (*roomv1.JoinRoomResponse, error) {
	id, err := fromProtoID(req.GetRoomId())
	if err != nil {
		return nil, err
	}
	result, err := h.d.Members.JoinRoom(ctx, id, viewer(ctx))
	if err != nil {
		return nil, errs.Wrap("h.d.Members.JoinRoom", err)
	}
	response := &roomv1.JoinRoomResponse{Room: toProtoRoom(result.View), Media: toProtoMedia(result.Media), ServerTime: timestamppb.New(result.ServerTime)}
	for _, c := range result.Chat {
		response.Chat = append(response.Chat, toProtoChat(c))
	}
	for _, id := range result.MutedIDs {
		response.MutedIds = append(response.MutedIds, id.String())
	}
	return response, nil
}
func (h *Handler) Token(ctx context.Context, req *roomv1.TokenRequest) (*roomv1.TokenResponse, error) {
	id, err := fromProtoID(req.GetRoomId())
	if err != nil {
		return nil, err
	}
	result, err := h.d.Members.Token(ctx, id, viewer(ctx))
	if err != nil {
		return nil, errs.Wrap("h.d.Members.Token", err)
	}
	return &roomv1.TokenResponse{Media: toProtoMedia(result)}, nil
}
func (h *Handler) SendChat(ctx context.Context, req *roomv1.SendChatRequest) (*roomv1.SendChatResponse, error) {
	id, err := fromProtoID(req.GetRoomId())
	if err != nil {
		return nil, err
	}
	sticker, err := fromProtoOptionalID(req.GetStickerId())
	if err != nil {
		return nil, err
	}
	var text room.ChatText
	if sticker == uuid.Nil {
		text, err = room.NewChatText(req.GetText())
		if err != nil {
			return nil, errs.Wrap("room.NewChatText", err)
		}
	} else if req.GetText() != "" {
		return nil, room.ErrInvalidChat
	}
	result, err := h.d.Content.SendChat(ctx, service.ChatInput{ActorID: viewer(ctx), RoomID: id, StickerID: sticker, Text: text})
	if err != nil {
		return nil, errs.Wrap("h.d.Content.SendChat", err)
	}
	return &roomv1.SendChatResponse{Chat: toProtoChat(result)}, nil
}
func (h *Handler) SetPlayback(ctx context.Context, req *roomv1.SetPlaybackRequest) (*roomv1.SetPlaybackResponse, error) {
	id, err := fromProtoID(req.GetRoomId())
	if err != nil {
		return nil, err
	}
	in, err := fromProtoPlayback(req.GetPlayback())
	if err != nil {
		return nil, err
	}
	result, err := h.d.Content.SetPlayback(ctx, service.PlaybackInput{ActorID: viewer(ctx), RoomID: id, Version: req.GetPlayback().GetVersion(), Playback: in})
	if err != nil {
		return nil, errs.Wrap("h.d.Content.SetPlayback", err)
	}
	return &roomv1.SetPlaybackResponse{Playback: toProtoPlayback(result)}, nil
}
func memberInput(ctx context.Context, id, user string) (service.MemberInput, error) {
	roomID, err := fromProtoID(id)
	if err != nil {
		return service.MemberInput{}, err
	}
	userID, err := fromProtoID(user)
	if err != nil {
		return service.MemberInput{}, err
	}
	return service.MemberInput{ActorID: viewer(ctx), RoomID: roomID, MemberID: userID}, nil
}
func (h *Handler) CurrentRooms(ctx context.Context, req *roomv1.CurrentRoomsRequest) (*roomv1.CurrentRoomsResponse, error) {
	if len(req.GetUserIds()) > 100 {
		return nil, room.ErrInvalidID
	}
	ids := make([]uuid.UUID, 0, len(req.GetUserIds()))
	for _, value := range req.GetUserIds() {
		id, err := fromProtoID(value)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	rows, err := h.d.Reader.CurrentRooms(ctx, viewer(ctx), ids)
	if err != nil {
		return nil, errs.Wrap("h.d.Reader.CurrentRooms", err)
	}
	result := &roomv1.CurrentRoomsResponse{}
	for _, id := range ids {
		if r, ok := rows[id]; ok {
			result.Rooms = append(result.Rooms, &roomv1.CurrentRoom{UserId: id.String(), Room: toProtoRoom(r)})
		}
	}
	return result, nil
}
func (h *Handler) Subscribe(_ *roomv1.SubscribeRequest, stream roomv1.RoomService_SubscribeServer) error {
	ctx := stream.Context()
	id := viewer(ctx)
	if id == uuid.Nil {
		return status.Error(codes.Unauthenticated, "missing access token")
	}
	events, cancel := h.d.Events.Subscribe(id)
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return jwtauth.StreamEnd(ctx)
		case event, ok := <-events:
			if !ok {
				return status.Error(codes.Unavailable, "room events lost, rejoin room")
			}
			if err := stream.Send(toProtoEvent(event)); err != nil {
				return status.Error(codes.Unavailable, "room stream closed")
			}
		}
	}
}

func (h *Handler) CloseRoom(ctx context.Context, req *roomv1.CloseRoomRequest) (*roomv1.CloseRoomResponse, error) {
	id, err := fromProtoID(req.GetRoomId())
	if err != nil {
		return nil, err
	}
	if err = h.d.Editor.CloseRoom(ctx, id, viewer(ctx)); err != nil {
		return nil, errs.Wrap("h.d.Editor.CloseRoom", err)
	}
	return &roomv1.CloseRoomResponse{}, nil
}

func (h *Handler) LeaveRoom(ctx context.Context, req *roomv1.LeaveRoomRequest) (*roomv1.LeaveRoomResponse, error) {
	id, err := fromProtoID(req.GetRoomId())
	if err != nil {
		return nil, err
	}
	if err = h.d.Members.LeaveRoom(ctx, id, viewer(ctx)); err != nil {
		return nil, errs.Wrap("h.d.Members.LeaveRoom", err)
	}
	return &roomv1.LeaveRoomResponse{}, nil
}

func (h *Handler) Heartbeat(ctx context.Context, req *roomv1.HeartbeatRequest) (*roomv1.HeartbeatResponse, error) {
	id, err := fromProtoID(req.GetRoomId())
	if err != nil {
		return nil, err
	}
	if err = h.d.Members.Heartbeat(ctx, id, viewer(ctx)); err != nil {
		return nil, errs.Wrap("h.d.Members.Heartbeat", err)
	}
	return &roomv1.HeartbeatResponse{}, nil
}

func (h *Handler) PurgeUser(ctx context.Context, req *roomv1.PurgeUserRequest) (*roomv1.PurgeUserResponse, error) {
	id, err := fromProtoID(req.GetUserId())
	if err != nil {
		return nil, err
	}
	if err = h.d.Cleaner.PurgeUser(ctx, id); err != nil {
		return nil, errs.Wrap("h.d.Cleaner.PurgeUser", err)
	}
	return &roomv1.PurgeUserResponse{}, nil
}

func (h *Handler) PurgeClub(ctx context.Context, req *roomv1.PurgeClubRequest) (*roomv1.PurgeClubResponse, error) {
	id, err := fromProtoID(req.GetClubId())
	if err != nil {
		return nil, err
	}
	if err = h.d.Cleaner.PurgeClub(ctx, id); err != nil {
		return nil, errs.Wrap("h.d.Cleaner.PurgeClub", err)
	}
	return &roomv1.PurgeClubResponse{}, nil
}

func (h *Handler) KickMember(ctx context.Context, req *roomv1.KickMemberRequest) (*roomv1.KickMemberResponse, error) {
	in, err := memberInput(ctx, req.GetRoomId(), req.GetUserId())
	if err != nil {
		return nil, err
	}
	if err = h.d.Management.KickMember(ctx, in); err != nil {
		return nil, errs.Wrap("h.d.Management.KickMember", err)
	}
	return &roomv1.KickMemberResponse{}, nil
}

func (h *Handler) MuteMember(ctx context.Context, req *roomv1.MuteMemberRequest) (*roomv1.MuteMemberResponse, error) {
	in, err := memberInput(ctx, req.GetRoomId(), req.GetUserId())
	if err != nil {
		return nil, err
	}
	if err = h.d.Management.MuteMember(ctx, in); err != nil {
		return nil, errs.Wrap("h.d.Management.MuteMember", err)
	}
	return &roomv1.MuteMemberResponse{}, nil
}

func (h *Handler) Invite(ctx context.Context, req *roomv1.InviteRequest) (*roomv1.InviteResponse, error) {
	in, err := memberInput(ctx, req.GetRoomId(), req.GetUserId())
	if err != nil {
		return nil, err
	}
	if err = h.d.Management.Invite(ctx, in); err != nil {
		return nil, errs.Wrap("h.d.Management.Invite", err)
	}
	return &roomv1.InviteResponse{}, nil
}

func (h *Handler) AuthorizeMedia(ctx context.Context, req *roomv1.AuthorizeMediaRequest) (*roomv1.AuthorizeMediaResponse, error) {
	if err := h.d.MediaAccess.AuthorizeMedia(ctx, req.GetToken()); err != nil {
		return nil, errs.Wrap("h.d.MediaAccess.AuthorizeMedia", err)
	}
	return &roomv1.AuthorizeMediaResponse{}, nil
}
