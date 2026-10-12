package s3

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/media/internal/avatar"
)

// AvatarStore lays objects out as avatars/<user>/<avatar>/<size>.jpg.
type AvatarStore struct {
	bucket *Bucket
}

func NewAvatarStore(bucket *Bucket) *AvatarStore {
	return &AvatarStore{bucket: bucket}
}

func avatarKey(userID, avatarID uuid.UUID, size avatar.Size) string {
	return "avatars/" + userID.String() + "/" + avatarID.String() + "/" + string(size) + ".jpg"
}

func (s *AvatarStore) Save(ctx context.Context, userID, avatarID uuid.UUID, images avatar.Images) error {
	for _, size := range []avatar.Size{avatar.Small, avatar.Large} {
		if err := s.bucket.put(ctx, avatarKey(userID, avatarID, size), images.Of(size)); err != nil {
			return err
		}
	}

	return nil
}

func (s *AvatarStore) Load(ctx context.Context, userID, avatarID uuid.UUID, size avatar.Size) ([]byte, error) {
	if size != avatar.Large && size != avatar.Small {
		return nil, avatar.ErrNotFound
	}

	data, ok, err := s.bucket.get(ctx, avatarKey(userID, avatarID, size))
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, avatar.ErrNotFound
	}

	return data, nil
}

func (s *AvatarStore) DeleteExcept(ctx context.Context, userID, keep uuid.UUID) error {
	prefix := "avatars/" + userID.String() + "/"

	objects, err := s.bucket.list(ctx, prefix)
	if err != nil {
		return err
	}

	for _, o := range objects {
		if keep != uuid.Nil && strings.HasPrefix(o.key, prefix+keep.String()+"/") {
			continue
		}

		if err = s.bucket.remove(ctx, o.key); err != nil {
			return err
		}
	}

	return nil
}
