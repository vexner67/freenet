package grpc

import (
	"errors"
	"log/slog"

	"github.com/vexner67/freenet/federation/internal/fedi"
	"github.com/vexner67/freenet/pkg/grpcserver"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrorInterceptor turns federation errors into client-safe statuses.
func ErrorInterceptor(log *slog.Logger) grpc.UnaryServerInterceptor {
	return grpcserver.ErrorInterceptor(log, toStatus)
}

func toStatus(err error) (*status.Status, bool) {
	switch {
	case errors.Is(err, fedi.ErrNotFound), errors.Is(err, fedi.ErrNotFederated):
		return status.New(codes.NotFound, "not found"), true
	case errors.Is(err, fedi.ErrPageHidden):
		return status.New(codes.FailedPrecondition, err.Error()), true
	// An actor whose server says it is gone cannot have signed anything.
	case errors.Is(err, fedi.ErrBadSignature), errors.Is(err, fedi.ErrGone):
		return status.New(codes.Unauthenticated, "bad signature"), true
	case errors.Is(err, fedi.ErrInvalidActivity), errors.Is(err, fedi.ErrInvalidURL):
		return status.New(codes.InvalidArgument, "invalid activity"), true
	case errors.Is(err, fedi.ErrTooMany):
		return status.New(codes.ResourceExhausted, err.Error()), true
	default:
		return nil, false
	}
}
