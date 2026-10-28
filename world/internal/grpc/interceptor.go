package grpc

import (
	"errors"
	"log/slog"

	"github.com/vexner67/freenet/pkg/grpcserver"
	"github.com/vexner67/freenet/world/internal/service"
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
	invalid = []error{
		world.ErrGrid, world.ErrRules, world.ErrFlag, world.ErrStateName, world.ErrAdjective, world.ErrMotto,
		world.ErrColor, world.ErrGovernment, world.ErrRole, world.ErrDonation, world.ErrNotProvince,
		world.ErrSelf, service.ErrTooManyEvents,
	}
	missing = []error{
		world.ErrNotFound, world.ErrNoWorld, world.ErrTileNotFound, world.ErrStateNotFound, world.ErrNoApplication,
		world.ErrProvinceNotFound,
	}
	refused = []error{
		world.ErrNotReady, world.ErrNotOpen, world.ErrProvinceTaken, world.ErrTooClose, world.ErrNotNeighbour,
		world.ErrNoColonists, world.ErrTreasury, world.ErrCitizen, world.ErrNotCitizen, world.ErrApplied,
		world.ErrRulerLeaves, world.ErrFreeCity, service.ErrNotEnoughVotes, service.ErrNotSettled,
	}
	taken     = []error{world.ErrNameTaken}
	forbidden = []error{world.ErrNotAllowed, service.ErrPageGone}
)

func toStatus(err error) (*status.Status, bool) {
	for _, group := range []struct {
		code  codes.Code
		known []error
	}{
		{codes.InvalidArgument, invalid},
		{codes.NotFound, missing},
		{codes.FailedPrecondition, refused},
		{codes.AlreadyExists, taken},
		{codes.PermissionDenied, forbidden},
	} {
		for _, e := range group.known {
			if errors.Is(err, e) {
				return status.New(group.code, e.Error()), true
			}
		}
	}

	return nil, false
}
