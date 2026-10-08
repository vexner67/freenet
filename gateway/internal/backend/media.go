package backend

import (
	"context"

	mediav1 "github.com/vexner67/freenet/media/api/media/v1"
	"github.com/vexner67/freenet/pkg/errs"
	"google.golang.org/grpc"
)

// MaxAvatarBytes is the largest upload the media service accepts.
const MaxAvatarBytes = 8 << 20

// maxMessageBytes lets an upload through; gRPC allows 4 MB by default.
const maxMessageBytes = MaxAvatarBytes + 1<<20

type Media struct {
	api mediav1.MediaServiceClient
}

func NewMedia(api mediav1.MediaServiceClient) *Media {
	return &Media{
		api: api,
	}
}

func (m *Media) UploadAvatar(ctx context.Context, accessToken string, image []byte) (string, error) {
	resp, err := m.api.UploadAvatar(
		withToken(ctx, accessToken),
		&mediav1.UploadAvatarRequest{Image: image},
		grpc.MaxCallSendMsgSize(maxMessageBytes),
	)
	if err != nil {
		return "", errs.Wrap("m.api.UploadAvatar", fromStatus(err))
	}

	return resp.GetAvatarId(), nil
}

// DeleteAvatars removes the caller's avatars except keep ("" removes all).
func (m *Media) DeleteAvatars(ctx context.Context, accessToken, keep string) error {
	if _, err := m.api.DeleteAvatars(withToken(ctx, accessToken), &mediav1.DeleteAvatarsRequest{KeepAvatarId: keep}); err != nil {
		return errs.Wrap("m.api.DeleteAvatars", fromStatus(err))
	}

	return nil
}

// UploadClubAvatar stores a photo of a club, which the caller runs, as the
// gateway has checked.
func (m *Media) UploadClubAvatar(ctx context.Context, accessToken, clubID string, image []byte) (string, error) {
	resp, err := m.api.UploadClubAvatar(
		withToken(ctx, accessToken),
		&mediav1.UploadClubAvatarRequest{ClubId: clubID, Image: image},
		grpc.MaxCallSendMsgSize(maxMessageBytes),
	)
	if err != nil {
		return "", errs.Wrap("m.api.UploadClubAvatar", fromStatus(err))
	}

	return resp.GetAvatarId(), nil
}

func (m *Media) DeleteClubAvatars(ctx context.Context, accessToken, clubID, keep string) error {
	_, err := m.api.DeleteClubAvatars(withToken(ctx, accessToken), &mediav1.DeleteClubAvatarsRequest{ClubId: clubID, KeepAvatarId: keep})
	if err != nil {
		return errs.Wrap("m.api.DeleteClubAvatars", fromStatus(err))
	}

	return nil
}

// Avatar returns the JPEG of an avatar, small (100px) or large (400px).
func (m *Media) Avatar(ctx context.Context, userID, avatarID string, small bool) ([]byte, error) {
	size := mediav1.AvatarSize_AVATAR_SIZE_LARGE
	if small {
		size = mediav1.AvatarSize_AVATAR_SIZE_SMALL
	}

	resp, err := m.api.GetAvatar(ctx, &mediav1.GetAvatarRequest{UserId: userID, AvatarId: avatarID, Size: size})
	if err != nil {
		return nil, errs.Wrap("m.api.GetAvatar", fromStatus(err))
	}

	return resp.GetImage(), nil
}

// UploadPhoto stores a photo of the caller for a post and returns its ID.
func (m *Media) UploadPhoto(ctx context.Context, accessToken string, image []byte) (string, error) {
	resp, err := m.api.UploadPhoto(
		withToken(ctx, accessToken),
		&mediav1.UploadPhotoRequest{Image: image},
		grpc.MaxCallSendMsgSize(maxMessageBytes),
	)
	if err != nil {
		return "", errs.Wrap("m.api.UploadPhoto", fromStatus(err))
	}

	return resp.GetPhotoId(), nil
}

// OwnPhotos returns which of photoIDs the caller has.
func (m *Media) OwnPhotos(ctx context.Context, accessToken string, photoIDs []string) ([]string, error) {
	resp, err := m.api.ListOwnPhotos(withToken(ctx, accessToken), &mediav1.ListOwnPhotosRequest{PhotoIds: photoIDs})
	if err != nil {
		return nil, errs.Wrap("m.api.ListOwnPhotos", fromStatus(err))
	}

	return resp.GetPhotoIds(), nil
}

// Photo returns the JPEG of a photo, small (up to 640px) or large (1280px).
func (m *Media) Photo(ctx context.Context, userID, photoID string, small bool) ([]byte, error) {
	size := mediav1.PhotoSize_PHOTO_SIZE_LARGE
	if small {
		size = mediav1.PhotoSize_PHOTO_SIZE_SMALL
	}

	resp, err := m.api.GetPhoto(ctx, &mediav1.GetPhotoRequest{UserId: userID, PhotoId: photoID, Size: size})
	if err != nil {
		return nil, errs.Wrap("m.api.GetPhoto", fromStatus(err))
	}

	return resp.GetImage(), nil
}
