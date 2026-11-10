package grpc

import (
	"context"
	"errors"
	"github.com/vexner67/freenet/pkg/grpcserver"
	"github.com/vexner67/freenet/room/internal/room"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"log/slog"
)

func ErrorInterceptor(log *slog.Logger) grpc.UnaryServerInterceptor {
	return grpcserver.ErrorInterceptor(log, toStatus)
}
func toStatus(err error) (*status.Status, bool) {
	for _, candidate := range []struct {
		err  error
		code codes.Code
	}{
		{room.ErrUnavailable, codes.Unavailable},
		{room.ErrInvalidID, codes.InvalidArgument}, {room.ErrInvalidTitle, codes.InvalidArgument}, {room.ErrInvalidKind, codes.InvalidArgument}, {room.ErrInvalidAccess, codes.InvalidArgument}, {room.ErrInvalidControl, codes.InvalidArgument}, {room.ErrInvalidPlayback, codes.InvalidArgument}, {room.ErrInvalidChat, codes.InvalidArgument},
		{room.ErrNotFound, codes.NotFound}, {room.ErrFull, codes.ResourceExhausted}, {room.ErrTooManyRooms, codes.ResourceExhausted}, {room.ErrClosed, codes.FailedPrecondition}, {room.ErrStalePlayback, codes.FailedPrecondition}, {room.ErrNotMember, codes.FailedPrecondition}, {room.ErrForbidden, codes.PermissionDenied}, {room.ErrKicked, codes.PermissionDenied},
		{context.Canceled, codes.Canceled}, {context.DeadlineExceeded, codes.DeadlineExceeded},
	} {
		if errors.Is(err, candidate.err) {
			return status.New(candidate.code, candidate.err.Error()), true
		}
	}
	return nil, false
}
