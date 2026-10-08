package grpc

import (
	"context"

	"github.com/google/uuid"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	"github.com/vexner67/freenet/post/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (h *PostHandler) CreateComment(ctx context.Context, req *postv1.CreateCommentRequest) (*postv1.CreateCommentResponse, error) {
	authorID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	postID, err := uuid.Parse(req.GetPostId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid post id")
	}

	content, err := fromProtoCommentContent(req)
	if err != nil {
		return nil, err
	}

	parentID := uuid.Nil
	if req.GetReplyToId() != "" {
		if parentID, err = uuid.Parse(req.GetReplyToId()); err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid comment id")
		}
	}

	c, err := h.commentService.Create(ctx, service.CommentInput{
		AuthorID: authorID, PostID: postID, ParentID: parentID, Content: content, FromWall: req.GetFromWall(),
	})
	if err != nil {
		return nil, err
	}

	return &postv1.CreateCommentResponse{Comment: toProtoComment(c, false, authorID)}, nil
}

func (h *PostHandler) LikeComment(ctx context.Context, req *postv1.LikeCommentRequest) (*postv1.LikeCommentResponse, error) {
	likes, err := h.setCommentLike(ctx, req.GetCommentId(), h.commentService.Like)
	if err != nil {
		return nil, err
	}

	return &postv1.LikeCommentResponse{LikeCount: likes}, nil
}

func (h *PostHandler) UnlikeComment(ctx context.Context, req *postv1.UnlikeCommentRequest) (*postv1.UnlikeCommentResponse, error) {
	likes, err := h.setCommentLike(ctx, req.GetCommentId(), h.commentService.Unlike)
	if err != nil {
		return nil, err
	}

	return &postv1.UnlikeCommentResponse{LikeCount: likes}, nil
}

func (h *PostHandler) setCommentLike(ctx context.Context, id string, set func(ctx context.Context, userID, commentID uuid.UUID) (int, error)) (int32, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return 0, err
	}

	commentID, err := uuid.Parse(id)
	if err != nil {
		return 0, status.Error(codes.InvalidArgument, "invalid comment id")
	}

	likes, err := set(ctx, userID, commentID)
	if err != nil {
		return 0, err
	}

	return int32(likes), nil //nolint:gosec // counters come from int4 columns
}

func (h *PostHandler) DeleteComment(ctx context.Context, req *postv1.DeleteCommentRequest) (*emptypb.Empty, error) {
	requesterID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	commentID, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid comment id")
	}

	if err = h.commentService.Delete(ctx, requesterID, commentID); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *PostHandler) GetComments(ctx context.Context, req *postv1.GetCommentsRequest) (*postv1.GetCommentsResponse, error) {
	ids, err := parseIDs(req.GetIds())
	if err != nil {
		return nil, err
	}

	comments, err := h.commentService.GetMany(ctx, ids)
	if err != nil {
		return nil, err
	}

	resp := &postv1.GetCommentsResponse{Comments: make([]*postv1.Comment, 0, len(comments))}
	for _, c := range comments {
		resp.Comments = append(resp.Comments, toProtoComment(c, false, uuid.Nil))
	}

	return resp, nil
}

func (h *PostHandler) ListComments(ctx context.Context, req *postv1.ListCommentsRequest) (*postv1.ListCommentsResponse, error) {
	postID, err := uuid.Parse(req.GetPostId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid post id")
	}

	cursor, err := parseCursor(req.GetCursor())
	if err != nil {
		return nil, err
	}

	page, err := h.commentService.List(ctx, viewerID(ctx), postID, cursor, int(req.GetLimit()))
	if err != nil {
		return nil, err
	}

	resp := &postv1.ListCommentsResponse{
		Comments:   make([]*postv1.Comment, 0, len(page.Comments)),
		NextCursor: cursorString(page.NextCursor),
		Total:      int32(page.Total), //nolint:gosec // counters come from int4 columns
	}

	for _, c := range page.Comments {
		resp.Comments = append(resp.Comments, toProtoComment(c, page.Liked[c.ID()], viewerID(ctx)))
	}

	return resp, nil
}
