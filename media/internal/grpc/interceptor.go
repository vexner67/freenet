package grpc

import (
	"errors"
	"log/slog"

	"github.com/vexner67/freenet/media/internal/avatar"
	"github.com/vexner67/freenet/media/internal/imaging"
	"github.com/vexner67/freenet/pkg/grpcserver"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrorInterceptor turns media errors into client-safe statuses.
func ErrorInterceptor(log *slog.Logger) grpc.UnaryServerInterceptor {
	return grpcserver.ErrorInterceptor(log, toStatus)
}

func toStatus(err error) (*status.Status, bool) {
	for _, invalid := range []error{imaging.ErrTooLarge, imaging.ErrNotImage, imaging.ErrTooManyPixels, avatar.ErrTooSmall} {
		if errors.Is(err, invalid) {
			return status.New(codes.InvalidArgument, invalid.Error()), true
		}
	}

	if errors.Is(err, avatar.ErrNotFound) {
		return status.New(codes.NotFound, "avatar not found"), true
	}

	return nil, false
}
