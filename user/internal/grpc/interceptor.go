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

// invalidStickerPack are what a pack of a person may not be.
var invalidStickerPack = []error{
	user.ErrStickerPackNameEmpty,
	user.ErrStickerPackNameTooLong,
	user.ErrStickerPackAboutTooLong,
	user.ErrStickerPackPrice,
	user.ErrStickerPackEmpty,
	user.ErrTooManyPackStickers,
}

func toStatus(err error) (*status.Status, bool) {
	for _, invalid := range invalidStickerPack {
		if errors.Is(err, invalid) {
			return status.New(codes.InvalidArgument, invalid.Error()), true
		}
	}

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
	case errors.Is(err, user.ErrInviteExpired):
		return status.New(codes.FailedPrecondition, user.ErrInviteExpired.Error()), true
	case errors.Is(err, user.ErrAlreadyInvited):
		return status.New(codes.FailedPrecondition, user.ErrAlreadyInvited.Error()), true
	case errors.Is(err, user.ErrBlockSelf):
		return status.New(codes.InvalidArgument, "you cannot blacklist yourself"), true
	case errors.Is(err, user.ErrIgnoreSelf):
		return status.New(codes.InvalidArgument, user.ErrIgnoreSelf.Error()), true
	case errors.Is(err, user.ErrTooManyIgnored):
		return status.New(codes.FailedPrecondition, user.ErrTooManyIgnored.Error()), true
	case errors.Is(err, user.ErrUnknownGift):
		return status.New(codes.InvalidArgument, user.ErrUnknownGift.Error()), true
	case errors.Is(err, user.ErrGiftNotFound):
		return status.New(codes.NotFound, user.ErrGiftNotFound.Error()), true
	case errors.Is(err, user.ErrGiftRefused):
		return status.New(codes.PermissionDenied, user.ErrGiftRefused.Error()), true
	case errors.Is(err, user.ErrGiftsHidden):
		return status.New(codes.PermissionDenied, user.ErrGiftsHidden.Error()), true
	case errors.Is(err, user.ErrCannotDeleteGift):
		return status.New(codes.PermissionDenied, user.ErrCannotDeleteGift.Error()), true
	case errors.Is(err, user.ErrUnknownStickerPack):
		return status.New(codes.InvalidArgument, user.ErrUnknownStickerPack.Error()), true
	case errors.Is(err, user.ErrTooManyStickerPacks):
		return status.New(codes.FailedPrecondition, user.ErrTooManyStickerPacks.Error()), true
	case errors.Is(err, user.ErrStickerPackTaken):
		return status.New(codes.FailedPrecondition, user.ErrStickerPackTaken.Error()), true
	case errors.Is(err, user.ErrCannotChangeStickerPack):
		return status.New(codes.PermissionDenied, user.ErrCannotChangeStickerPack.Error()), true
	case errors.Is(err, user.ErrNotEnoughVotes):
		return status.New(codes.FailedPrecondition, user.ErrNotEnoughVotes.Error()), true
	case errors.Is(err, user.ErrTransferSelf):
		return status.New(codes.InvalidArgument, user.ErrTransferSelf.Error()), true
	case errors.Is(err, user.ErrTransferRefused):
		return status.New(codes.PermissionDenied, user.ErrTransferRefused.Error()), true
	case errors.Is(err, user.ErrVoucherInvalid):
		return status.New(codes.FailedPrecondition, user.ErrVoucherInvalid.Error()), true
	case errors.Is(err, user.ErrVoucherExists):
		return status.New(codes.AlreadyExists, user.ErrVoucherExists.Error()), true
	case errors.Is(err, user.ErrVoucherEmpty):
		return status.New(codes.InvalidArgument, user.ErrVoucherEmpty.Error()), true
	case errors.Is(err, user.ErrVoucherAmountInvalid):
		return status.New(codes.InvalidArgument, user.ErrVoucherAmountInvalid.Error()), true
	case errors.Is(err, user.ErrVoucherUsagesInvalid):
		return status.New(codes.InvalidArgument, user.ErrVoucherUsagesInvalid.Error()), true
	case errors.Is(err, user.ErrNotAdmin):
		return status.New(codes.PermissionDenied, user.ErrNotAdmin.Error()), true
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
