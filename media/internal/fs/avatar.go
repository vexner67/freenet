// Package fs keeps media in a directory: one node, a mounted volume in
// Docker. Package s3 keeps it for several, behind the same ports.
package fs

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/media/internal/avatar"
	"github.com/vexner67/freenet/pkg/errs"
)

// AvatarStore lays files out as <root>/avatars/<user>/<avatar>/<size>.jpg.
// Paths are built from UUIDs and fixed names only, never from user input.
type AvatarStore struct {
	root string
}

func NewAvatarStore(root string) (*AvatarStore, error) {
	dir := filepath.Join(root, "avatars")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, errs.Wrap("os.MkdirAll", err)
	}

	return &AvatarStore{root: dir}, nil
}

func (s *AvatarStore) userDir(userID uuid.UUID) string {
	return filepath.Join(s.root, userID.String())
}

func (s *AvatarStore) Save(_ context.Context, userID, avatarID uuid.UUID, images avatar.Images) error {
	return saveDir(filepath.Join(s.userDir(userID), avatarID.String()), map[string][]byte{
		string(avatar.Large) + ".jpg": images.Of(avatar.Large),
		string(avatar.Small) + ".jpg": images.Of(avatar.Small),
	})
}

func (s *AvatarStore) Load(_ context.Context, userID, avatarID uuid.UUID, size avatar.Size) ([]byte, error) {
	if size != avatar.Large && size != avatar.Small {
		return nil, avatar.ErrNotFound
	}

	data, err := os.ReadFile(filepath.Join(s.userDir(userID), avatarID.String(), string(size)+".jpg"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, avatar.ErrNotFound
	}
	if err != nil {
		return nil, errs.Wrap("os.ReadFile", err)
	}

	return data, nil
}

func (s *AvatarStore) DeleteExcept(_ context.Context, userID, keep uuid.UUID) error {
	// Nothing to keep: the user's directory goes too.
	if keep == uuid.Nil {
		if err := os.RemoveAll(s.userDir(userID)); err != nil {
			return errs.Wrap("os.RemoveAll", err)
		}
		return nil
	}

	entries, err := os.ReadDir(s.userDir(userID))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return errs.Wrap("os.ReadDir", err)
	}

	for _, e := range entries {
		if keep != uuid.Nil && e.Name() == keep.String() {
			continue
		}

		if err = os.RemoveAll(filepath.Join(s.userDir(userID), e.Name())); err != nil {
			return errs.Wrap("os.RemoveAll", err)
		}
	}

	return nil
}
