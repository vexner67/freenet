package grpc

import (
	"errors"
	"log/slog"

	"github.com/vexner67/freenet/pkg/grpcserver"
	"github.com/vexner67/freenet/user/internal/user"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrorInterceptor turns user errors into client-safe statuses.
func ErrorInterceptor(log *slog.Logger) grpc.UnaryServerInterceptor {
	return grpcserver.ErrorInterceptor(log, toStatus)
}

func toStatus(err error) (*status.Status, bool) {
	switch {
	case errors.Is(err, user.ErrNotFound):
		return status.New(codes.NotFound, "user not found"), true
	case errors.Is(err, user.ErrAlreadyExists):
		return status.New(codes.AlreadyExists, "user already exists"), true
	case errors.Is(err, user.ErrNicknameTaken):
		return status.New(codes.AlreadyExists, "nickname is already taken"), true
	case errors.Is(err, user.ErrFollowSelf):
		return status.New(codes.InvalidArgument, "you cannot follow yourself"), true
	default:
		return nil, false
	}
}
