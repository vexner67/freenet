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

func (h *PostHandler) CreateAlbum(ctx context.Context, req *postv1.CreateAlbumRequest) (*postv1.CreateAlbumResponse, error) {
	requesterID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	ownerID := requesterID
	if req.GetClubId() != "" {
		if ownerID, err = uuid.Parse(req.GetClubId()); err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid club id")
		}
	}

	name, description, err := albumNaming(req.GetName(), req.GetDescription())
	if err != nil {
		return nil, err
	}

	a, err := h.albumService.Create(ctx, service.CreateAlbumInput{
		OwnerID: ownerID, RequesterID: requesterID, Name: name, Description: description,
	})
	if err != nil {
		return nil, err
	}

	return &postv1.CreateAlbumResponse{Album: toProtoAlbum(a, nil, requesterID)}, nil
}

func (h *PostHandler) GetAlbum(ctx context.Context, req *postv1.GetAlbumRequest) (*postv1.GetAlbumResponse, error) {
	id, err := parseAlbumID(req.GetId())
	if err != nil {
		return nil, err
	}

	a, cover, err := h.albumService.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	return &postv1.GetAlbumResponse{Album: toProtoAlbum(a, cover, viewerID(ctx))}, nil
}

func (h *PostHandler) ListAlbums(ctx context.Context, req *postv1.ListAlbumsRequest) (*postv1.ListAlbumsResponse, error) {
	ownerID, err := uuid.Parse(req.GetOwnerId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid owner id")
	}

	before, err := parseCursor(req.GetCursor())
	if err != nil {
		return nil, err
	}

	page, err := h.albumService.List(ctx, ownerID, before, int(req.GetLimit()))
	if err != nil {
		return nil, err
	}

	viewer := viewerID(ctx)
	resp := &postv1.ListAlbumsResponse{
		Albums:     make([]*postv1.Album, 0, len(page.Albums)),
		NextCursor: cursorString(page.NextCursor),
		Total:      int32(page.Total), //nolint:gosec // a count of rows
	}
	for _, a := range page.Albums {
		resp.Albums = append(resp.Albums, toProtoAlbum(a, page.Covers[a.ID()], viewer))
	}

	return resp, nil
}

func (h *PostHandler) EditAlbum(ctx context.Context, req *postv1.EditAlbumRequest) (*postv1.EditAlbumResponse, error) {
	requesterID, id, err := callerAndAlbum(ctx, req.GetId())
	if err != nil {
		return nil, err
	}

	name, description, err := albumNaming(req.GetName(), req.GetDescription())
	if err != nil {
		return nil, err
	}

	a, err := h.albumService.Change(ctx, requesterID, id, name, description)
	if err != nil {
		return nil, err
	}

	return &postv1.EditAlbumResponse{Album: toProtoAlbum(a, nil, requesterID)}, nil
}

func (h *PostHandler) DeleteAlbum(ctx context.Context, req *postv1.DeleteAlbumRequest) (*emptypb.Empty, error) {
	requesterID, id, err := callerAndAlbum(ctx, req.GetId())
	if err != nil {
		return nil, err
	}

	if err = h.albumService.Delete(ctx, requesterID, id); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *PostHandler) AddAlbumPhotos(ctx context.Context, req *postv1.AddAlbumPhotosRequest) (*postv1.AddAlbumPhotosResponse, error) {
	requesterID, albumID, err := callerAndAlbum(ctx, req.GetAlbumId())
	if err != nil {
		return nil, err
	}

	if len(req.GetPhotos()) > service.MaxPhotosAtOnce {
		return nil, status.Error(codes.InvalidArgument, service.ErrTooManyAlbumPhotos.Error())
	}

	in := make([]service.NewAlbumPhoto, 0, len(req.GetPhotos()))
	for _, p := range req.GetPhotos() {
		id, err := uuid.Parse(p.GetPhotoId())
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid photo id")
		}

		description, err := post.NewDescription(p.GetDescription())
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}

		in = append(in, service.NewAlbumPhoto{ID: id, Description: description})
	}

	photos, err := h.albumService.AddPhotos(ctx, requesterID, albumID, in)
	if err != nil {
		return nil, err
	}

	resp := &postv1.AddAlbumPhotosResponse{Photos: make([]*postv1.AlbumPhoto, 0, len(photos))}
	for _, p := range photos {
		resp.Photos = append(resp.Photos, toProtoAlbumPhoto(p, requesterID))
	}

	return resp, nil
}

