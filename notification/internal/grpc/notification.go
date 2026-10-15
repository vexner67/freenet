package grpc

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	notificationv1 "github.com/vexner67/freenet/notification/api/notification/v1"
	"github.com/vexner67/freenet/notification/internal/notification"
	"github.com/vexner67/freenet/notification/internal/service"
	"github.com/vexner67/freenet/pkg/jwtauth"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type NotificationService interface {
	Notify(ctx context.Context, e notification.Event) error
	Retract(ctx context.Context, e notification.Event) error
	ForgetPost(ctx context.Context, postID uuid.UUID) error
	ForgetComment(ctx context.Context, commentID uuid.UUID) error
	PurgeUser(ctx context.Context, userID uuid.UUID) error
	List(ctx context.Context, userID uuid.UUID, unread bool, cursor uuid.UUID, size int) (service.Page, error)
	CountUnread(ctx context.Context, userID uuid.UUID) (int, error)
	MarkRead(ctx context.Context, userID, notificationID uuid.UUID) error
	Subscribe(userID uuid.UUID) (<-chan service.Change, func())
}

var changeToProto = map[service.Change]notificationv1.Change{
	service.ChangeNew:  notificationv1.Change_CHANGE_NEW,
	service.ChangeGone: notificationv1.Change_CHANGE_GONE,
	service.ChangeRead: notificationv1.Change_CHANGE_READ,
}

// Subscribe streams what happens to the caller's notifications until the
// token expires (the stream's deadline), the client leaves or the server
// stops, or the caller falls behind.
func (h *NotificationHandler) Subscribe(_ *notificationv1.SubscribeRequest, stream notificationv1.NotificationService_SubscribeServer) error {
	ctx := stream.Context()

	userID, err := currentUserID(ctx)
	if err != nil {
		return err
	}

	changes, cancel := h.notificationService.Subscribe(userID)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return jwtauth.StreamEnd(ctx)
		case c, ok := <-changes:
			if !ok {
				return status.Error(codes.ResourceExhausted, "fell behind, subscribe again")
			}
			if err = stream.Send(&notificationv1.SubscribeResponse{Change: changeToProto[c]}); err != nil {
				return err
			}
		}
	}
}

type NotificationHandler struct {
	notificationv1.UnimplementedNotificationServiceServer
	notificationService NotificationService
	pushService         PushService
	logger              *slog.Logger
}

func NewNotificationHandler(notificationService NotificationService, pushService PushService, logger *slog.Logger) *NotificationHandler {
	return &NotificationHandler{
		notificationService: notificationService,
		pushService:         pushService,
		logger:              logger,
	}
}

func (h *NotificationHandler) Notify(ctx context.Context, req *notificationv1.NotifyRequest) (*emptypb.Empty, error) {
	e, err := fromProtoEvent(req.GetEvent())
	if err != nil {
		return nil, err
	}

	if err = h.notificationService.Notify(ctx, e); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *NotificationHandler) Retract(ctx context.Context, req *notificationv1.RetractRequest) (*emptypb.Empty, error) {
	e, err := fromProtoEvent(req.GetEvent())
	if err != nil {
		return nil, err
	}

	if err = h.notificationService.Retract(ctx, e); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *NotificationHandler) ForgetPost(ctx context.Context, req *notificationv1.ForgetPostRequest) (*emptypb.Empty, error) {
	postID, err := uuid.Parse(req.GetPostId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid post id")
	}

	if err = h.notificationService.ForgetPost(ctx, postID); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *NotificationHandler) ForgetComment(ctx context.Context, req *notificationv1.ForgetCommentRequest) (*emptypb.Empty, error) {
	commentID, err := uuid.Parse(req.GetCommentId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid comment id")
	}

	if err = h.notificationService.ForgetComment(ctx, commentID); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *NotificationHandler) PurgeUser(ctx context.Context, req *notificationv1.PurgeUserRequest) (*emptypb.Empty, error) {
	userID, err := uuid.Parse(req.GetUserId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user id")
	}

	if err = h.notificationService.PurgeUser(ctx, userID); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *NotificationHandler) ListNotifications(ctx context.Context, req *notificationv1.ListNotificationsRequest) (*notificationv1.ListNotificationsResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	var unread bool

	switch req.GetBox() {
	case notificationv1.Box_BOX_UNREAD:
		unread = true
	case notificationv1.Box_BOX_ARCHIVE:
	default:
		return nil, status.Error(codes.InvalidArgument, "unknown box")
	}

	cursor, err := parseCursor(req.GetCursor())
	if err != nil {
		return nil, err
	}

	page, err := h.notificationService.List(ctx, userID, unread, cursor, int(req.GetLimit()))
	if err != nil {
		return nil, err
	}

	resp := &notificationv1.ListNotificationsResponse{
		Notifications: make([]*notificationv1.Notification, 0, len(page.Notifications)),
		NextCursor:    optionalID(page.NextCursor),
	}

	for _, n := range page.Notifications {
		resp.Notifications = append(resp.Notifications, toProtoNotification(n))
	}

	return resp, nil
}

func (h *NotificationHandler) CountUnread(ctx context.Context, _ *notificationv1.CountUnreadRequest) (*notificationv1.CountUnreadResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	count, err := h.notificationService.CountUnread(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &notificationv1.CountUnreadResponse{Count: int32(count)}, nil //nolint:gosec // at most service.MaxUnreadCount
}

func (h *NotificationHandler) MarkRead(ctx context.Context, req *notificationv1.MarkReadRequest) (*emptypb.Empty, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	notificationID, err := uuid.Parse(req.GetNotificationId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid notification id")
	}

	if err = h.notificationService.MarkRead(ctx, userID, notificationID); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

// parseCursor reads a page cursor; "" is the first page.
func parseCursor(s string) (uuid.UUID, error) {
	if s == "" {
		return uuid.Nil, nil
	}

	cursor, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, status.Error(codes.InvalidArgument, "invalid cursor")
	}

	return cursor, nil
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
