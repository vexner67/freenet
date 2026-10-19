package grpc

import (
	"context"

	"github.com/google/uuid"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	"github.com/vexner67/freenet/post/internal/post"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (h *PostHandler) LikeStory(ctx context.Context, req *postv1.LikeStoryRequest) (*emptypb.Empty, error) {
	userID, id, err := callerAndStory(ctx, req.GetId())
	if err != nil {
		return nil, err
	}

	if err = h.storyService.Like(ctx, userID, id); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *PostHandler) UnlikeStory(ctx context.Context, req *postv1.UnlikeStoryRequest) (*emptypb.Empty, error) {
	userID, id, err := callerAndStory(ctx, req.GetId())
	if err != nil {
		return nil, err
	}

	if err = h.storyService.Unlike(ctx, userID, id); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *PostHandler) CreateStoryComment(ctx context.Context, req *postv1.CreateStoryCommentRequest) (*postv1.CreateStoryCommentResponse, error) {
	userID, id, err := callerAndStory(ctx, req.GetStoryId())
	if err != nil {
		return nil, err
	}

	text, err := post.NewCommentText(req.GetText())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	c, err := h.storyService.Comment(ctx, userID, id, text)
	if err != nil {
		return nil, err
	}

	return &postv1.CreateStoryCommentResponse{Comment: toProtoStoryComment(c, userID)}, nil
}

func (h *PostHandler) ListStoryComments(ctx context.Context, req *postv1.ListStoryCommentsRequest) (*postv1.ListStoryCommentsResponse, error) {
	viewerID, id, err := callerAndStory(ctx, req.GetStoryId())
	if err != nil {
		return nil, err
	}

	after := uuid.Nil
	if req.GetCursor() != "" {
		if after, err = uuid.Parse(req.GetCursor()); err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid cursor")
		}
	}

	page, err := h.storyService.Comments(ctx, viewerID, id, after, int(req.GetLimit()))
	if err != nil {
		return nil, err
	}

	resp := &postv1.ListStoryCommentsResponse{
		Comments: make([]*postv1.StoryComment, 0, len(page.Comments)),
		Total:    int32(page.Total), //nolint:gosec // comments of a story are far fewer than 2^31
	}
	for _, c := range page.Comments {
		resp.Comments = append(resp.Comments, toProtoStoryComment(c, viewerID))
	}
	if page.Next != uuid.Nil {
		resp.NextCursor = page.Next.String()
	}

	return resp, nil
}

func (h *PostHandler) DeleteStoryComment(ctx context.Context, req *postv1.DeleteStoryCommentRequest) (*emptypb.Empty, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid comment id")
	}

	if err = h.storyService.DeleteComment(ctx, userID, id); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func toProtoStoryComment(c *post.StoryComment, viewerID uuid.UUID) *postv1.StoryComment {
	return &postv1.StoryComment{
		Id:        c.ID().String(),
		StoryId:   c.StoryID().String(),
		AuthorId:  c.AuthorID().String(),
		Text:      c.Text().String(),
		CreatedAt: timestamppb.New(c.CreatedAt()),
		Mine:      c.AuthorID() == viewerID,
	}
}
