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
	case errors.Is(err, user.ErrNameEmpty):
		// An address for a page that is not filled in yet.
		return status.New(codes.InvalidArgument, user.ErrNameEmpty.Error()), true
	case errors.Is(err, user.ErrFollowSelf):
		return status.New(codes.InvalidArgument, "you cannot follow yourself"), true
	case errors.Is(err, user.ErrFollowBlocked):
		return status.New(codes.PermissionDenied, "you cannot follow this user"), true
	case errors.Is(err, user.ErrBlockSelf):
		return status.New(codes.InvalidArgument, "you cannot blacklist yourself"), true
	case errors.Is(err, user.ErrIgnoreSelf):
		return status.New(codes.InvalidArgument, user.ErrIgnoreSelf.Error()), true
	case errors.Is(err, user.ErrTooManyIgnored):
		return status.New(codes.FailedPrecondition, user.ErrTooManyIgnored.Error()), true
	case errors.Is(err, user.ErrGiftNotFound):
		return status.New(codes.NotFound, user.ErrGiftNotFound.Error()), true
	case errors.Is(err, user.ErrGiftRefused):
		return status.New(codes.PermissionDenied, user.ErrGiftRefused.Error()), true
	case errors.Is(err, user.ErrGiftsHidden):
		return status.New(codes.PermissionDenied, user.ErrGiftsHidden.Error()), true
	case errors.Is(err, user.ErrCannotDeleteGift):
		return status.New(codes.PermissionDenied, user.ErrCannotDeleteGift.Error()), true
	case errors.Is(err, user.ErrDeactivated):
		return status.New(codes.PermissionDenied, user.ErrDeactivated.Error()), true
	case errors.Is(err, user.ErrNotDeactivated):
		return status.New(codes.FailedPrecondition, user.ErrNotDeactivated.Error()), true
	case errors.Is(err, user.ErrRestoreExpired):
		return status.New(codes.FailedPrecondition, user.ErrRestoreExpired.Error()), true
	case errors.Is(err, user.ErrPartnerNotFound):
		return status.New(codes.InvalidArgument, user.ErrPartnerNotFound.Error()), true
	case errors.Is(err, user.ErrPartnerIsSelf):
		return status.New(codes.InvalidArgument, user.ErrPartnerIsSelf.Error()), true
	case errors.Is(err, user.ErrTooManyExtraFields):
		return status.New(codes.InvalidArgument, user.ErrTooManyExtraFields.Error()), true
	default:
		return nil, false
	}
}
