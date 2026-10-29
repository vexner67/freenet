package grpc

import (
	"errors"
	"log/slog"

	"github.com/vexner67/freenet/pkg/grpcserver"
	"github.com/vexner67/freenet/world/internal/world"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrorInterceptor turns the errors of the world into client-safe
// statuses.
func ErrorInterceptor(log *slog.Logger) grpc.UnaryServerInterceptor {
	return grpcserver.ErrorInterceptor(log, toStatus)
}

// The errors told to the client as they are, by the code they get.
var (
	invalid = []error{world.ErrGrid}
	missing = []error{world.ErrNotFound, world.ErrNoWorld, world.ErrTileNotFound}
	refused = []error{world.ErrNotReady, world.ErrNotOpen}
)

func toStatus(err error) (*status.Status, bool) {
	for _, group := range []struct {
		code  codes.Code
		known []error
	}{
		{codes.InvalidArgument, invalid},
		{codes.NotFound, missing},
		{codes.FailedPrecondition, refused},
	} {
		for _, e := range group.known {
			if errors.Is(err, e) {
				return status.New(group.code, e.Error()), true
			}
		}
	}

	return nil, false
}
