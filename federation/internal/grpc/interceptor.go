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
// An account of another server not found tells why in the log: what its
// server did is no business of the caller, but of whoever runs the site.
func ErrorInterceptor(log *slog.Logger) grpc.UnaryServerInterceptor {
	return grpcserver.ErrorInterceptor(log, func(err error) (*status.Status, bool) {
		if errors.Is(err, fedi.ErrAccountNotFound) {
			log.Warn("fediverse account not read", "error", err)
		}

		return toStatus(err)
	})
}

func toStatus(err error) (*status.Status, bool) {
	switch {
	// Before ErrGone, which may be why the account is not found.
	case errors.Is(err, fedi.ErrAccountNotFound):
		return status.New(codes.NotFound, fedi.ErrAccountNotFound.Error()), true
	case errors.Is(err, fedi.ErrInvalidAddress):
		return status.New(codes.InvalidArgument, fedi.ErrInvalidAddress.Error()), true
	case errors.Is(err, fedi.ErrTooManyLookups):
		return status.New(codes.ResourceExhausted, fedi.ErrTooManyLookups.Error()), true
	case errors.Is(err, fedi.ErrTooManyLikes):
		return status.New(codes.ResourceExhausted, fedi.ErrTooManyLikes.Error()), true
	case errors.Is(err, fedi.ErrTooManySubscriptions):
		return status.New(codes.FailedPrecondition, fedi.ErrTooManySubscriptions.Error()), true
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
