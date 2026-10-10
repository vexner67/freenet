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

func (h *PostHandler) LikeAlbumPhoto(ctx context.Context, req *postv1.LikeAlbumPhotoRequest) (*postv1.LikeAlbumPhotoResponse, error) {
	userID, photoID, err := photoTarget(ctx, req.GetPhotoId())
	if err != nil {
		return nil, err
	}

	likes, err := h.albumService.LikePhoto(ctx, userID, photoID)
	if err != nil {
		return nil, err
	}

	return &postv1.LikeAlbumPhotoResponse{LikeCount: int32(likes)}, nil //nolint:gosec // a counter from an int4 column
}

func (h *PostHandler) UnlikeAlbumPhoto(ctx context.Context, req *postv1.UnlikeAlbumPhotoRequest) (*postv1.UnlikeAlbumPhotoResponse, error) {
	userID, photoID, err := photoTarget(ctx, req.GetPhotoId())
	if err != nil {
		return nil, err
	}

	likes, err := h.albumService.UnlikePhoto(ctx, userID, photoID)
	if err != nil {
		return nil, err
	}

	return &postv1.UnlikeAlbumPhotoResponse{LikeCount: int32(likes)}, nil //nolint:gosec // a counter from an int4 column
}

func (h *PostHandler) ListAlbumPhotoLikers(ctx context.Context, req *postv1.ListAlbumPhotoLikersRequest) (*postv1.ListAlbumPhotoLikersResponse, error) {
	photoID, err := parsePhotoID(req.GetPhotoId())
	if err != nil {
		return nil, err
	}

	cursor, err := parseLikerCursor(req.GetCursor())
	if err != nil {
		return nil, err
	}

	page, err := h.albumService.PhotoLikers(ctx, photoID, cursor, int(req.GetLimit()))
	if err != nil {
		return nil, err
	}

	ids, next, total := toProtoLikers(page)

	return &postv1.ListAlbumPhotoLikersResponse{UserIds: ids, NextCursor: next, Total: total}, nil
}

func (h *PostHandler) CreatePhotoComment(ctx context.Context, req *postv1.CreatePhotoCommentRequest) (*postv1.CreatePhotoCommentResponse, error) {
	authorID, photoID, err := photoTarget(ctx, req.GetPhotoId())
	if err != nil {
		return nil, err
	}

	text, err := post.NewCommentText(req.GetText())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	c, err := h.albumService.CommentPhoto(ctx, authorID, photoID, text)
	if err != nil {
		return nil, err
	}

	return &postv1.CreatePhotoCommentResponse{Comment: toProtoPhotoComment(c, authorID)}, nil
}

func (h *PostHandler) DeletePhotoComment(ctx context.Context, req *postv1.DeletePhotoCommentRequest) (*emptypb.Empty, error) {
	requesterID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid comment id")
	}

	if err = h.albumService.DeletePhotoComment(ctx, requesterID, id); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *PostHandler) ListPhotoComments(ctx context.Context, req *postv1.ListPhotoCommentsRequest) (*postv1.ListPhotoCommentsResponse, error) {
	photoID, err := parsePhotoID(req.GetPhotoId())
	if err != nil {
		return nil, err
	}

	after, err := parseCursor(req.GetCursor())
	if err != nil {
		return nil, err
	}

	page, err := h.albumService.PhotoComments(ctx, photoID, after, int(req.GetLimit()))
	if err != nil {
		return nil, err
	}

	viewer := viewerID(ctx)
	resp := &postv1.ListPhotoCommentsResponse{
		Comments:   make([]*postv1.PhotoComment, 0, len(page.Comments)),
		NextCursor: cursorString(page.NextCursor),
		Total:      int32(page.Total), //nolint:gosec // a counter from an int4 column
	}
	for _, c := range page.Comments {
		pc := toProtoPhotoComment(c, viewer)
		pc.Text, pc.TextCut = c.Text().Excerpt(int(req.GetTextLimit()))
		resp.Comments = append(resp.Comments, pc)
	}

	return resp, nil
}

func (h *PostHandler) GetPhotoComment(ctx context.Context, req *postv1.GetPhotoCommentRequest) (*postv1.GetPhotoCommentResponse, error) {
	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid comment id")
	}

	c, err := h.albumService.PhotoComment(ctx, id)
	if err != nil {
		return nil, err
	}

	return &postv1.GetPhotoCommentResponse{Comment: toProtoPhotoComment(c, viewerID(ctx))}, nil
}

// photoTarget is the caller and the photo of a request about it.
func photoTarget(ctx context.Context, rawPhotoID string) (uuid.UUID, uuid.UUID, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	photoID, err := parsePhotoID(rawPhotoID)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	return userID, photoID, nil
}

func toProtoPhotoComment(c *post.PhotoComment, viewerID uuid.UUID) *postv1.PhotoComment {
	return &postv1.PhotoComment{
		Id:        c.ID().String(),
		PhotoId:   c.PhotoID().String(),
		AuthorId:  c.AuthorID().String(),
		Text:      c.Text().String(),
		CreatedAt: timestamppb.New(c.CreatedAt()),
		Mine:      viewerID != uuid.Nil && viewerID == c.AuthorID(),
	}
}
