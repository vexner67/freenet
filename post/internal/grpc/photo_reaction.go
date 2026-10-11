package grpc

import (
	"context"

	"github.com/google/uuid"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
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

	replyTo, err := parseOptionalCommentID(req.GetReplyToId())
	if err != nil {
		return nil, err
	}

	mentions, err := parseMentions(req.GetMentionedIds())
	if err != nil {
		return nil, err
	}

	c, err := h.albumService.CommentPhoto(ctx, service.PhotoCommentInput{
		AuthorID: authorID,
		PhotoID:  photoID,
		ReplyTo:  replyTo,
		Text:     text,
		Mentions: mentions,
	})
	if err != nil {
		return nil, err
	}

	return &postv1.CreatePhotoCommentResponse{Comment: toProtoPhotoComment(c, authorID, false)}, nil
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

	viewer := viewerID(ctx)

	page, err := h.albumService.PhotoComments(ctx, viewer, photoID, after, int(req.GetLimit()))
	if err != nil {
		return nil, err
	}

	resp := &postv1.ListPhotoCommentsResponse{
		Comments:   make([]*postv1.PhotoComment, 0, len(page.Comments)),
		NextCursor: cursorString(page.NextCursor),
		Total:      int32(page.Total), //nolint:gosec // a counter from an int4 column
	}
	for _, c := range page.Comments {
		pc := toProtoPhotoComment(c, viewer, page.Liked[c.ID()])
		pc.Text, pc.TextCut = c.Text().Excerpt(int(req.GetTextLimit()))
		resp.Comments = append(resp.Comments, pc)
	}

	return resp, nil
}

// GetPhotoComments tells nobody whether they like the comments: a
// notification quotes them.
func (h *PostHandler) GetPhotoComments(ctx context.Context, req *postv1.GetPhotoCommentsRequest) (*postv1.GetPhotoCommentsResponse, error) {
	ids, err := parseIDs(req.GetIds())
	if err != nil {
		return nil, err
	}

	found, err := h.albumService.PhotoCommentsByID(ctx, ids)
	if err != nil {
		return nil, err
	}

	viewer := viewerID(ctx)
	resp := &postv1.GetPhotoCommentsResponse{Comments: make([]*postv1.PhotoComment, 0, len(found))}

	for _, c := range found {
		pc := toProtoPhotoComment(c, viewer, false)
		pc.Text, pc.TextCut = c.Text().Excerpt(int(req.GetTextLimit()))
		resp.Comments = append(resp.Comments, pc)
	}

	return resp, nil
}

func (h *PostHandler) LikePhotoComment(ctx context.Context, req *postv1.LikePhotoCommentRequest) (*postv1.LikePhotoCommentResponse, error) {
	likes, err := h.likePhotoComment(ctx, req.GetId(), h.albumService.LikePhotoComment)
	if err != nil {
		return nil, err
	}

	return &postv1.LikePhotoCommentResponse{LikeCount: likes}, nil
}

func (h *PostHandler) UnlikePhotoComment(ctx context.Context, req *postv1.UnlikePhotoCommentRequest) (*postv1.UnlikePhotoCommentResponse, error) {
	likes, err := h.likePhotoComment(ctx, req.GetId(), h.albumService.UnlikePhotoComment)
	if err != nil {
		return nil, err
	}

	return &postv1.UnlikePhotoCommentResponse{LikeCount: likes}, nil
}

func (h *PostHandler) likePhotoComment(ctx context.Context, rawID string, set func(ctx context.Context, userID, id uuid.UUID) (int, error)) (int32, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return 0, err
	}

	id, err := uuid.Parse(rawID)
	if err != nil {
		return 0, status.Error(codes.InvalidArgument, "invalid comment id")
	}

	likes, err := set(ctx, userID, id)
	if err != nil {
		return 0, err
	}

	return int32(likes), nil //nolint:gosec // a counter from an int4 column
}

func (h *PostHandler) ListPhotoCommentLikers(ctx context.Context, req *postv1.ListPhotoCommentLikersRequest) (*postv1.ListPhotoCommentLikersResponse, error) {
	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid comment id")
	}

	cursor, err := parseLikerCursor(req.GetCursor())
	if err != nil {
		return nil, err
	}

	page, err := h.albumService.PhotoCommentLikers(ctx, id, cursor, int(req.GetLimit()))
	if err != nil {
		return nil, err
	}

	ids, next, total := toProtoLikers(page)

	return &postv1.ListPhotoCommentLikersResponse{UserIds: ids, NextCursor: next, Total: total}, nil
}

// parseOptionalCommentID reads the comment to answer; uuid.Nil for none.
func parseOptionalCommentID(raw string) (uuid.UUID, error) {
	if raw == "" {
		return uuid.Nil, nil
	}

	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, status.Error(codes.InvalidArgument, "invalid comment id")
	}

	return id, nil
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

	return &postv1.GetPhotoCommentResponse{Comment: toProtoPhotoComment(c, viewerID(ctx), false)}, nil
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

func toProtoPhotoComment(c *post.PhotoComment, viewerID uuid.UUID, liked bool) *postv1.PhotoComment {
	pc := &postv1.PhotoComment{
		Id:        c.ID().String(),
		PhotoId:   c.PhotoID().String(),
		AuthorId:  c.AuthorID().String(),
		Text:      c.Text().String(),
		CreatedAt: timestamppb.New(c.CreatedAt()),
		Mine:      viewerID != uuid.Nil && viewerID == c.AuthorID(),
		LikeCount: int32(c.Likes()), //nolint:gosec // a counter from an int4 column
		Liked:     liked,
	}

	if c.ReplyTo() != uuid.Nil {
		pc.ReplyToId = c.ReplyTo().String()
	}

	return pc
}
