package grpc

import (
	"errors"
	"log/slog"

	"github.com/vexner67/freenet/message/internal/message"
	"github.com/vexner67/freenet/pkg/grpcserver"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrorInterceptor turns message errors into client-safe statuses.
func ErrorInterceptor(log *slog.Logger) grpc.UnaryServerInterceptor {
	return grpcserver.ErrorInterceptor(log, toStatus)
}

func toStatus(err error) (*status.Status, bool) {
	switch {
	case errors.Is(err, message.ErrNotFound):
		return status.New(codes.NotFound, "message not found"), true
	case errors.Is(err, message.ErrToYourself):
		return status.New(codes.InvalidArgument, "you cannot write to yourself"), true
	case errors.Is(err, message.ErrCannotDelete):
		return status.New(codes.PermissionDenied, "only the sender can delete the message"), true
	case errors.Is(err, message.ErrDeleteExpired):
		return status.New(codes.FailedPrecondition, "the message can no longer be deleted"), true
	case errors.Is(err, message.ErrCannotEdit):
		return status.New(codes.PermissionDenied, "only the sender can edit the message"), true
	case errors.Is(err, message.ErrEditExpired):
		return status.New(codes.FailedPrecondition, "the message can no longer be edited"), true
	case errors.Is(err, message.ErrTextEmpty):
		return status.New(codes.InvalidArgument, "message text is empty"), true
	default:
		return nil, false
	}
}
