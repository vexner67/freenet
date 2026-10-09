package grpc

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	mediav1 "github.com/vexner67/freenet/media/api/media/v1"
	"github.com/vexner67/freenet/media/internal/avatar"
	"github.com/vexner67/freenet/media/internal/photo"
	"github.com/vexner67/freenet/media/internal/service"
	"github.com/vexner67/freenet/pkg/jwtauth"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type AvatarService interface {
	Upload(ctx context.Context, userID uuid.UUID, data []byte) (uuid.UUID, error)
	Get(ctx context.Context, userID, avatarID uuid.UUID, size avatar.Size) ([]byte, error)
	DeleteExcept(ctx context.Context, userID, keep uuid.UUID) error
}

type PhotoService interface {
	Upload(ctx context.Context, ownerID uuid.UUID, data []byte) (uuid.UUID, error)
	Get(ctx context.Context, ownerID, photoID uuid.UUID, size photo.Size) ([]byte, error)
	Existing(ctx context.Context, ownerID uuid.UUID, photoIDs []uuid.UUID) ([]uuid.UUID, error)
	Publish(ctx context.Context, ownerID uuid.UUID, photoIDs []uuid.UUID) error
	Delete(ctx context.Context, ownerID uuid.UUID, photoIDs []uuid.UUID) error
	Copy(ctx context.Context, ownerID uuid.UUID, copies []service.PhotoCopy) error
	UploadKept(ctx context.Context, ownerID uuid.UUID, data []byte) (uuid.UUID, error)
	DeleteExcept(ctx context.Context, ownerID uuid.UUID, keep []uuid.UUID) error
}

// PurgeService erases a user's files once the page is deleted for good.
type PurgeService interface {
	PurgeUser(ctx context.Context, userID uuid.UUID) error
}

type MediaHandler struct {
	mediav1.UnimplementedMediaServiceServer
	avatars AvatarService
	photos  PhotoService
	purges  PurgeService
	logger  *slog.Logger
}

func NewMediaHandler(avatars AvatarService, photos PhotoService, purges PurgeService, logger *slog.Logger) *MediaHandler {
	return &MediaHandler{
		avatars: avatars,
		photos:  photos,
		purges:  purges,
		logger:  logger,
	}
}

func (h *MediaHandler) PurgeUser(ctx context.Context, req *mediav1.PurgeUserRequest) (*emptypb.Empty, error) {
	userID, err := uuid.Parse(req.GetUserId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user id")
	}

	if err = h.purges.PurgeUser(ctx, userID); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *MediaHandler) UploadAvatar(ctx context.Context, req *mediav1.UploadAvatarRequest) (*mediav1.UploadAvatarResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	id, err := h.avatars.Upload(ctx, userID, req.GetImage())
	if err != nil {
		return nil, err
	}

	return &mediav1.UploadAvatarResponse{AvatarId: id.String()}, nil
}

func (h *MediaHandler) DeleteAvatars(ctx context.Context, req *mediav1.DeleteAvatarsRequest) (*emptypb.Empty, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	var keep uuid.UUID
	if req.GetKeepAvatarId() != "" {
		if keep, err = parseID(req.GetKeepAvatarId(), "avatar id"); err != nil {
			return nil, err
		}
	}

	if err = h.avatars.DeleteExcept(ctx, userID, keep); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *MediaHandler) UploadClubAvatar(ctx context.Context, req *mediav1.UploadClubAvatarRequest) (*mediav1.UploadAvatarResponse, error) {
	clubID, err := parseID(req.GetClubId(), "club id")
	if err != nil {
		return nil, err
	}

	id, err := h.avatars.Upload(ctx, clubID, req.GetImage())
	if err != nil {
		return nil, err
	}

	return &mediav1.UploadAvatarResponse{AvatarId: id.String()}, nil
}

func (h *MediaHandler) DeleteClubAvatars(ctx context.Context, req *mediav1.DeleteClubAvatarsRequest) (*emptypb.Empty, error) {
	clubID, err := parseID(req.GetClubId(), "club id")
	if err != nil {
		return nil, err
	}

	var keep uuid.UUID
	if req.GetKeepAvatarId() != "" {
		if keep, err = parseID(req.GetKeepAvatarId(), "avatar id"); err != nil {
			return nil, err
		}
	}

	if err = h.avatars.DeleteExcept(ctx, clubID, keep); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *MediaHandler) UploadClubPhoto(ctx context.Context, req *mediav1.UploadClubPhotoRequest) (*mediav1.UploadPhotoResponse, error) {
	clubID, err := parseID(req.GetClubId(), "club id")
	if err != nil {
		return nil, err
	}

	id, err := h.photos.UploadKept(ctx, clubID, req.GetImage())
	if err != nil {
		return nil, err
	}

	return &mediav1.UploadPhotoResponse{PhotoId: id.String()}, nil
}

func (h *MediaHandler) DeleteClubPhotos(ctx context.Context, req *mediav1.DeleteClubPhotosRequest) (*emptypb.Empty, error) {
	clubID, err := parseID(req.GetClubId(), "club id")
	if err != nil {
		return nil, err
	}

	keep := make([]uuid.UUID, 0, len(req.GetKeepPhotoIds()))
	for _, raw := range req.GetKeepPhotoIds() {
		id, err := parseID(raw, "photo id")
		if err != nil {
			return nil, err
		}
		keep = append(keep, id)
	}

	if err = h.photos.DeleteExcept(ctx, clubID, keep); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *MediaHandler) GetAvatar(ctx context.Context, req *mediav1.GetAvatarRequest) (*mediav1.GetAvatarResponse, error) {
	userID, err := parseID(req.GetUserId(), "user id")
	if err != nil {
		return nil, err
	}

	avatarID, err := parseID(req.GetAvatarId(), "avatar id")
	if err != nil {
		return nil, err
	}

	size := avatar.Large
	if req.GetSize() == mediav1.AvatarSize_AVATAR_SIZE_SMALL {
		size = avatar.Small
	}

	data, err := h.avatars.Get(ctx, userID, avatarID, size)
	if err != nil {
		return nil, err
	}

	return &mediav1.GetAvatarResponse{Image: data}, nil
}

