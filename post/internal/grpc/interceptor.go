package grpc

import (
	"errors"
	"log/slog"

	"github.com/vexner67/freenet/pkg/grpcserver"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrorInterceptor turns post errors into client-safe statuses.
func ErrorInterceptor(log *slog.Logger) grpc.UnaryServerInterceptor {
	return grpcserver.ErrorInterceptor(log, toStatus)
}

func toStatus(err error) (*status.Status, bool) {
	switch {
	case errors.Is(err, post.ErrNotFound):
		return status.New(codes.NotFound, "post not found"), true
	case errors.Is(err, service.ErrTooManyAuthors):
		return status.New(codes.InvalidArgument, "too many authors"), true
	case errors.Is(err, service.ErrTooManyIDs):
		return status.New(codes.InvalidArgument, "too many ids"), true
	case errors.Is(err, post.ErrCannotDelete):
		return status.New(codes.PermissionDenied, "only the author or the owner of the wall can delete the post"), true
	case errors.Is(err, post.ErrCannotEdit):
		return status.New(codes.PermissionDenied, "only the author can edit the post"), true
	case errors.Is(err, post.ErrEditExpired):
		return status.New(codes.FailedPrecondition, "the post can no longer be edited"), true
	case errors.Is(err, post.ErrTextEmpty):
		return status.New(codes.InvalidArgument, "post text is empty"), true
	case errors.Is(err, post.ErrRepostPhotos):
		return status.New(codes.InvalidArgument, "a repost has no photos of its own"), true
	case errors.Is(err, post.ErrCannotPin):
		return status.New(codes.PermissionDenied, "only the owner of the wall can pin the post"), true
	case errors.Is(err, post.ErrCommentNotFound):
		return status.New(codes.NotFound, "comment not found"), true
	case errors.Is(err, post.ErrCannotDeleteComment):
		return status.New(codes.PermissionDenied, "only the author of the comment or of the post or the owner of the wall can delete it"), true
	default:
		return nil, false
	}
}
