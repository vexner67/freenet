package backend

import (
	"context"

	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/pkg/errs"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
)

// Faves is rest.Faves over the post service: what the caller of a token
// likes ("Закладки") and what they wrote ("Написанные мной записи").
type Faves struct {
	api postv1.PostServiceClient
}

func NewFaves(api postv1.PostServiceClient) *Faves {
	return &Faves{
		api: api,
	}
}

// Posts are the posts the caller likes, newest like first, with the
// beginnings of their texts.
func (f *Faves) Posts(ctx context.Context, accessToken, cursor string, limit int) (model.PostPage, error) {
	resp, err := f.api.ListLikedPosts(withToken(ctx, accessToken), &postv1.ListLikedPostsRequest{
		Cursor: cursor, Limit: toLimit(limit), TextLimit: model.ExcerptLength,
	})
	if err != nil {
		return model.PostPage{}, errs.Wrap("f.api.ListLikedPosts", fromStatus(err))
	}

	return model.PostPage{Posts: toPosts(resp.GetPosts()), NextCursor: resp.GetNextCursor(), Total: int(resp.GetTotal())}, nil
}

// Comments are the comments under posts the caller likes.
func (f *Faves) Comments(ctx context.Context, accessToken, cursor string, limit int) (model.CommentPage, error) {
	resp, err := f.api.ListLikedComments(withToken(ctx, accessToken), &postv1.ListLikedCommentsRequest{
		Cursor: cursor, Limit: toLimit(limit), TextLimit: model.ExcerptLength,
	})
	if err != nil {
		return model.CommentPage{}, errs.Wrap("f.api.ListLikedComments", fromStatus(err))
	}

	page := model.CommentPage{
		Comments:   make([]model.Comment, 0, len(resp.GetComments())),
		NextCursor: resp.GetNextCursor(),
		Total:      int(resp.GetTotal()),
	}
	for _, c := range resp.GetComments() {
		page.Comments = append(page.Comments, toComment(c))
	}

	return page, nil
}

// Photos are the photos of albums the caller likes, each with whom its
// album belongs to.
func (f *Faves) Photos(ctx context.Context, accessToken, cursor string, limit int) (model.FavePhotoPage, error) {
	resp, err := f.api.ListLikedAlbumPhotos(withToken(ctx, accessToken), &postv1.ListLikedAlbumPhotosRequest{
		Cursor: cursor, Limit: toLimit(limit),
	})
	if err != nil {
		return model.FavePhotoPage{}, errs.Wrap("f.api.ListLikedAlbumPhotos", fromStatus(err))
	}

	page := model.FavePhotoPage{
		Photos:     make([]model.FavePhoto, 0, len(resp.GetPhotos())),
		NextCursor: resp.GetNextCursor(),
		Total:      int(resp.GetTotal()),
	}

	owners := resp.GetOwnerIds()
	for i, pp := range resp.GetPhotos() {
		photo := model.FavePhoto{AlbumPhoto: toAlbumPhoto(pp)}
		if i < len(owners) {
			photo.OwnerID = owners[i]
		}

		page.Photos = append(page.Photos, photo)
	}

	return page, nil
}

// OwnPosts are the posts the caller wrote, on whichever wall, newest
// first.
func (f *Faves) OwnPosts(ctx context.Context, accessToken, cursor string, limit int) (model.PostPage, error) {
	resp, err := f.api.ListOwnPosts(withToken(ctx, accessToken), &postv1.ListOwnPostsRequest{
		Cursor: cursor, Limit: toLimit(limit), TextLimit: model.ExcerptLength,
	})
	if err != nil {
		return model.PostPage{}, errs.Wrap("f.api.ListOwnPosts", fromStatus(err))
	}

	return model.PostPage{Posts: toPosts(resp.GetPosts()), NextCursor: resp.GetNextCursor(), Total: int(resp.GetTotal())}, nil
}
