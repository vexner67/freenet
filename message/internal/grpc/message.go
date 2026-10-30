package grpc

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	messagev1 "github.com/vexner67/freenet/message/api/message/v1"
	"github.com/vexner67/freenet/message/internal/message"
	"github.com/vexner67/freenet/message/internal/service"
	"github.com/vexner67/freenet/pkg/jwtauth"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type MessageService interface {
	Send(ctx context.Context, senderID, recipientID uuid.UUID, d service.Draft) (*message.Message, error)
	Dialogs(ctx context.Context, userID, cursor uuid.UUID, size int) (service.DialogPage, error)
	CountUnread(ctx context.Context, userID uuid.UUID) (int, error)
	History(ctx context.Context, userID, peerID, cursor uuid.UUID, size int) (service.HistoryPage, error)
	Newer(ctx context.Context, userID, peerID, after uuid.UUID, size int) (service.HistoryPage, error)
	Around(ctx context.Context, userID, peerID, id uuid.UUID, size int) (service.HistoryPage, error)
	MarkRead(ctx context.Context, userID, peerID, upTo uuid.UUID) error
	Typing(ctx context.Context, userID, peerID uuid.UUID) error
	CallSignal(ctx context.Context, userID, peerID uuid.UUID, sealed string) error
	CallServers(userID uuid.UUID, now time.Time) service.CallServers
	Delete(ctx context.Context, userID, id uuid.UUID) error
	Hide(ctx context.Context, userID, id uuid.UUID) error
	DeleteMany(ctx context.Context, userID uuid.UUID, ids []uuid.UUID, forEveryone bool) error
	Edit(ctx context.Context, userID, id uuid.UUID, rev message.Revision) (*message.Message, error)
	React(ctx context.Context, userID, id uuid.UUID, r message.Reaction) (*message.Message, error)
	SetKey(ctx context.Context, userID uuid.UUID, key message.Key) error
	Key(ctx context.Context, userID uuid.UUID) (message.Key, error)
	HasMessagesFrom(ctx context.Context, userID, peerID uuid.UUID) (bool, error)
	Subscribe(userID uuid.UUID) (<-chan service.Event, func())
	PurgeUser(ctx context.Context, userID uuid.UUID) error
}

type MessageHandler struct {
	messagev1.UnimplementedMessageServiceServer
	messageService MessageService
	chatService    ChatService
	logger         *slog.Logger
}

func NewMessageHandler(messageService MessageService, chatService ChatService, logger *slog.Logger) *MessageHandler {
	return &MessageHandler{
		messageService: messageService,
		chatService:    chatService,
		logger:         logger,
	}
}

func (h *MessageHandler) SendMessage(ctx context.Context, req *messagev1.SendMessageRequest) (*messagev1.SendMessageResponse, error) {
	senderID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	recipientID, err := parseID(req.GetRecipientId(), "invalid recipient id")
	if err != nil {
		return nil, err
	}

	draft, err := fromProtoDraft(req)
	if err != nil {
		return nil, err
	}

	if draft.Sealed, err = message.NewSealed(req.GetSealed()); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	m, err := h.messageService.Send(ctx, senderID, recipientID, draft)
	if err != nil {
		return nil, err
	}

	return &messagev1.SendMessageResponse{Message: toProtoMessage(m, false)}, nil
}

func (h *MessageHandler) ListDialogs(ctx context.Context, req *messagev1.ListDialogsRequest) (*messagev1.ListDialogsResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	cursor, err := parseOptionalID(req.GetCursor(), "invalid cursor")
	if err != nil {
		return nil, err
	}

	page, err := h.messageService.Dialogs(ctx, userID, cursor, int(req.GetLimit()))
	if err != nil {
		return nil, err
	}

	resp := &messagev1.ListDialogsResponse{
		Dialogs:    make([]*messagev1.Dialog, 0, len(page.Dialogs)),
		NextCursor: optionalID(page.NextCursor),
	}

	for _, d := range page.Dialogs {
		resp.Dialogs = append(resp.Dialogs, toProtoDialog(d, userID, req.GetTextLimit()))
	}

	return resp, nil
}

func (h *MessageHandler) CountUnreadDialogs(ctx context.Context, _ *messagev1.CountUnreadDialogsRequest) (*messagev1.CountUnreadDialogsResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	count, err := h.messageService.CountUnread(ctx, userID)
	if err != nil {
		return nil, err
	}

	// The counter of the menu is one for dialogs and chats.
	chats, err := h.chatService.CountUnread(ctx, userID)
	if err != nil {
		return nil, err
	}

	count = min(count+chats, service.MaxUnreadCount)

	return &messagev1.CountUnreadDialogsResponse{Count: int32(count)}, nil //nolint:gosec // at most service.MaxUnreadCount
}

