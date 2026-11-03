package fs

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/media/internal/audio"
	"github.com/vexner67/freenet/media/internal/document"
	"github.com/vexner67/freenet/pkg/errs"
)

// FileStore lays files out as <root>/<folder>/<owner>/<file>: documents
// in docs, audio in audio. Paths are built from UUIDs only, never from
// the names people give files.
type FileStore struct {
	root string
	// missing is what Load returns for a file that is not there.
	missing error
}

func NewDocumentStore(root string) (*FileStore, error) {
	return newFileStore(root, "docs", document.ErrNotFound)
}

func NewAudioStore(root string) (*FileStore, error) {
	return newFileStore(root, "audio", audio.ErrNotFound)
}

func newFileStore(root, folder string, missing error) (*FileStore, error) {
	dir := filepath.Join(root, folder)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, errs.Wrap("os.MkdirAll", err)
	}

	return &FileStore{root: dir, missing: missing}, nil
}

func (s *FileStore) ownerDir(ownerID uuid.UUID) string {
	return filepath.Join(s.root, ownerID.String())
}

// Save writes into a temporary file and renames it, so a reader never sees
// half of a file.
func (s *FileStore) Save(_ context.Context, ownerID, fileID uuid.UUID, data []byte) error {
	dir := s.ownerDir(ownerID)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return errs.Wrap("os.MkdirAll", err)
	}

	tmp, err := os.CreateTemp(dir, ".upload-*")
	if err != nil {
		return errs.Wrap("os.CreateTemp", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }() // gone already when renamed

	if _, err = tmp.Write(data); err != nil {
		_ = tmp.Close()
		return errs.Wrap("tmp.Write", err)
	}
	if err = tmp.Close(); err != nil {
		return errs.Wrap("tmp.Close", err)
	}

	return errs.Wrap("os.Rename", os.Rename(tmp.Name(), filepath.Join(dir, fileID.String())))
}

func (s *FileStore) Load(_ context.Context, ownerID, fileID uuid.UUID) ([]byte, error) {
	data, err := os.ReadFile(filepath.Join(s.ownerDir(ownerID), fileID.String()))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, s.missing
	}
	if err != nil {
		return nil, errs.Wrap("os.ReadFile", err)
	}

	return data, nil
}

func (s *FileStore) Delete(_ context.Context, ownerID uuid.UUID, fileIDs []uuid.UUID) error {
	for _, id := range fileIDs {
		err := os.Remove(filepath.Join(s.ownerDir(ownerID), id.String()))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return errs.Wrap("os.Remove", err)
		}
	}

	return nil
}

func (s *FileStore) DeleteAll(_ context.Context, ownerID uuid.UUID) error {
	return errs.Wrap("os.RemoveAll", os.RemoveAll(s.ownerDir(ownerID)))
}
