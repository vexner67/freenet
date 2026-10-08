package grpc

import (
	"errors"
	"log/slog"

	"github.com/vexner67/freenet/club/internal/club"
	"github.com/vexner67/freenet/club/internal/service"
	"github.com/vexner67/freenet/pkg/grpcserver"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrorInterceptor turns club errors into client-safe statuses.
func ErrorInterceptor(log *slog.Logger) grpc.UnaryServerInterceptor {
	return grpcserver.ErrorInterceptor(log, toStatus)
}

// invalid are the errors of input, told to the client as they are.
var invalid = []error{
	club.ErrNameEmpty, club.ErrNameTooLong, club.ErrDescriptionTooLong,
	club.ErrAddressTooLong, club.ErrAddressInvalid, club.ErrInvalidWall, club.ErrInvalidRole,
	club.ErrSearchQueryEmpty, club.ErrSearchQueryTooLong, service.ErrTooManyIDs,
}

func toStatus(err error) (*status.Status, bool) {
	for _, e := range invalid {
		if errors.Is(err, e) {
			return status.New(codes.InvalidArgument, e.Error()), true
		}
	}

	switch {
	case errors.Is(err, club.ErrNotFound):
		return status.New(codes.NotFound, "club not found"), true
	case errors.Is(err, club.ErrNotMember):
		return status.New(codes.NotFound, "the user is not a member of the club"), true
	case errors.Is(err, club.ErrAddressTaken):
		return status.New(codes.AlreadyExists, "club address is already taken"), true
	case errors.Is(err, club.ErrCannotEdit):
		return status.New(codes.PermissionDenied, "only the admins can edit the club"), true
	case errors.Is(err, club.ErrCannotAppoint):
		return status.New(codes.PermissionDenied, "only the owner appoints the admins"), true
	case errors.Is(err, club.ErrCannotPin):
		return status.New(codes.PermissionDenied, "only the admins can add the club to their menu"), true
	case errors.Is(err, club.ErrTooManyPinned):
		return status.New(codes.FailedPrecondition, "at most 10 clubs in the menu"), true
	case errors.Is(err, club.ErrOwnerCannotLeave):
		return status.New(codes.FailedPrecondition, "the owner cannot leave the club"), true
	default:
		return nil, false
	}
}
