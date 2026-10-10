package grpc

import (
	"context"
	"log/slog"

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
	Send(ctx context.Context, senderID, recipientID uuid.UUID, content message.Content) (*message.Message, error)
	Dialogs(ctx context.Context, userID, cursor uuid.UUID, size int) (service.DialogPage, error)
	CountUnread(ctx context.Context, userID uuid.UUID) (int, error)
	History(ctx context.Context, userID, peerID, cursor uuid.UUID, size int) (service.HistoryPage, error)
	Newer(ctx context.Context, userID, peerID, after uuid.UUID, size int) (service.HistoryPage, error)
	MarkRead(ctx context.Context, userID, peerID, upTo uuid.UUID) error
	Delete(ctx context.Context, userID, id uuid.UUID) error
	Hide(ctx context.Context, userID, id uuid.UUID) error
	Edit(ctx context.Context, userID, id uuid.UUID, text message.Text) (*message.Message, error)
	Search(ctx context.Context, userID, peerID uuid.UUID, q message.Query, cursor uuid.UUID, size int) (service.SearchPage, error)
	HasMessagesFrom(ctx context.Context, userID, peerID uuid.UUID) (bool, error)
	Subscribe(userID uuid.UUID) (<-chan service.Event, func())
	PurgeUser(ctx context.Context, userID uuid.UUID) error
}

type MessageHandler struct {
	messagev1.UnimplementedMessageServiceServer
	messageService MessageService
	logger         *slog.Logger
}

func NewMessageHandler(messageService MessageService, logger *slog.Logger) *MessageHandler {
	return &MessageHandler{
		messageService: messageService,
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

	content, err := fromProtoContent(req)
	if err != nil {
		return nil, err
	}

	m, err := h.messageService.Send(ctx, senderID, recipientID, content)
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
		resp.Dialogs = append(resp.Dialogs, toProtoDialog(d, userID))
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

	return toListMessagesResponse(page, userID), nil
}

// history pages back with cursor, or polls for newer messages with after.
func (h *MessageHandler) history(ctx context.Context, userID, peerID uuid.UUID, req *messagev1.ListMessagesRequest) (service.HistoryPage, error) {
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

func (h *MessageHandler) EditMessage(ctx context.Context, req *messagev1.EditMessageRequest) (*messagev1.EditMessageResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	id, err := parseID(req.GetId(), "invalid message id")
	if err != nil {
		return nil, err
	}

	text, err := message.NewText(req.GetText())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	m, err := h.messageService.Edit(ctx, userID, id, text)
	if err != nil {
		return nil, err
	}

	return &messagev1.EditMessageResponse{Message: toProtoMessage(m, false)}, nil
}

func (h *MessageHandler) SearchMessages(ctx context.Context, req *messagev1.SearchMessagesRequest) (*messagev1.SearchMessagesResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	peerID, err := parseOptionalID(req.GetPeerId(), "invalid peer id")
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

	page, err := h.messageService.Search(ctx, userID, peerID, query, cursor, int(req.GetLimit()))
	if err != nil {
		return nil, err
	}

	resp := &messagev1.SearchMessagesResponse{
		Messages:   make([]*messagev1.Message, 0, len(page.Messages)),
		NextCursor: optionalID(page.NextCursor),
	}
	for _, m := range page.Messages {
		resp.Messages = append(resp.Messages, toProtoMessage(m, false))
	}

	return resp, nil
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
