package grpc

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	"github.com/vexner67/freenet/post/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ListPostLikers is openvk's "Понравилось N людям" of a post.
func (h *PostHandler) ListPostLikers(ctx context.Context, req *postv1.ListPostLikersRequest) (*postv1.ListPostLikersResponse, error) {
	postID, err := uuid.Parse(req.GetPostId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid post id")
	}

	cursor, err := parseLikerCursor(req.GetCursor())
	if err != nil {
		return nil, err
	}

	page, err := h.postService.Likers(ctx, postID, cursor, int(req.GetLimit()))
	if err != nil {
		return nil, err
	}

	ids, next, total := toProtoLikers(page)

	return &postv1.ListPostLikersResponse{UserIds: ids, NextCursor: next, Total: total}, nil
}

// ListCommentLikers is ListPostLikers of a comment.
func (h *PostHandler) ListCommentLikers(ctx context.Context, req *postv1.ListCommentLikersRequest) (*postv1.ListCommentLikersResponse, error) {
	commentID, err := uuid.Parse(req.GetCommentId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid comment id")
	}

	cursor, err := parseLikerCursor(req.GetCursor())
	if err != nil {
		return nil, err
	}

	page, err := h.commentService.Likers(ctx, commentID, cursor, int(req.GetLimit()))
	if err != nil {
		return nil, err
	}

	ids, next, total := toProtoLikers(page)

	return &postv1.ListCommentLikersResponse{UserIds: ids, NextCursor: next, Total: total}, nil
}

func toProtoLikers(page service.LikerPage) (ids []string, next string, total int32) {
	ids = make([]string, 0, len(page.Likers))
	for _, l := range page.Likers {
		ids = append(ids, l.UserID.String())
	}

	if page.Next != nil {
		next = timedCursor(page.Next.LikedAt, page.Next.UserID)
	}

	return ids, next, int32(page.Total) //nolint:gosec // a counter from an int4 column
}

// parseLikerCursor reads "<microseconds>_<user ID>", nil for none.
func parseLikerCursor(s string) (*service.LikerCursor, error) {
	at, id, ok, err := parseTimedCursor(s)
	if err != nil || !ok {
		return nil, err
	}

	return &service.LikerCursor{LikedAt: at, UserID: id}, nil
}

// parseTimedCursor reads "<microseconds>_<ID>"; ok is false for none.
func parseTimedCursor(s string) (at time.Time, id uuid.UUID, ok bool, err error) {
	if s == "" {
		return time.Time{}, uuid.Nil, false, nil
	}

	invalid := status.Error(codes.InvalidArgument, "invalid cursor")

	micros, rawID, cut := strings.Cut(s, "_")
	if !cut {
		return time.Time{}, uuid.Nil, false, invalid
	}

	m, err := strconv.ParseInt(micros, 10, 64)
	if err != nil {
		return time.Time{}, uuid.Nil, false, invalid
	}

	if id, err = uuid.Parse(rawID); err != nil {
		return time.Time{}, uuid.Nil, false, invalid
	}

	return time.UnixMicro(m).UTC(), id, true, nil
}

func timedCursor(at time.Time, id uuid.UUID) string {
	return strconv.FormatInt(at.UnixMicro(), 10) + "_" + id.String()
}
