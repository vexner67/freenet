package grpc

import (
	"bytes"
	"context"

	"github.com/google/uuid"
	messagev1 "github.com/vexner67/freenet/message/api/message/v1"
	"github.com/vexner67/freenet/message/internal/message"
	"github.com/vexner67/freenet/message/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ChatService is the chats of several people.
type ChatService interface {
	Create(ctx context.Context, ownerID uuid.UUID, title message.Title, memberIDs []uuid.UUID) (message.ChatView, error)
	Get(ctx context.Context, userID, chatID uuid.UUID) (service.ChatDetails, error)
	Chats(ctx context.Context, userID, cursor uuid.UUID, size int) (service.ChatPage, error)
	CountUnread(ctx context.Context, userID uuid.UUID) (int, error)
	Rename(ctx context.Context, userID, chatID uuid.UUID, title message.Title) error
	Pin(ctx context.Context, userID, chatID, id uuid.UUID) error
	Unpin(ctx context.Context, userID, chatID uuid.UUID) error
	SetPhoto(ctx context.Context, userID, chatID, photoID uuid.UUID) error
	Invite(ctx context.Context, userID, chatID uuid.UUID, ids []uuid.UUID) error
	Kick(ctx context.Context, userID, chatID, targetID uuid.UUID) error
	Leave(ctx context.Context, userID, chatID uuid.UUID) error
	Return(ctx context.Context, userID, chatID uuid.UUID) error
	Send(ctx context.Context, senderID, chatID uuid.UUID, d service.Draft) (*message.ChatMessage, error)
	Message(ctx context.Context, userID, id uuid.UUID) (*message.ChatMessage, message.Member, error)
	Edit(ctx context.Context, userID, id uuid.UUID, rev message.Revision) (*message.ChatMessage, error)
	Delete(ctx context.Context, userID, id uuid.UUID) error
	History(ctx context.Context, userID, chatID, cursor uuid.UUID, size int) (service.ChatHistoryPage, error)
	Newer(ctx context.Context, userID, chatID, after uuid.UUID, size int) (service.ChatHistoryPage, error)
	Around(ctx context.Context, userID, chatID, id uuid.UUID, size int) (service.ChatHistoryPage, error)
	Search(ctx context.Context, userID, chatID uuid.UUID, q message.Query, cursor uuid.UUID, size int) (service.ChatSearchPage, error)
	MarkRead(ctx context.Context, userID, chatID, upTo uuid.UUID) error
	Typing(ctx context.Context, userID, chatID uuid.UUID) error
	PurgeUser(ctx context.Context, userID uuid.UUID) error
}

// maxInvited bounds how many people one request names: a chat holds no
// more.
const maxInvited = message.MaxChatMembers - 1

// chatCaller reads who calls and which chat they mean.
func chatCaller(ctx context.Context, chatID string) (uuid.UUID, uuid.UUID, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	id, err := parseID(chatID, "invalid chat id")
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	return userID, id, nil
}

// invitedIDs reads the people a request names.
func invitedIDs(raw []string) ([]uuid.UUID, error) {
	if len(raw) > maxInvited {
		return nil, status.Error(codes.InvalidArgument, message.ErrChatFull.Error())
	}

	return parseIDs(raw, "invalid user id")
}

func (h *MessageHandler) CreateChat(ctx context.Context, req *messagev1.CreateChatRequest) (*messagev1.CreateChatResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	title, err := message.NewTitle(req.GetTitle())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	ids, err := invitedIDs(req.GetMemberIds())
	if err != nil {
		return nil, err
	}

	view, err := h.chatService.Create(ctx, userID, title, ids)
	if err != nil {
		return nil, err
	}

	return &messagev1.CreateChatResponse{Chat: toProtoChat(view, 0)}, nil
}

func (h *MessageHandler) GetChat(ctx context.Context, req *messagev1.GetChatRequest) (*messagev1.GetChatResponse, error) {
	userID, chatID, err := chatCaller(ctx, req.GetChatId())
	if err != nil {
		return nil, err
	}

	details, err := h.chatService.Get(ctx, userID, chatID)
	if err != nil {
		return nil, err
	}

	resp := &messagev1.GetChatResponse{
		Chat:    toProtoChat(details.View, 0),
		Members: make([]*messagev1.ChatMember, 0, len(details.Members)),
	}

	if details.Pinned != nil {
		resp.Pinned = shownChatMessage(details.Pinned, details.View.Me, 0)
	}

	for _, m := range details.Members {
		resp.Members = append(resp.Members, &messagev1.ChatMember{UserId: m.UserID.String(), JoinedAt: timestamppb.New(m.JoinedAt)})
	}

	return resp, nil
}

func (h *MessageHandler) ListChats(ctx context.Context, req *messagev1.ListChatsRequest) (*messagev1.ListChatsResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	cursor, err := parseOptionalID(req.GetCursor(), "invalid cursor")
	if err != nil {
		return nil, err
	}

	page, err := h.chatService.Chats(ctx, userID, cursor, int(req.GetLimit()))
	if err != nil {
		return nil, err
	}

	resp := &messagev1.ListChatsResponse{
		Chats:      make([]*messagev1.Chat, 0, len(page.Chats)),
		NextCursor: optionalID(page.NextCursor),
	}

	for _, view := range page.Chats {
		resp.Chats = append(resp.Chats, toProtoChat(view, req.GetTextLimit()))
	}

	return resp, nil
}

func (h *MessageHandler) RenameChat(ctx context.Context, req *messagev1.RenameChatRequest) (*emptypb.Empty, error) {
	userID, chatID, err := chatCaller(ctx, req.GetChatId())
	if err != nil {
		return nil, err
	}

	title, err := message.NewTitle(req.GetTitle())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if err = h.chatService.Rename(ctx, userID, chatID, title); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *MessageHandler) PinChatMessage(ctx context.Context, req *messagev1.PinChatMessageRequest) (*emptypb.Empty, error) {
	userID, chatID, err := chatCaller(ctx, req.GetChatId())
	if err != nil {
		return nil, err
	}

	id, err := parseID(req.GetMessageId(), "invalid message id")
	if err != nil {
		return nil, err
	}

	if err = h.chatService.Pin(ctx, userID, chatID, id); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *MessageHandler) UnpinChatMessage(ctx context.Context, req *messagev1.UnpinChatMessageRequest) (*emptypb.Empty, error) {
	userID, chatID, err := chatCaller(ctx, req.GetChatId())
	if err != nil {
		return nil, err
	}

	if err = h.chatService.Unpin(ctx, userID, chatID); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *MessageHandler) SetChatPhoto(ctx context.Context, req *messagev1.SetChatPhotoRequest) (*emptypb.Empty, error) {
	userID, chatID, err := chatCaller(ctx, req.GetChatId())
	if err != nil {
		return nil, err
	}

	photoID, err := parseOptionalID(req.GetPhotoId(), "invalid photo id")
	if err != nil {
		return nil, err
	}

	if err = h.chatService.SetPhoto(ctx, userID, chatID, photoID); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *MessageHandler) AddChatMembers(ctx context.Context, req *messagev1.AddChatMembersRequest) (*emptypb.Empty, error) {
	userID, chatID, err := chatCaller(ctx, req.GetChatId())
	if err != nil {
		return nil, err
	}

	ids, err := invitedIDs(req.GetUserIds())
	if err != nil {
		return nil, err
	}

	if err = h.chatService.Invite(ctx, userID, chatID, ids); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *MessageHandler) RemoveChatMember(ctx context.Context, req *messagev1.RemoveChatMemberRequest) (*emptypb.Empty, error) {
	userID, chatID, err := chatCaller(ctx, req.GetChatId())
	if err != nil {
		return nil, err
	}

	targetID, err := parseID(req.GetUserId(), "invalid user id")
	if err != nil {
		return nil, err
	}

	if err = h.chatService.Kick(ctx, userID, chatID, targetID); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *MessageHandler) LeaveChat(ctx context.Context, req *messagev1.LeaveChatRequest) (*emptypb.Empty, error) {
	userID, chatID, err := chatCaller(ctx, req.GetChatId())
	if err != nil {
		return nil, err
	}

	if err = h.chatService.Leave(ctx, userID, chatID); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *MessageHandler) ReturnToChat(ctx context.Context, req *messagev1.ReturnToChatRequest) (*emptypb.Empty, error) {
	userID, chatID, err := chatCaller(ctx, req.GetChatId())
	if err != nil {
		return nil, err
	}

	if err = h.chatService.Return(ctx, userID, chatID); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *MessageHandler) SendChatMessage(ctx context.Context, req *messagev1.SendChatMessageRequest) (*messagev1.SendChatMessageResponse, error) {
	userID, chatID, err := chatCaller(ctx, req.GetChatId())
	if err != nil {
		return nil, err
	}

	draft, err := fromProtoDraft(req)
	if err != nil {
		return nil, err
	}

	m, err := h.chatService.Send(ctx, userID, chatID, draft)
	if err != nil {
		return nil, err
	}

	return &messagev1.SendChatMessageResponse{Message: toProtoChatMessage(m)}, nil
}

func (h *MessageHandler) ListChatMessages(ctx context.Context, req *messagev1.ListChatMessagesRequest) (*messagev1.ListChatMessagesResponse, error) {
	userID, chatID, err := chatCaller(ctx, req.GetChatId())
	if err != nil {
		return nil, err
	}

	page, err := h.chatHistory(ctx, userID, chatID, req)
	if err != nil {
		return nil, err
	}

	resp := &messagev1.ListChatMessagesResponse{
		Messages:   make([]*messagev1.ChatMessage, 0, len(page.Messages)),
		NextCursor: optionalID(page.NextCursor),
	}

	for _, m := range page.Messages {
		resp.Messages = append(resp.Messages, shownChatMessage(m, page.Me, req.GetTextLimit()))
	}

	return resp, nil
}

// chatHistory pages back with cursor, polls for newer messages with after
// or opens the chat around a message.
func (h *MessageHandler) chatHistory(ctx context.Context, userID, chatID uuid.UUID, req *messagev1.ListChatMessagesRequest) (service.ChatHistoryPage, error) {
	if req.GetAround() != "" {
		if req.GetCursor() != "" || req.GetAfter() != "" {
			return service.ChatHistoryPage{}, status.Error(codes.InvalidArgument, "around cannot be used with cursor or after")
		}

		around, err := parseID(req.GetAround(), "invalid around")
		if err != nil {
			return service.ChatHistoryPage{}, err
		}

		return h.chatService.Around(ctx, userID, chatID, around, int(req.GetLimit()))
	}

	if req.GetAfter() == "" {
		cursor, err := parseOptionalID(req.GetCursor(), "invalid cursor")
		if err != nil {
			return service.ChatHistoryPage{}, err
		}

		return h.chatService.History(ctx, userID, chatID, cursor, int(req.GetLimit()))
	}

	if req.GetCursor() != "" {
		return service.ChatHistoryPage{}, status.Error(codes.InvalidArgument, "cursor and after cannot be used together")
	}

	after, err := parseID(req.GetAfter(), "invalid after")
	if err != nil {
		return service.ChatHistoryPage{}, err
	}

	return h.chatService.Newer(ctx, userID, chatID, after, int(req.GetLimit()))
}

func (h *MessageHandler) GetChatMessage(ctx context.Context, req *messagev1.GetChatMessageRequest) (*messagev1.GetChatMessageResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	id, err := parseID(req.GetId(), "invalid message id")
	if err != nil {
		return nil, err
	}

	m, me, err := h.chatService.Message(ctx, userID, id)
	if err != nil {
		return nil, err
	}

	return &messagev1.GetChatMessageResponse{Message: shownChatMessage(m, me, 0)}, nil
}

func (h *MessageHandler) EditChatMessage(ctx context.Context, req *messagev1.EditChatMessageRequest) (*messagev1.EditChatMessageResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	id, err := parseID(req.GetId(), "invalid message id")
	if err != nil {
		return nil, err
	}

	rev, err := fromProtoRevision(req, req.BaseText)
	if err != nil {
		return nil, err
	}

	m, err := h.chatService.Edit(ctx, userID, id, rev)
	if err != nil {
		return nil, err
	}

	return &messagev1.EditChatMessageResponse{Message: toProtoChatMessage(m)}, nil
}

func (h *MessageHandler) SearchChatMessages(ctx context.Context, req *messagev1.SearchChatMessagesRequest) (*messagev1.SearchChatMessagesResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	chatID, err := parseOptionalID(req.GetChatId(), "invalid chat id")
	if err != nil {
		return nil, err
	}

	query, err := message.NewQuery(req.GetQuery())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	cursor, err := parseOptionalID(req.GetCursor(), "invalid cursor")
	if err != nil {
		return nil, err
	}

	page, err := h.chatService.Search(ctx, userID, chatID, query, cursor, int(req.GetLimit()))
	if err != nil {
		return nil, err
	}

	resp := &messagev1.SearchChatMessagesResponse{
		Messages:   make([]*messagev1.ChatMessage, 0, len(page.Messages)),
		NextCursor: optionalID(page.NextCursor),
	}

	for _, m := range page.Messages {
		pm := toProtoChatMessage(m)
		pm.Text, pm.TextCut = m.Text().ExcerptAround(query.String(), int(req.GetTextLimit()))

		for i, f := range m.Forwarded() {
			text, cut := f.Text.ExcerptAround(query.String(), int(req.GetTextLimit()))
			pm.Forwarded[i].Text = text
			pm.TextCut = pm.TextCut || cut
		}

		resp.Messages = append(resp.Messages, pm)
	}

	return resp, nil
}

func (h *MessageHandler) DeleteChatMessage(ctx context.Context, req *messagev1.DeleteChatMessageRequest) (*emptypb.Empty, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	id, err := parseID(req.GetId(), "invalid message id")
	if err != nil {
		return nil, err
	}

	if err = h.chatService.Delete(ctx, userID, id); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *MessageHandler) MarkChatRead(ctx context.Context, req *messagev1.MarkChatReadRequest) (*emptypb.Empty, error) {
	userID, chatID, err := chatCaller(ctx, req.GetChatId())
	if err != nil {
		return nil, err
	}

	upTo, err := parseID(req.GetUpToId(), "invalid message id")
	if err != nil {
		return nil, err
	}

	if err = h.chatService.MarkRead(ctx, userID, chatID, upTo); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *MessageHandler) SetChatTyping(ctx context.Context, req *messagev1.SetChatTypingRequest) (*emptypb.Empty, error) {
	userID, chatID, err := chatCaller(ctx, req.GetChatId())
	if err != nil {
		return nil, err
	}

	if err = h.chatService.Typing(ctx, userID, chatID); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

// toProtoChat cuts the text of the last message to textLimit.
func toProtoChat(view message.ChatView, textLimit int32) *messagev1.Chat {
	c := &messagev1.Chat{
		Id:          view.Chat.ID().String(),
		Title:       view.Chat.Title().String(),
		OwnerId:     view.Chat.OwnerID().String(),
		CreatedAt:   timestamppb.New(view.Chat.CreatedAt()),
		MemberCount: int32(view.Members), //nolint:gosec // at most message.MaxChatMembers
		State:       toProtoMemberState(view.Me.State),
		Unread:      view.Me.HasUnread(),
		PhotoId:     optionalID(view.Chat.PhotoID()),
	}

	if view.LastMessage != nil {
		c.LastMessage = shownChatMessage(view.LastMessage, view.Me, textLimit)
	}

	return c
}

func toProtoMemberState(s message.MemberState) messagev1.ChatMemberState {
	switch s {
	case message.MemberActive:
		return messagev1.ChatMemberState_CHAT_MEMBER_STATE_ACTIVE
	case message.MemberLeft:
		return messagev1.ChatMemberState_CHAT_MEMBER_STATE_LEFT
	case message.MemberKicked:
		return messagev1.ChatMemberState_CHAT_MEMBER_STATE_KICKED
	default:
		return messagev1.ChatMemberState_CHAT_MEMBER_STATE_UNSPECIFIED
	}
}

func toProtoChatAction(a message.Action) messagev1.ChatAction {
	switch a {
	case message.ActionCreate:
		return messagev1.ChatAction_CHAT_ACTION_CREATE
	case message.ActionTitle:
		return messagev1.ChatAction_CHAT_ACTION_TITLE
	case message.ActionInvite:
		return messagev1.ChatAction_CHAT_ACTION_INVITE
	case message.ActionKick:
		return messagev1.ChatAction_CHAT_ACTION_KICK
	case message.ActionLeave:
		return messagev1.ChatAction_CHAT_ACTION_LEAVE
	case message.ActionReturn:
		return messagev1.ChatAction_CHAT_ACTION_RETURN
	case message.ActionPin:
		return messagev1.ChatAction_CHAT_ACTION_PIN
	case message.ActionUnpin:
		return messagev1.ChatAction_CHAT_ACTION_UNPIN
	case message.ActionPhoto:
		return messagev1.ChatAction_CHAT_ACTION_PHOTO
	case message.ActionPhotoRemove:
		return messagev1.ChatAction_CHAT_ACTION_PHOTO_REMOVE
	default:
		return messagev1.ChatAction_CHAT_ACTION_NONE
	}
}

func toProtoChatMessage(m *message.ChatMessage) *messagev1.ChatMessage {
	pm := &messagev1.ChatMessage{
		Id:          m.ID().String(),
		ChatId:      m.ChatID().String(),
		SenderId:    m.SenderID().String(),
		Text:        m.Text().String(),
		PhotoIds:    photoStrings(m.Photos()),
		Action:      toProtoChatAction(m.Action()),
		TargetId:    optionalID(m.TargetID()),
		CreatedAt:   timestamppb.New(m.CreatedAt()),
		PostId:      postString(m.Post()),
		PhotoId:     postString(m.SharedPhoto()),
		Forwarded:   toProtoForwarded(m.Forwarded()),
		Sticker:     m.Sticker().String(),
		DocumentIds: idStrings(m.Documents()),
	}

	if m.IsDeleted() {
		pm.DeletedAt = timestamppb.New(m.DeletedAt())

		return pm
	}

	// What happened in the chat is neither edited nor taken back.
	if m.IsAction() {
		return pm
	}

	pm.DeletableUntil = timestamppb.New(m.DeletableUntil())
	pm.EditableUntil = timestamppb.New(m.EditableUntil())

	if !m.EditedAt().IsZero() {
		pm.EditedAt = timestamppb.New(m.EditedAt())
	}

	return pm
}

// shownChatMessage is a message as me has it: unread while it is someone
// else's and above their mark, its text and those it carries on cut to
// textLimit. The title an action tells of is short and stays whole.
func shownChatMessage(m *message.ChatMessage, me message.Member, textLimit int32) *messagev1.ChatMessage {
	pm := toProtoChatMessage(m)
	pm.Text, pm.TextCut = m.Text().Excerpt(int(textLimit))

	for i, f := range m.Forwarded() {
		text, cut := f.Text.Excerpt(int(textLimit))
		pm.Forwarded[i].Text = text
		pm.TextCut = pm.TextCut || cut
	}

	id := m.ID()
	pm.Unread = m.SenderID() != me.UserID && bytes.Compare(id[:], me.ReadUpTo[:]) > 0

	return pm
}

// toProtoChatEvent is what happened in a chat as the stream tells it.
func toProtoChatEvent(e *service.ChatEvent) *messagev1.SubscribeResponse {
	switch {
	case e.Read != nil:
		return &messagev1.SubscribeResponse{Event: &messagev1.SubscribeResponse_ChatRead{ChatRead: &messagev1.ChatRead{
			ChatId: e.Read.ChatID.String(),
			UpToId: e.Read.UpTo.String(),
		}}}
	case e.Edited != nil:
		return &messagev1.SubscribeResponse{Event: &messagev1.SubscribeResponse_ChatEdited{ChatEdited: toProtoChatMessage(e.Edited)}}
	default:
		return &messagev1.SubscribeResponse{Event: &messagev1.SubscribeResponse_ChatMessage{ChatMessage: toProtoChatMessage(e.Message)}}
	}
}
