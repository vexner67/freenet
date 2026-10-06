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
	default:
		return nil, false
	}
}
