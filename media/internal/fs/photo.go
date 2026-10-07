package fs

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/media/internal/photo"
	"github.com/vexner67/freenet/pkg/errs"
)

// PhotoStore lays files out as <root>/photos/<owner>/<photo>/<size>.jpg.
// Paths are built from UUIDs and fixed names only, never from user input.
type PhotoStore struct {
	root string
}

func NewPhotoStore(root string) (*PhotoStore, error) {
	dir := filepath.Join(root, "photos")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, errs.Wrap("os.MkdirAll", err)
	}

	return &PhotoStore{root: dir}, nil
}

func (s *PhotoStore) dir(ownerID, photoID uuid.UUID) string {
	return filepath.Join(s.root, ownerID.String(), photoID.String())
}

func (s *PhotoStore) Save(_ context.Context, ownerID, photoID uuid.UUID, images photo.Images) error {
	return saveDir(s.dir(ownerID, photoID), map[string][]byte{
		string(photo.Large) + ".jpg": images.Of(photo.Large),
		string(photo.Small) + ".jpg": images.Of(photo.Small),
	})
}

func (s *PhotoStore) Load(_ context.Context, ownerID, photoID uuid.UUID, size photo.Size) ([]byte, error) {
	if size != photo.Large && size != photo.Small {
		return nil, photo.ErrNotFound
	}

	data, err := os.ReadFile(filepath.Join(s.dir(ownerID, photoID), string(size)+".jpg"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, photo.ErrNotFound
	}
	if err != nil {
		return nil, errs.Wrap("os.ReadFile", err)
	}

	return data, nil
}

func (s *PhotoStore) Existing(_ context.Context, ownerID uuid.UUID, photoIDs []uuid.UUID) ([]uuid.UUID, error) {
	var found []uuid.UUID

	for _, id := range photoIDs {
		_, err := os.Stat(s.dir(ownerID, id))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, errs.Wrap("os.Stat", err)
		}

		found = append(found, id)
	}

	return found, nil
}

func (s *PhotoStore) Delete(_ context.Context, ownerID uuid.UUID, photoIDs []uuid.UUID) error {
	for _, id := range photoIDs {
		if err := os.RemoveAll(s.dir(ownerID, id)); err != nil {
			return errs.Wrap("os.RemoveAll", err)
		}
	}

	return nil
}

func (s *PhotoStore) DeleteAll(_ context.Context, ownerID uuid.UUID) error {
	if err := os.RemoveAll(filepath.Join(s.root, ownerID.String())); err != nil {
		return errs.Wrap("os.RemoveAll", err)
	}

	return nil
}