func (h *MessageHandler) ListMessages(ctx context.Context, req *messagev1.ListMessagesRequest) (*messagev1.ListMessagesResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	peerID, err := parseID(req.GetPeerId(), "invalid peer id")
	if err != nil {
		return nil, err
	}

	page, err := h.history(ctx, userID, peerID, req)
	if err != nil {
		return nil, err
	}

	return toListMessagesResponse(page, userID, req.GetTextLimit()), nil
}

// history pages back with cursor, polls for newer messages with after, or
// opens the dialog around a message.
func (h *MessageHandler) history(ctx context.Context, userID, peerID uuid.UUID, req *messagev1.ListMessagesRequest) (service.HistoryPage, error) {
	if req.GetAround() != "" {
		if req.GetCursor() != "" || req.GetAfter() != "" {
			return service.HistoryPage{}, status.Error(codes.InvalidArgument, "around cannot be used with cursor or after")
		}

		around, err := parseID(req.GetAround(), "invalid around")
		if err != nil {
			return service.HistoryPage{}, err
		}

		return h.messageService.Around(ctx, userID, peerID, around, int(req.GetLimit()))
	}

	if req.GetAfter() == "" {
		cursor, err := parseOptionalID(req.GetCursor(), "invalid cursor")
		if err != nil {
			return service.HistoryPage{}, err
		}

		return h.messageService.History(ctx, userID, peerID, cursor, int(req.GetLimit()))
	}

	if req.GetCursor() != "" {
		return service.HistoryPage{}, status.Error(codes.InvalidArgument, "cursor and after cannot be used together")
	}

	after, err := parseID(req.GetAfter(), "invalid after")
	if err != nil {
		return service.HistoryPage{}, err
	}

	return h.messageService.Newer(ctx, userID, peerID, after, int(req.GetLimit()))
}

func (h *MessageHandler) PurgeUser(ctx context.Context, req *messagev1.PurgeUserRequest) (*emptypb.Empty, error) {
	userID, err := uuid.Parse(req.GetUserId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user id")
	}

	if err = h.messageService.PurgeUser(ctx, userID); err != nil {
		return nil, err
	}

	if err = h.chatService.PurgeUser(ctx, userID); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *MessageHandler) HasMessagesFrom(ctx context.Context, req *messagev1.HasMessagesFromRequest) (*messagev1.HasMessagesFromResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	peerID, err := parseID(req.GetPeerId(), "invalid peer id")
	if err != nil {
		return nil, err
	}

	ok, err := h.messageService.HasMessagesFrom(ctx, userID, peerID)
	if err != nil {
		return nil, err
	}

	return &messagev1.HasMessagesFromResponse{HasMessages: ok}, nil
}

// Subscribe streams the caller's events until the token expires (the
// stream's deadline), the client leaves or the server stops, or the caller
// falls behind and has to catch up from storage.
func (h *MessageHandler) Subscribe(_ *messagev1.SubscribeRequest, stream messagev1.MessageService_SubscribeServer) error {
	ctx := stream.Context()

	userID, err := currentUserID(ctx)
	if err != nil {
		return err
	}

	events, cancel := h.messageService.Subscribe(userID)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return jwtauth.StreamEnd(ctx)
		case e, ok := <-events:
			if !ok {
				return status.Error(codes.ResourceExhausted, "fell behind, subscribe again")
			}
			if err = stream.Send(toProtoEvent(e)); err != nil {
				return err
			}
		}
	}
}

func (h *MessageHandler) MarkDialogRead(ctx context.Context, req *messagev1.MarkDialogReadRequest) (*emptypb.Empty, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	peerID, err := parseID(req.GetPeerId(), "invalid peer id")
	if err != nil {
		return nil, err
	}

	upTo, err := parseID(req.GetUpToId(), "invalid message id")
	if err != nil {
		return nil, err
	}

	if err = h.messageService.MarkRead(ctx, userID, peerID, upTo); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *MessageHandler) SetTyping(ctx context.Context, req *messagev1.SetTypingRequest) (*emptypb.Empty, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	peerID, err := parseID(req.GetPeerId(), "invalid peer id")
	if err != nil {
		return nil, err
	}

	if err = h.messageService.Typing(ctx, userID, peerID); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *MessageHandler) SendCallSignal(ctx context.Context, req *messagev1.SendCallSignalRequest) (*emptypb.Empty, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	peerID, err := parseID(req.GetPeerId(), "invalid peer id")
	if err != nil {
		return nil, err
	}

	if err = h.messageService.CallSignal(ctx, userID, peerID, req.GetSealed()); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *MessageHandler) GetCallServers(ctx context.Context, _ *messagev1.GetCallServersRequest) (*messagev1.GetCallServersResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	servers := h.messageService.CallServers(userID, time.Now())

	return &messagev1.GetCallServersResponse{Urls: servers.URLs, Username: servers.Username, Credential: servers.Credential}, nil
}

