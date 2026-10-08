package grpc

import (
	"context"

	"github.com/google/uuid"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
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

	c, err := h.commentService.Create(ctx, authorID, postID, content)
	if err != nil {
		return nil, err
	}

	return &postv1.CreateCommentResponse{Comment: toProtoComment(c)}, nil
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
		resp.Comments = append(resp.Comments, toProtoComment(c))
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

	page, err := h.commentService.List(ctx, postID, cursor, int(req.GetLimit()))
	if err != nil {
		return nil, err
	}

	resp := &postv1.ListCommentsResponse{
		Comments:   make([]*postv1.Comment, 0, len(page.Comments)),
		NextCursor: cursorString(page.NextCursor),
		Total:      int32(page.Total), //nolint:gosec // counters come from int4 columns
	}

	for _, c := range page.Comments {
		resp.Comments = append(resp.Comments, toProtoComment(c))
	}

	return resp, nil
}