// AddAvatarPhoto puts a new photo of a page in its "Фотографии со
// страницы".
func (h *PostHandler) AddAvatarPhoto(ctx context.Context, req *postv1.AddAvatarPhotoRequest) (*postv1.AddAvatarPhotoResponse, error) {
	requesterID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	ownerID := requesterID
	if req.GetClubId() != "" {
		if ownerID, err = uuid.Parse(req.GetClubId()); err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid club id")
		}
	}

	photoID, err := uuid.Parse(req.GetPhotoId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid photo id")
	}

	p, err := h.albumService.AddAvatar(ctx, requesterID, ownerID, photoID)
	if err != nil {
		return nil, err
	}

	return &postv1.AddAvatarPhotoResponse{Photo: toProtoAlbumPhoto(p, requesterID)}, nil
}

func (h *PostHandler) ListAlbumPhotos(ctx context.Context, req *postv1.ListAlbumPhotosRequest) (*postv1.ListAlbumPhotosResponse, error) {
	albumID, err := parseAlbumID(req.GetAlbumId())
	if err != nil {
		return nil, err
	}

	after, err := parseCursor(req.GetCursor())
	if err != nil {
		return nil, err
	}

	page, err := h.albumService.Photos(ctx, albumID, after, int(req.GetLimit()))
	if err != nil {
		return nil, err
	}

	viewer := viewerID(ctx)
	resp := &postv1.ListAlbumPhotosResponse{
		Photos:     make([]*postv1.AlbumPhoto, 0, len(page.Photos)),
		NextCursor: cursorString(page.NextCursor),
		Total:      int32(page.Total), //nolint:gosec // a counter from an int4 column
	}
	ids := make([]uuid.UUID, 0, len(page.Photos))
	for _, p := range page.Photos {
		ids = append(ids, p.ID())
	}

	liked, err := h.albumService.LikedPhotos(ctx, viewer, ids)
	if err != nil {
		return nil, err
	}

	for _, p := range page.Photos {
		pp := toProtoAlbumPhoto(p, viewer)
		pp.Liked = liked[p.ID()]
		resp.Photos = append(resp.Photos, pp)
	}

	return resp, nil
}

func (h *PostHandler) GetAlbumPhoto(ctx context.Context, req *postv1.GetAlbumPhotoRequest) (*postv1.GetAlbumPhotoResponse, error) {
	id, err := parsePhotoID(req.GetId())
	if err != nil {
		return nil, err
	}

	p, a, err := h.albumService.Photo(ctx, id)
	if err != nil {
		return nil, err
	}

	viewer := viewerID(ctx)

	liked, err := h.albumService.LikedPhotos(ctx, viewer, []uuid.UUID{p.ID()})
	if err != nil {
		return nil, err
	}

	pp := toProtoAlbumPhoto(p, viewer)
	pp.Liked = liked[p.ID()]

	return &postv1.GetAlbumPhotoResponse{Photo: pp, Album: toProtoAlbum(a, nil, viewer)}, nil
}

func (h *PostHandler) EditAlbumPhoto(ctx context.Context, req *postv1.EditAlbumPhotoRequest) (*postv1.EditAlbumPhotoResponse, error) {
	requesterID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	id, err := parsePhotoID(req.GetId())
	if err != nil {
		return nil, err
	}

	description, err := post.NewDescription(req.GetDescription())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	p, err := h.albumService.Describe(ctx, requesterID, id, description)
	if err != nil {
		return nil, err
	}

	return &postv1.EditAlbumPhotoResponse{Photo: toProtoAlbumPhoto(p, requesterID)}, nil
}

