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
	case errors.Is(err, service.ErrSyncMalformed):
		return status.New(codes.InvalidArgument, service.ErrSyncMalformed.Error()), true
	case errors.Is(err, service.ErrSourceIsReader):
		return status.New(codes.InvalidArgument, service.ErrSourceIsReader.Error()), true
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
	case errors.Is(err, post.ErrFromOwnWall):
		return status.New(codes.InvalidArgument, "a post on your own wall speaks for you"), true
	case errors.Is(err, post.ErrInvalidPrice), errors.Is(err, post.ErrPricedElsewhere), errors.Is(err, post.ErrPriceWithoutMedia):
		return status.New(codes.InvalidArgument, err.Error()), true
	case errors.Is(err, post.ErrRepostPhotos):
		return status.New(codes.InvalidArgument, "a post sharing a photo has no photos of its own"), true
	case errors.Is(err, post.ErrCannotPin):
		return status.New(codes.PermissionDenied, "only the owner of the wall can pin the post"), true
	case errors.Is(err, post.ErrReplyOtherPost):
		return status.New(codes.InvalidArgument, "the comment to reply to is under another post"), true
	case errors.Is(err, post.ErrReplyOtherPhoto):
		return status.New(codes.InvalidArgument, "the comment to reply to is under another photo"), true
	case errors.Is(err, post.ErrCommentNotFound):
		return status.New(codes.NotFound, "comment not found"), true
	case errors.Is(err, post.ErrSuggestionNotFound):
		return status.New(codes.NotFound, "suggestion not found"), true
	case errors.Is(err, post.ErrCannotWithdraw):
		return status.New(codes.PermissionDenied, "only the author or the admins can delete the suggestion"), true
	case errors.Is(err, post.ErrCannotPublish):
		return status.New(codes.PermissionDenied, "only the admins can publish the suggestion"), true
	case errors.Is(err, post.ErrCannotSeeAll):
		return status.New(codes.PermissionDenied, "only the admins see all the suggestions"), true
	case errors.Is(err, post.ErrCannotDeleteComment):
		return status.New(codes.PermissionDenied, "only the author of the comment or of the post or the owner of the wall can delete it"), true
	default:
		return topicStatus(err)
	}
}

// topicStatus is toStatus for the discussions of communities and the
// photo albums.
func topicStatus(err error) (*status.Status, bool) {
	for _, known := range []struct {
		err  error
		code codes.Code
	}{
		{post.ErrCannotArchive, codes.PermissionDenied},
		{post.ErrWallClosed, codes.PermissionDenied},
		{post.ErrCannotCloseComments, codes.PermissionDenied},
		{post.ErrCommentsClosed, codes.FailedPrecondition},
		{post.ErrWallQueryTooLong, codes.InvalidArgument},
		{post.ErrSearchQueryTooLong, codes.InvalidArgument},
		{post.ErrInvalidWallYear, codes.InvalidArgument},
		{post.ErrTopicNotFound, codes.NotFound},
		{post.ErrMessageNotFound, codes.NotFound},
		{post.ErrCannotChangeTopic, codes.PermissionDenied},
		{post.ErrCannotPinTopic, codes.PermissionDenied},
		{post.ErrCannotDeleteMessage, codes.PermissionDenied},
		{post.ErrTopicRestricted, codes.PermissionDenied},
		{post.ErrRestrictOwnTopic, codes.InvalidArgument},
		{post.ErrTopicClosed, codes.FailedPrecondition},
		{post.ErrAlbumNotFound, codes.NotFound},
		{post.ErrAlbumPhotoNotFound, codes.NotFound},
		{post.ErrCannotChangeAlbum, codes.PermissionDenied},
		{post.ErrCannotDescribePhoto, codes.PermissionDenied},
		{post.ErrCannotDeleteAlbumPhoto, codes.PermissionDenied},
		{post.ErrCannotDeletePhotoComment, codes.PermissionDenied},
		{post.ErrCommentEmpty, codes.InvalidArgument},
		{post.ErrNoAlbumPhotos, codes.InvalidArgument},
		{post.ErrSystemAlbum, codes.FailedPrecondition},
		{post.ErrCannotMovePhoto, codes.InvalidArgument},
		{post.ErrDuplicatePhoto, codes.InvalidArgument},
		{post.ErrInvalidPhoto, codes.InvalidArgument},
		{service.ErrTooManyAlbumPhotos, codes.InvalidArgument},
		{post.ErrPollNotFound, codes.NotFound},
		{post.ErrPollDays, codes.InvalidArgument},
		{post.ErrPollChoice, codes.InvalidArgument},
		{post.ErrPollEnded, codes.FailedPrecondition},
		{post.ErrPollVoted, codes.FailedPrecondition},
		{post.ErrPollNotVoted, codes.FailedPrecondition},
		{post.ErrPollLocked, codes.FailedPrecondition},
		{post.ErrPollAnonymous, codes.FailedPrecondition},
		{post.ErrPostponedNotFound, codes.NotFound},
		{post.ErrPublishTime, codes.InvalidArgument},
		{post.ErrPostponedElsewhere, codes.InvalidArgument},
		{post.ErrPostponedContent, codes.InvalidArgument},
		{post.ErrCannotSeePostponed, codes.PermissionDenied},
		{post.ErrCannotPostpone, codes.PermissionDenied},
		{post.ErrTooManyPostponed, codes.FailedPrecondition},
		{post.ErrStoryNotFound, codes.NotFound},
		{post.ErrStoryTextTooLong, codes.InvalidArgument},
		{post.ErrStoryWithoutPhoto, codes.InvalidArgument},
		{post.ErrTooManyStories, codes.FailedPrecondition},
		{post.ErrCannotDeleteStory, codes.PermissionDenied},
		{post.ErrNotStoryAuthor, codes.PermissionDenied},
		{post.ErrStoryCommentNotFound, codes.NotFound},
		{post.ErrStoryCommentTooLong, codes.InvalidArgument},
		{post.ErrCannotDeleteStoryComment, codes.PermissionDenied},
		{post.ErrDocumentNotFound, codes.NotFound},
		{post.ErrArticleNotFound, codes.NotFound},
		{post.ErrCannotChangeArticle, codes.PermissionDenied},
		{post.ErrArticleTitleEmpty, codes.InvalidArgument},
		{post.ErrArticleTitleTooLong, codes.InvalidArgument},
		{post.ErrArticleEmpty, codes.InvalidArgument},
		{post.ErrArticleTooLong, codes.InvalidArgument},
		{post.ErrArticleBlock, codes.InvalidArgument},
		{post.ErrArticleLink, codes.InvalidArgument},
		{post.ErrArticlePhotos, codes.InvalidArgument},
		{post.ErrCannotChangeDocument, codes.PermissionDenied},
		{post.ErrInvalidDocument, codes.InvalidArgument},
		{post.ErrInvalidDocumentSize, codes.InvalidArgument},
	} {
		if errors.Is(err, known.err) {
			return status.New(known.code, known.err.Error()), true
		}
	}

	return nil, false
}
