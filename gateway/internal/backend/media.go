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