func (h *MessageHandler) DeleteMessage(ctx context.Context, req *messagev1.DeleteMessageRequest) (*emptypb.Empty, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	id, err := parseID(req.GetId(), "invalid message id")
	if err != nil {
		return nil, err
	}

	if err = h.messageService.Delete(ctx, userID, id); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *MessageHandler) HideMessage(ctx context.Context, req *messagev1.HideMessageRequest) (*emptypb.Empty, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	id, err := parseID(req.GetId(), "invalid message id")
	if err != nil {
		return nil, err
	}

	if err = h.messageService.Hide(ctx, userID, id); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *MessageHandler) DeleteMessages(ctx context.Context, req *messagev1.DeleteMessagesRequest) (*emptypb.Empty, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	// Before any of them is parsed: a request may name no more.
	if len(req.GetIds()) > message.MaxBatch {
		return nil, status.Error(codes.InvalidArgument, message.ErrTooManyMessages.Error())
	}

	ids, err := parseIDs(req.GetIds(), "invalid message id")
	if err != nil {
		return nil, err
	}

	if err = h.messageService.DeleteMany(ctx, userID, ids, req.GetForEveryone()); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *MessageHandler) EditMessage(ctx context.Context, req *messagev1.EditMessageRequest) (*messagev1.EditMessageResponse, error) {
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

	if rev.Sealed, err = message.NewSealed(req.GetSealed()); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if req.BaseSealed != nil {
		base, err := message.NewSealed(req.GetBaseSealed())
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}

		rev.BaseSealed = &base
	}

	m, err := h.messageService.Edit(ctx, userID, id, rev)
	if err != nil {
		return nil, err
	}

	return &messagev1.EditMessageResponse{Message: toProtoMessage(m, false)}, nil
}

// reactionRequest is a reaction as the requests of dialogs and of chats
// name it.
type reactionRequest interface {
	GetMessageId() string
	GetReaction() string
}

// reactionOf reads who reacts, to what and how; an empty reaction takes
// theirs away.
func reactionOf(ctx context.Context, req reactionRequest) (uuid.UUID, uuid.UUID, message.Reaction, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return uuid.Nil, uuid.Nil, "", err
	}

	id, err := parseID(req.GetMessageId(), "invalid message id")
	if err != nil {
		return uuid.Nil, uuid.Nil, "", err
	}

	if req.GetReaction() == "" {
		return userID, id, "", nil
	}

	r, err := message.NewReaction(req.GetReaction())
	if err != nil {
		return uuid.Nil, uuid.Nil, "", status.Error(codes.InvalidArgument, err.Error())
	}

	return userID, id, r, nil
}

func (h *MessageHandler) ReactToMessage(ctx context.Context, req *messagev1.ReactToMessageRequest) (*emptypb.Empty, error) {
	userID, id, r, err := reactionOf(ctx, req)
	if err != nil {
		return nil, err
	}

	if _, err := h.messageService.React(ctx, userID, id, r); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *MessageHandler) SetKey(ctx context.Context, req *messagev1.SetKeyRequest) (*emptypb.Empty, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	key, err := message.NewKey(req.GetPublicKey())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if err = h.messageService.SetKey(ctx, userID, key); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *MessageHandler) GetKey(ctx context.Context, req *messagev1.GetKeyRequest) (*messagev1.GetKeyResponse, error) {
	userID, err := parseID(req.GetUserId(), "invalid user id")
	if err != nil {
		return nil, err
	}

	key, err := h.messageService.Key(ctx, userID)
	if errors.Is(err, message.ErrNoKey) {
		return &messagev1.GetKeyResponse{}, nil
	}

	if err != nil {
		return nil, err
	}

	return &messagev1.GetKeyResponse{PublicKey: key.Bytes()}, nil
}

func parseID(s, invalid string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, status.Error(codes.InvalidArgument, invalid)
	}

	return id, nil
}

// parseOptionalID reads a cursor; "" is uuid.Nil, the first page.
func parseOptionalID(s, invalid string) (uuid.UUID, error) {
	if s == "" {
		return uuid.Nil, nil
	}

	return parseID(s, invalid)
}

// currentUserID is the caller's ID from the access token verified by
// jwtauth.UnaryServerInterceptor; never trust an ID from the request body.
func currentUserID(ctx context.Context) (uuid.UUID, error) {
	identity, ok := jwtauth.IdentityFromContext(ctx)
	if !ok {
		return uuid.Nil, status.Error(codes.Unauthenticated, "missing access token")
	}

	return identity.UserID, nil
}
