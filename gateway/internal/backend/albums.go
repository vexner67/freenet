package backend

import (
	"context"

	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/pkg/errs"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
)

// Albums is rest.Albums over the post service, which keeps the photo
// albums of people and communities.
type Albums struct {
	api postv1.PostServiceClient
}

func NewAlbums(api postv1.PostServiceClient) *Albums {
	return &Albums{
		api: api,
	}
}

func (a *Albums) Create(ctx context.Context, accessToken string, na model.NewAlbum) (model.Album, error) {
	resp, err := a.api.CreateAlbum(withToken(ctx, accessToken), &postv1.CreateAlbumRequest{
		ClubId: na.ClubID, Name: na.Name, Description: na.Description,
	})
	if err != nil {
		return model.Album{}, errs.Wrap("a.api.CreateAlbum", fromStatus(err))
	}

	return toAlbum(resp.GetAlbum()), nil
}

func (a *Albums) Get(ctx context.Context, accessToken, id string) (model.Album, error) {
	resp, err := a.api.GetAlbum(withToken(ctx, accessToken), &postv1.GetAlbumRequest{Id: id})
	if err != nil {
		return model.Album{}, errs.Wrap("a.api.GetAlbum", fromStatus(err))
	}

	return toAlbum(resp.GetAlbum()), nil
}

func (a *Albums) List(ctx context.Context, accessToken, ownerID, cursor string, limit int) (model.AlbumPage, error) {
	resp, err := a.api.ListAlbums(withToken(ctx, accessToken), &postv1.ListAlbumsRequest{
		OwnerId: ownerID, Cursor: cursor, Limit: int32(limit), //nolint:gosec // limit is validated by the caller
	})
	if err != nil {
		return model.AlbumPage{}, errs.Wrap("a.api.ListAlbums", fromStatus(err))
	}

	page := model.AlbumPage{NextCursor: resp.GetNextCursor(), Total: int(resp.GetTotal())}
	for _, pa := range resp.GetAlbums() {
		page.Albums = append(page.Albums, toAlbum(pa))
	}

	return page, nil
}

func (a *Albums) Edit(ctx context.Context, accessToken, id, name, description string) (model.Album, error) {
	resp, err := a.api.EditAlbum(withToken(ctx, accessToken), &postv1.EditAlbumRequest{Id: id, Name: name, Description: description})
	if err != nil {
		return model.Album{}, errs.Wrap("a.api.EditAlbum", fromStatus(err))
	}

	return toAlbum(resp.GetAlbum()), nil
}

func (a *Albums) Delete(ctx context.Context, accessToken, id string) error {
	if _, err := a.api.DeleteAlbum(withToken(ctx, accessToken), &postv1.DeleteAlbumRequest{Id: id}); err != nil {
		return errs.Wrap("a.api.DeleteAlbum", fromStatus(err))
	}

	return nil
}

// AddAvatarPhoto puts a new photo of the caller's page, or of a club they
// run (clubID), in its "Фотографии со страницы".
func (a *Albums) AddAvatarPhoto(ctx context.Context, accessToken, clubID, photoID string) error {
	_, err := a.api.AddAvatarPhoto(withToken(ctx, accessToken), &postv1.AddAvatarPhotoRequest{ClubId: clubID, PhotoId: photoID})
	if err != nil {
		return errs.Wrap("a.api.AddAvatarPhoto", fromStatus(err))
	}

	return nil
}

func (a *Albums) AddPhotos(ctx context.Context, accessToken, albumID string, photos []model.NewAlbumPhoto) ([]model.AlbumPhoto, error) {
	req := &postv1.AddAlbumPhotosRequest{AlbumId: albumID}
	for _, p := range photos {
		req.Photos = append(req.Photos, &postv1.NewAlbumPhoto{PhotoId: p.PhotoID, Description: p.Description})
	}

	resp, err := a.api.AddAlbumPhotos(withToken(ctx, accessToken), req)
	if err != nil {
		return nil, errs.Wrap("a.api.AddAlbumPhotos", fromStatus(err))
	}

	added := make([]model.AlbumPhoto, 0, len(resp.GetPhotos()))
	for _, pp := range resp.GetPhotos() {
		added = append(added, toAlbumPhoto(pp))
	}

	return added, nil
}

