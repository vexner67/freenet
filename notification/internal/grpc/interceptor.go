package grpc

import (
	"errors"
	"log/slog"

	"github.com/vexner67/freenet/notification/internal/notification"
	"github.com/vexner67/freenet/notification/internal/push"
	"github.com/vexner67/freenet/notification/internal/service"
	"github.com/vexner67/freenet/pkg/grpcserver"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrorInterceptor turns notification errors into client-safe statuses.
func ErrorInterceptor(log *slog.Logger) grpc.UnaryServerInterceptor {
	return grpcserver.ErrorInterceptor(log, toStatus)
}

func toStatus(err error) (*status.Status, bool) {
	switch {
	case errors.Is(err, notification.ErrNotFound):
		return status.New(codes.NotFound, "notification not found"), true
	case errors.Is(err, notification.ErrUnknownKind),
		errors.Is(err, notification.ErrNoRecipient),
		errors.Is(err, notification.ErrNoActor),
		errors.Is(err, notification.ErrActorRecipient),
		errors.Is(err, notification.ErrTargetMismatch):
		return status.New(codes.InvalidArgument, err.Error()), true
	case errors.Is(err, push.ErrEndpointInvalid),
		errors.Is(err, push.ErrKeysInvalid),
		errors.Is(err, push.ErrNoUser),
		errors.Is(err, push.ErrNoRecipients),
		errors.Is(err, push.ErrTooManyRecipients):
		return status.New(codes.InvalidArgument, err.Error()), true
	case errors.Is(err, service.ErrPushOff):
		return status.New(codes.FailedPrecondition, err.Error()), true
	default:
		return nil, false
	}
}
