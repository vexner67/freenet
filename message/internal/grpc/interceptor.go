package grpc

import (
	"errors"
	"log/slog"

	"github.com/vexner67/freenet/message/internal/message"
	"github.com/vexner67/freenet/pkg/grpcserver"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrorInterceptor turns message errors into client-safe statuses.
func ErrorInterceptor(log *slog.Logger) grpc.UnaryServerInterceptor {
	return grpcserver.ErrorInterceptor(log, toStatus)
}

func toStatus(err error) (*status.Status, bool) {
	switch {
	case errors.Is(err, message.ErrNotFound):
		return status.New(codes.NotFound, "message not found"), true
	case errors.Is(err, message.ErrToYourself):
		return status.New(codes.InvalidArgument, "you cannot write to yourself"), true
	case errors.Is(err, message.ErrCannotDelete):
		return status.New(codes.PermissionDenied, "only the sender can delete the message"), true
	case errors.Is(err, message.ErrDeleteExpired):
		return status.New(codes.FailedPrecondition, "the message can no longer be deleted"), true
	case errors.Is(err, message.ErrCannotEdit):
		return status.New(codes.PermissionDenied, "only the sender can edit the message"), true
	case errors.Is(err, message.ErrEditExpired):
		return status.New(codes.FailedPrecondition, "the message can no longer be edited"), true
	case errors.Is(err, message.ErrEditConflict):
		return status.New(codes.FailedPrecondition, "the message was changed"), true
	case errors.Is(err, message.ErrForwardNotFound):
		return status.New(codes.InvalidArgument, "forwarded message not found"), true
	case errors.Is(err, message.ErrTooManyForwarded):
		return status.New(codes.InvalidArgument, "a message forwards at most 20 messages"), true
	case errors.Is(err, message.ErrInvalidSticker):
		return status.New(codes.InvalidArgument, "invalid sticker"), true
	case errors.Is(err, message.ErrStickerAlone):
		return status.New(codes.InvalidArgument, "a sticker goes alone"), true
	case errors.Is(err, message.ErrSharedPhotoAlone):
		return status.New(codes.InvalidArgument, "a shared photo goes with text alone"), true
	case errors.Is(err, message.ErrTooManyPhotos):
		return status.New(codes.InvalidArgument, message.ErrTooManyPhotos.Error()), true
	case errors.Is(err, message.ErrNoMessages):
		return status.New(codes.InvalidArgument, "no messages to delete"), true
	case errors.Is(err, message.ErrTooManyMessages):
		return status.New(codes.InvalidArgument, "at most 100 messages are deleted at once"), true
	case errors.Is(err, message.ErrChatNotFound):
		return status.New(codes.NotFound, "chat not found"), true
	case errors.Is(err, message.ErrMemberNotFound):
		return status.New(codes.NotFound, "member not found"), true
	case errors.Is(err, message.ErrTooFewMembers):
		return status.New(codes.InvalidArgument, "a chat needs at least two other people"), true
	case errors.Is(err, message.ErrKickYourself):
		return status.New(codes.InvalidArgument, "you cannot remove yourself, leave the chat instead"), true
	case errors.Is(err, message.ErrCannotKick):
		return status.New(codes.PermissionDenied, "only the creator can remove members"), true
	case errors.Is(err, message.ErrChatFull):
		return status.New(codes.FailedPrecondition, "the chat is full"), true
	case errors.Is(err, message.ErrNotInChat):
		return status.New(codes.FailedPrecondition, "you are not in this chat"), true
	case errors.Is(err, message.ErrMemberLeft):
		return status.New(codes.FailedPrecondition, "the user has left the chat"), true
	case errors.Is(err, message.ErrKicked):
		return status.New(codes.FailedPrecondition, "you were removed from this chat"), true
	case errors.Is(err, message.ErrTextEmpty):
		return status.New(codes.InvalidArgument, "message text is empty"), true
	case errors.Is(err, message.ErrKeyChanged):
		return status.New(codes.FailedPrecondition, message.ErrKeyChanged.Error()), true
	case errors.Is(err, message.ErrNoKey):
		return status.New(codes.NotFound, message.ErrNoKey.Error()), true
	case errors.Is(err, message.ErrPlainText):
		return status.New(codes.InvalidArgument, message.ErrPlainText.Error()), true
	case errors.Is(err, message.ErrSealedInChat):
		return status.New(codes.InvalidArgument, message.ErrSealedInChat.Error()), true
	case errors.Is(err, message.ErrForwardIntoChat):
		return status.New(codes.InvalidArgument, message.ErrForwardIntoChat.Error()), true
	default:
		return nil, false
	}
}