func (h *MediaHandler) UploadPhoto(ctx context.Context, req *mediav1.UploadPhotoRequest) (*mediav1.UploadPhotoResponse, error) {
	ownerID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	id, err := h.photos.Upload(ctx, ownerID, req.GetImage())
	if err != nil {
		return nil, err
	}

	return &mediav1.UploadPhotoResponse{PhotoId: id.String()}, nil
}

func (h *MediaHandler) ListOwnPhotos(ctx context.Context, req *mediav1.ListOwnPhotosRequest) (*mediav1.ListOwnPhotosResponse, error) {
	ownerID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	ids, err := parsePhotoIDs(req.GetPhotoIds())
	if err != nil {
		return nil, err
	}

	found, err := h.photos.Existing(ctx, ownerID, ids)
	if err != nil {
		return nil, err
	}

	resp := &mediav1.ListOwnPhotosResponse{PhotoIds: make([]string, 0, len(found))}
	for _, id := range found {
		resp.PhotoIds = append(resp.PhotoIds, id.String())
	}

	return resp, nil
}

// PublishPhotos and DeletePublishedPhotos are internal, for post, message
// and the gateway (the backdrop of a person): the owner comes in the
// request, not from a token.
func (h *MediaHandler) PublishPhotos(ctx context.Context, req *mediav1.PublishPhotosRequest) (*emptypb.Empty, error) {
	ownerID, ids, err := parseOwnerPhotos(req.GetOwnerId(), req.GetPhotoIds())
	if err != nil {
		return nil, err
	}

	if err = h.photos.Publish(ctx, ownerID, ids); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *MediaHandler) DeletePublishedPhotos(ctx context.Context, req *mediav1.DeletePublishedPhotosRequest) (*emptypb.Empty, error) {
	ownerID, ids, err := parseOwnerPhotos(req.GetOwnerId(), req.GetPhotoIds())
	if err != nil {
		return nil, err
	}

	if err = h.photos.Delete(ctx, ownerID, ids); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

// CopyPhotos is internal, for post: the photos of a post, kept once more
// for its author's "Фотографии со стены".
func (h *MediaHandler) CopyPhotos(ctx context.Context, req *mediav1.CopyPhotosRequest) (*emptypb.Empty, error) {
	ownerID, err := parseID(req.GetOwnerId(), "owner id")
	if err != nil {
		return nil, err
	}

	copies := make([]service.PhotoCopy, 0, len(req.GetCopies()))
	for _, c := range req.GetCopies() {
		from, err := parseID(c.GetFromId(), "photo id")
		if err != nil {
			return nil, err
		}

		to, err := parseID(c.GetToId(), "photo id")
		if err != nil {
			return nil, err
		}

		copies = append(copies, service.PhotoCopy{From: from, To: to})
	}

	if err = h.photos.Copy(ctx, ownerID, copies); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func parseOwnerPhotos(owner string, photos []string) (uuid.UUID, []uuid.UUID, error) {
	ownerID, err := parseID(owner, "owner id")
	if err != nil {
		return uuid.Nil, nil, err
	}

	ids, err := parsePhotoIDs(photos)
	if err != nil {
		return uuid.Nil, nil, err
	}

	return ownerID, ids, nil
}

func (h *MediaHandler) GetPhoto(ctx context.Context, req *mediav1.GetPhotoRequest) (*mediav1.GetPhotoResponse, error) {
	ownerID, err := parseID(req.GetUserId(), "user id")
	if err != nil {
		return nil, err
	}

	photoID, err := parseID(req.GetPhotoId(), "photo id")
	if err != nil {
		return nil, err
	}

	size := photo.Large
	if req.GetSize() == mediav1.PhotoSize_PHOTO_SIZE_SMALL {
		size = photo.Small
	}

	data, err := h.photos.Get(ctx, ownerID, photoID, size)
	if err != nil {
		return nil, err
	}

	return &mediav1.GetPhotoResponse{Image: data}, nil
}

// parsePhotoIDs reads at most service.MaxPhotoIDs IDs.
func parsePhotoIDs(raw []string) ([]uuid.UUID, error) {
	if len(raw) > service.MaxPhotoIDs {
		return nil, status.Error(codes.InvalidArgument, "too many photo ids")
	}

	ids := make([]uuid.UUID, 0, len(raw))
	for _, s := range raw {
		id, err := parseID(s, "photo id")
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}

	return ids, nil
}

func parseID(s, what string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, status.Error(codes.InvalidArgument, "invalid "+what)
	}

	return id, nil
}

// currentUserID is the caller's ID from the access token verified by
// jwtauth.UnaryServerInterceptor; never trust an ID from the request body.
func currentUserID(ctx context.Context) (uuid.UUID, error) {
	identity, ok := jwtauth.IdentityFromContext(ctx)
	if !ok {
		return uuid.Nil, status.Error(codes.Unauthenticated, "missing access token")
	}

	return identity.UserID, nil
}
