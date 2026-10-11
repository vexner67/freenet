package grpc

import (
	"context"

	"github.com/google/uuid"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	"github.com/vexner67/freenet/post/internal/service"
)

// FaveService is what a person likes ("Закладки" of openvk).
type FaveService interface {
	Posts(ctx context.Context, userID uuid.UUID, cursor *service.FaveCursor, size int) (service.FavePostPage, error)
	Comments(ctx context.Context, userID uuid.UUID, cursor *service.FaveCursor, size int) (service.FaveCommentPage, error)
	Photos(ctx context.Context, userID uuid.UUID, cursor *service.FaveCursor, size int) (service.FavePhotoPage, error)
}

func (h *PostHandler) ListLikedPosts(ctx context.Context, req *postv1.ListLikedPostsRequest) (*postv1.ListLikedPostsResponse, error) {
	userID, cursor, err := faveRequest(ctx, req.GetCursor())
	if err != nil {
		return nil, err
	}

	page, err := h.faveService.Posts(ctx, userID, cursor, int(req.GetLimit()))
	if err != nil {
		return nil, err
	}

	resp := &postv1.ListLikedPostsResponse{
		Posts:      make([]*postv1.Post, 0, len(page.Posts)),
		NextCursor: faveCursorString(page.Next),
		Total:      int32(page.Total), //nolint:gosec // nobody likes 2^31 posts
	}
	for _, p := range page.Posts {
		resp.Posts = append(resp.Posts, excerpted(toProtoPost(p, true, userID), p, req.GetTextLimit()))
	}

	return resp, nil
}

func (h *PostHandler) ListLikedComments(ctx context.Context, req *postv1.ListLikedCommentsRequest) (*postv1.ListLikedCommentsResponse, error) {
	userID, cursor, err := faveRequest(ctx, req.GetCursor())
	if err != nil {
		return nil, err
	}

	page, err := h.faveService.Comments(ctx, userID, cursor, int(req.GetLimit()))
	if err != nil {
		return nil, err
	}

	resp := &postv1.ListLikedCommentsResponse{
		Comments:   make([]*postv1.Comment, 0, len(page.Comments)),
		NextCursor: faveCursorString(page.Next),
		Total:      int32(page.Total), //nolint:gosec // nobody likes 2^31 comments
	}
	for _, c := range page.Comments {
		resp.Comments = append(resp.Comments, excerptedComment(toProtoComment(c, true, userID), c, req.GetTextLimit()))
	}

	return resp, nil
}

func (h *PostHandler) ListLikedAlbumPhotos(ctx context.Context, req *postv1.ListLikedAlbumPhotosRequest) (*postv1.ListLikedAlbumPhotosResponse, error) {
	userID, cursor, err := faveRequest(ctx, req.GetCursor())
	if err != nil {
		return nil, err
	}

	page, err := h.faveService.Photos(ctx, userID, cursor, int(req.GetLimit()))
	if err != nil {
		return nil, err
	}

	resp := &postv1.ListLikedAlbumPhotosResponse{
		Photos:     make([]*postv1.AlbumPhoto, 0, len(page.Photos)),
		OwnerIds:   make([]string, 0, len(page.Photos)),
		NextCursor: faveCursorString(page.Next),
		Total:      int32(page.Total), //nolint:gosec // nobody likes 2^31 photos
	}
	for _, p := range page.Photos {
		pp := toProtoAlbumPhoto(p, userID)
		pp.Liked = true
		resp.Photos = append(resp.Photos, pp)
		resp.OwnerIds = append(resp.OwnerIds, page.Owners[p.AlbumID()].String())
	}

	return resp, nil
}

// ListOwnPosts is what the caller wrote ("Написанные мной записи").
func (h *PostHandler) ListOwnPosts(ctx context.Context, req *postv1.ListOwnPostsRequest) (*postv1.ListOwnPostsResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	cursor, err := parseCursor(req.GetCursor())
	if err != nil {
		return nil, err
	}

	page, err := h.postService.Own(ctx, userID, cursor, int(req.GetLimit()))
	if err != nil {
		return nil, err
	}

	return &postv1.ListOwnPostsResponse{
		Posts:      toProtoPosts(page, userID, req.GetTextLimit()),
		NextCursor: cursorString(page.NextCursor),
		Total:      int32(page.Total), //nolint:gosec // nobody writes 2^31 posts
	}, nil
}

// faveRequest is who asks for their faves and from where.
func faveRequest(ctx context.Context, rawCursor string) (uuid.UUID, *service.FaveCursor, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return uuid.Nil, nil, err
	}

	at, id, ok, err := parseTimedCursor(rawCursor)
	if err != nil || !ok {
		return userID, nil, err
	}

	return userID, &service.FaveCursor{LikedAt: at, ID: id}, nil
}

func faveCursorString(next *service.FaveCursor) string {
	if next == nil {
		return ""
	}

	return timedCursor(next.LikedAt, next.ID)
}
