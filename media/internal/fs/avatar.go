// Package fs keeps media in a directory: one node, a mounted volume in
// Docker. An S3 store can replace it behind the same port.
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
	final := filepath.Join(s.userDir(userID), avatarID.String())

	// Write into a temporary directory and rename it, so a reader never
	// sees half an avatar.
	tmp, err := os.MkdirTemp(filepath.Dir(final), ".upload-*")
	if errors.Is(err, fs.ErrNotExist) {
		if err = os.MkdirAll(filepath.Dir(final), 0o750); err == nil {
			tmp, err = os.MkdirTemp(filepath.Dir(final), ".upload-*")
		}
	}
	if err != nil {
		return errs.Wrap("os.MkdirTemp", err)
	}

	for _, size := range []avatar.Size{avatar.Large, avatar.Small} {
		if err = os.WriteFile(filepath.Join(tmp, string(size)+".jpg"), images.Of(size), 0o640); err != nil {
			_ = os.RemoveAll(tmp)
			return errs.Wrap("os.WriteFile", err)
		}
	}

	if err = os.Rename(tmp, final); err != nil {
		_ = os.RemoveAll(tmp)
		return errs.Wrap("os.Rename", err)
	}

	return nil
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