func (a *Albums) Photos(ctx context.Context, accessToken, albumID, cursor string, limit int) (model.AlbumPhotoPage, error) {
	resp, err := a.api.ListAlbumPhotos(withToken(ctx, accessToken), &postv1.ListAlbumPhotosRequest{
		AlbumId: albumID, Cursor: cursor, Limit: int32(limit), //nolint:gosec // limit is validated by the caller
	})
	if err != nil {
		return model.AlbumPhotoPage{}, errs.Wrap("a.api.ListAlbumPhotos", fromStatus(err))
	}

	page := model.AlbumPhotoPage{NextCursor: resp.GetNextCursor(), Total: int(resp.GetTotal())}
	for _, pp := range resp.GetPhotos() {
		page.Photos = append(page.Photos, toAlbumPhoto(pp))
	}

	return page, nil
}

func (a *Albums) Photo(ctx context.Context, accessToken, id string) (model.AlbumPhoto, model.Album, error) {
	resp, err := a.api.GetAlbumPhoto(withToken(ctx, accessToken), &postv1.GetAlbumPhotoRequest{Id: id})
	if err != nil {
		return model.AlbumPhoto{}, model.Album{}, errs.Wrap("a.api.GetAlbumPhoto", fromStatus(err))
	}

	return toAlbumPhoto(resp.GetPhoto()), toAlbum(resp.GetAlbum()), nil
}

func (a *Albums) Describe(ctx context.Context, accessToken, id, description string) (model.AlbumPhoto, error) {
	resp, err := a.api.EditAlbumPhoto(withToken(ctx, accessToken), &postv1.EditAlbumPhotoRequest{Id: id, Description: description})
	if err != nil {
		return model.AlbumPhoto{}, errs.Wrap("a.api.EditAlbumPhoto", fromStatus(err))
	}

	return toAlbumPhoto(resp.GetPhoto()), nil
}

func (a *Albums) DeletePhoto(ctx context.Context, accessToken, id string) error {
	if _, err := a.api.DeleteAlbumPhoto(withToken(ctx, accessToken), &postv1.DeleteAlbumPhotoRequest{Id: id}); err != nil {
		return errs.Wrap("a.api.DeleteAlbumPhoto", fromStatus(err))
	}

	return nil
}

func (a *Albums) SetCover(ctx context.Context, accessToken, albumID, photoID string) error {
	req := &postv1.SetAlbumCoverRequest{AlbumId: albumID, PhotoId: photoID}
	if _, err := a.api.SetAlbumCover(withToken(ctx, accessToken), req); err != nil {
		return errs.Wrap("a.api.SetAlbumCover", fromStatus(err))
	}

	return nil
}

func (a *Albums) MovePhoto(ctx context.Context, accessToken, photoID, albumID string) error {
	req := &postv1.MoveAlbumPhotoRequest{PhotoId: photoID, AlbumId: albumID}
	if _, err := a.api.MoveAlbumPhoto(withToken(ctx, accessToken), req); err != nil {
		return errs.Wrap("a.api.MoveAlbumPhoto", fromStatus(err))
	}

	return nil
}

func toAlbum(pa *postv1.Album) model.Album {
	a := model.Album{
		ID:          pa.GetId(),
		OwnerID:     pa.GetOwnerId(),
		Name:        pa.GetName(),
		Description: pa.GetDescription(),
		CreatedAt:   fromTimestamp(pa.GetCreatedAt()),
		UpdatedAt:   fromTimestamp(pa.GetUpdatedAt()),
		Photos:      int(pa.GetPhotoCount()),
	}
	switch pa.GetKind() {
	case postv1.AlbumKind_ALBUM_KIND_AVATARS:
		a.Kind = "avatars"
	case postv1.AlbumKind_ALBUM_KIND_WALL:
		a.Kind = "wall"
	case postv1.AlbumKind_ALBUM_KIND_NONE:
	}
	if pa.GetCover() != nil {
		cover := toAlbumPhoto(pa.GetCover())
		a.Cover = &cover
	}

	return a
}