func (h *PostHandler) DeleteAlbumPhoto(ctx context.Context, req *postv1.DeleteAlbumPhotoRequest) (*emptypb.Empty, error) {
	requesterID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	id, err := parsePhotoID(req.GetId())
	if err != nil {
		return nil, err
	}

	if err = h.albumService.DeletePhoto(ctx, requesterID, id); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *PostHandler) SetAlbumCover(ctx context.Context, req *postv1.SetAlbumCoverRequest) (*emptypb.Empty, error) {
	requesterID, albumID, err := callerAndAlbum(ctx, req.GetAlbumId())
	if err != nil {
		return nil, err
	}

	photoID, err := parsePhotoID(req.GetPhotoId())
	if err != nil {
		return nil, err
	}

	if err = h.albumService.SetCover(ctx, requesterID, albumID, photoID); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *PostHandler) MoveAlbumPhoto(ctx context.Context, req *postv1.MoveAlbumPhotoRequest) (*emptypb.Empty, error) {
	requesterID, albumID, err := callerAndAlbum(ctx, req.GetAlbumId())
	if err != nil {
		return nil, err
	}

	photoID, err := parsePhotoID(req.GetPhotoId())
	if err != nil {
		return nil, err
	}

	if err = h.albumService.MovePhoto(ctx, requesterID, photoID, albumID); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func albumNaming(rawName, rawDescription string) (post.AlbumName, post.Description, error) {
	name, err := post.NewAlbumName(rawName)
	if err != nil {
		return "", "", status.Error(codes.InvalidArgument, err.Error())
	}

	description, err := post.NewDescription(rawDescription)
	if err != nil {
		return "", "", status.Error(codes.InvalidArgument, err.Error())
	}

	return name, description, nil
}

func callerAndAlbum(ctx context.Context, rawID string) (uuid.UUID, uuid.UUID, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	id, err := parseAlbumID(rawID)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	return userID, id, nil
}

func parseAlbumID(s string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, status.Error(codes.InvalidArgument, "invalid album id")
	}

	return id, nil
}

func parsePhotoID(s string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, status.Error(codes.InvalidArgument, "invalid photo id")
	}

	return id, nil
}

// toProtoAlbum: cover shows the album, nil if none or not looked up.
var albumKinds = map[post.AlbumKind]postv1.AlbumKind{
	post.AlbumRegular: postv1.AlbumKind_ALBUM_KIND_NONE,
	post.AlbumAvatars: postv1.AlbumKind_ALBUM_KIND_AVATARS,
	post.AlbumWall:    postv1.AlbumKind_ALBUM_KIND_WALL,
}

func toProtoAlbum(a *post.Album, cover *post.AlbumPhoto, viewerID uuid.UUID) *postv1.Album {
	pa := &postv1.Album{
		Id:          a.ID().String(),
		OwnerId:     a.OwnerID().String(),
		Name:        a.Name().String(),
		Description: a.Description().String(),
		CreatedAt:   timestamppb.New(a.CreatedAt()),
		UpdatedAt:   timestamppb.New(a.UpdatedAt()),
		PhotoCount:  int32(a.Photos()), //nolint:gosec // a counter from an int4 column
		Kind:        albumKinds[a.Kind()],
	}
	if cover != nil {
		pa.Cover = toProtoAlbumPhoto(cover, viewerID)
	}

	return pa
}

func toProtoAlbumPhoto(p *post.AlbumPhoto, viewerID uuid.UUID) *postv1.AlbumPhoto {
	return &postv1.AlbumPhoto{
		Id:           p.ID().String(),
		AlbumId:      p.AlbumID().String(),
		AuthorId:     p.AuthorID().String(),
		Description:  p.Description().String(),
		CreatedAt:    timestamppb.New(p.CreatedAt()),
		Mine:         viewerID != uuid.Nil && viewerID == p.AuthorID(),
		LikeCount:    int32(p.Likes()),    //nolint:gosec // a counter from an int4 column
		CommentCount: int32(p.Comments()), //nolint:gosec // a counter from an int4 column
	}
}
