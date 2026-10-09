package fs

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/media/internal/photo"
	"github.com/vexner67/freenet/pkg/errs"
)

// PhotoStore lays files out as <root>/photos/<owner>/<photo>/<size>.jpg,
// and photos not published yet the same way under <root>/uploads, so that
// finding the unpublished ones does not walk every photo. Paths are built
// from UUIDs and fixed names only, never from user input.
type PhotoStore struct {
	published string
	uploads   string
}

func NewPhotoStore(root string) (*PhotoStore, error) {
	s := &PhotoStore{
		published: filepath.Join(root, "photos"),
		uploads:   filepath.Join(root, "uploads"),
	}

	for _, dir := range []string{s.published, s.uploads} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, errs.Wrap("os.MkdirAll", err)
		}
	}

	return s, nil
}

// dirs are where a photo may be: published or not.
func (s *PhotoStore) dirs(ownerID, photoID uuid.UUID) [2]string {
	return [2]string{
		filepath.Join(s.published, ownerID.String(), photoID.String()),
		filepath.Join(s.uploads, ownerID.String(), photoID.String()),
	}
}

func (s *PhotoStore) Save(_ context.Context, ownerID, photoID uuid.UUID, images photo.Images) error {
	return saveDir(s.dirs(ownerID, photoID)[1], map[string][]byte{
		string(photo.Large) + ".jpg": images.Of(photo.Large),
		string(photo.Small) + ".jpg": images.Of(photo.Small),
	})
}

func (s *PhotoStore) Load(_ context.Context, ownerID, photoID uuid.UUID, size photo.Size) ([]byte, error) {
	if size != photo.Large && size != photo.Small {
		return nil, photo.ErrNotFound
	}

	// Published first: being published moves the photo there.
	for _, dir := range s.dirs(ownerID, photoID) {
		data, err := os.ReadFile(filepath.Join(dir, string(size)+".jpg"))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, errs.Wrap("os.ReadFile", err)
		}

		return data, nil
	}

	return nil, photo.ErrNotFound
}

func (s *PhotoStore) Existing(_ context.Context, ownerID uuid.UUID, photoIDs []uuid.UUID) ([]uuid.UUID, error) {
	var found []uuid.UUID

	for _, id := range photoIDs {
		for _, dir := range s.dirs(ownerID, id) {
			ok, err := exists(dir)
			if err != nil {
				return nil, err
			}
			if ok {
				found = append(found, id)
				break
			}
		}
	}

	return found, nil
}

func exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, errs.Wrap("os.Stat", err)
	}

	return true, nil
}

// Publish moves the photos out of the uploads; a rename, so a reader finds
// a photo in one place or the other.
func (s *PhotoStore) Publish(_ context.Context, ownerID uuid.UUID, photoIDs []uuid.UUID) error {
	if err := os.MkdirAll(filepath.Join(s.published, ownerID.String()), 0o750); err != nil {
		return errs.Wrap("os.MkdirAll", err)
	}

	for _, id := range photoIDs {
		dirs := s.dirs(ownerID, id)
		// Missing: published already, or deleted.
		if err := os.Rename(dirs[1], dirs[0]); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return errs.Wrap("os.Rename", err)
		}
	}

	return nil
}

// Copy links the files of from into a new published photo: the images
// never change, so both photos share them on disk until one is deleted.
func (s *PhotoStore) Copy(_ context.Context, ownerID, from, to uuid.UUID) error {
	final := s.dirs(ownerID, to)[0]
	if ok, err := exists(final); err != nil || ok {
		return err
	}

	for _, dir := range s.dirs(ownerID, from) {
		err := linkDir(dir, final, string(photo.Large)+".jpg", string(photo.Small)+".jpg")
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}

		return err
	}

	return nil
}

func (s *PhotoStore) Delete(_ context.Context, ownerID uuid.UUID, photoIDs []uuid.UUID) error {
	for _, id := range photoIDs {
		for _, dir := range s.dirs(ownerID, id) {
			if err := os.RemoveAll(dir); err != nil {
				return errs.Wrap("os.RemoveAll", err)
			}
		}
	}

	return nil
}

func (s *PhotoStore) DeleteAll(_ context.Context, ownerID uuid.UUID) error {
	for _, root := range []string{s.published, s.uploads} {
		if err := os.RemoveAll(filepath.Join(root, ownerID.String())); err != nil {
			return errs.Wrap("os.RemoveAll", err)
		}
	}

	return nil
}

func (s *PhotoStore) DeleteExcept(_ context.Context, ownerID uuid.UUID, keep []uuid.UUID) error {
	dir := filepath.Join(s.published, ownerID.String())

	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return errs.Wrap("os.ReadDir", err)
	}

	for _, e := range entries {
		if id, err := uuid.Parse(e.Name()); err == nil && slices.Contains(keep, id) {
			continue
		}

		if err = os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return errs.Wrap("os.RemoveAll", err)
		}
	}

	return nil
}

// DeleteUnpublished deletes the uploads older than before: by the time in
// their UUIDv7, or by the modification time for leftovers of failed saves.
// An owner's directory left empty goes too.
func (s *PhotoStore) DeleteUnpublished(ctx context.Context, before time.Time) (int, error) {
	owners, err := os.ReadDir(s.uploads)
	if err != nil {
		return 0, errs.Wrap("os.ReadDir", err)
	}

	deleted := 0

	for _, owner := range owners {
		if err = ctx.Err(); err != nil {
			return deleted, errs.Wrap("ctx.Err", err)
		}

		n, err := deleteOlder(filepath.Join(s.uploads, owner.Name()), before)
		deleted += n
		if err != nil {
			return deleted, err
		}
	}

	return deleted, nil
}

func deleteOlder(dir string, before time.Time) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, errs.Wrap("os.ReadDir", err)
	}

	deleted, removed := 0, 0

	for _, e := range entries {
		uploaded, err := uploadTime(e)
		if err != nil {
			return deleted, err
		}
		if !uploaded.Before(before) {
			continue
		}

		if err = os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return deleted, errs.Wrap("os.RemoveAll", err)
		}
		removed++
		if _, err = uuid.Parse(e.Name()); err == nil {
			deleted++
		}
	}

	// Fails while something is in it, a photo saved meanwhile included.
	if removed == len(entries) {
		_ = os.Remove(dir)
	}

	return deleted, nil
}

func uploadTime(e os.DirEntry) (time.Time, error) {
	if id, err := uuid.Parse(e.Name()); err == nil && id.Version() == 7 {
		return time.Unix(id.Time().UnixTime()), nil
	}

	info, err := e.Info()
	if err != nil {
		return time.Time{}, errs.Wrap("e.Info", err)
	}

	return info.ModTime(), nil
}