func toAlbumPhoto(pp *postv1.AlbumPhoto) model.AlbumPhoto {
	return model.AlbumPhoto{
		ID:          pp.GetId(),
		AlbumID:     pp.GetAlbumId(),
		AuthorID:    pp.GetAuthorId(),
		Description: pp.GetDescription(),
		CreatedAt:   fromTimestamp(pp.GetCreatedAt()),
		Mine:        pp.GetMine(),
		Likes:       int(pp.GetLikeCount()),
		Comments:    int(pp.GetCommentCount()),
		Liked:       pp.GetLiked(),
		NSFW:        pp.GetNsfw(),
	}
}

// LikePhoto and UnlikePhoto return how many like the photo now.
func (a *Albums) LikePhoto(ctx context.Context, accessToken, id string) (int, error) {
	resp, err := a.api.LikeAlbumPhoto(withToken(ctx, accessToken), &postv1.LikeAlbumPhotoRequest{PhotoId: id})
	if err != nil {
		return 0, errs.Wrap("a.api.LikeAlbumPhoto", fromStatus(err))
	}

	return int(resp.GetLikeCount()), nil
}

func (a *Albums) UnlikePhoto(ctx context.Context, accessToken, id string) (int, error) {
	resp, err := a.api.UnlikeAlbumPhoto(withToken(ctx, accessToken), &postv1.UnlikeAlbumPhotoRequest{PhotoId: id})
	if err != nil {
		return 0, errs.Wrap("a.api.UnlikeAlbumPhoto", fromStatus(err))
	}

	return int(resp.GetLikeCount()), nil
}

// PhotoLikers is who like the photo, newest first.
func (a *Albums) PhotoLikers(ctx context.Context, id, cursor string, limit int) (model.LikerPage, error) {
	resp, err := a.api.ListAlbumPhotoLikers(ctx, &postv1.ListAlbumPhotoLikersRequest{PhotoId: id, Cursor: cursor, Limit: toLimit(limit)})
	if err != nil {
		return model.LikerPage{}, errs.Wrap("a.api.ListAlbumPhotoLikers", fromStatus(err))
	}

	return model.LikerPage{UserIDs: resp.GetUserIds(), NextCursor: resp.GetNextCursor(), Total: int(resp.GetTotal())}, nil
}

func (a *Albums) CommentPhoto(ctx context.Context, accessToken, id, text string) (model.PhotoComment, error) {
	resp, err := a.api.CreatePhotoComment(withToken(ctx, accessToken), &postv1.CreatePhotoCommentRequest{PhotoId: id, Text: text})
	if err != nil {
		return model.PhotoComment{}, errs.Wrap("a.api.CreatePhotoComment", fromStatus(err))
	}

	return toPhotoComment(resp.GetComment()), nil
}

func (a *Albums) DeletePhotoComment(ctx context.Context, accessToken, id string) error {
	if _, err := a.api.DeletePhotoComment(withToken(ctx, accessToken), &postv1.DeletePhotoCommentRequest{Id: id}); err != nil {
		return errs.Wrap("a.api.DeletePhotoComment", fromStatus(err))
	}

	return nil
}

// PhotoComments pages the comments of a photo, oldest first.
func (a *Albums) PhotoComments(ctx context.Context, accessToken, id, cursor string, limit int) (model.PhotoCommentPage, error) {
	resp, err := a.api.ListPhotoComments(withToken(ctx, accessToken), &postv1.ListPhotoCommentsRequest{PhotoId: id, Cursor: cursor, Limit: toLimit(limit)})
	if err != nil {
		return model.PhotoCommentPage{}, errs.Wrap("a.api.ListPhotoComments", fromStatus(err))
	}

	page := model.PhotoCommentPage{NextCursor: resp.GetNextCursor(), Total: int(resp.GetTotal())}
	for _, c := range resp.GetComments() {
		page.Comments = append(page.Comments, toPhotoComment(c))
	}

	return page, nil
}

func toPhotoComment(pc *postv1.PhotoComment) model.PhotoComment {
	return model.PhotoComment{
		ID:        pc.GetId(),
		PhotoID:   pc.GetPhotoId(),
		AuthorID:  pc.GetAuthorId(),
		Text:      pc.GetText(),
		CreatedAt: fromTimestamp(pc.GetCreatedAt()),
		Mine:      pc.GetMine(),
	}
}
