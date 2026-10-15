package fs

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/media/internal/document"
	"github.com/vexner67/freenet/pkg/errs"
)

// DocumentStore lays files out as <root>/docs/<owner>/<document>. Paths
// are built from UUIDs only, never from the names people give files.
type DocumentStore struct {
	root string
}

func NewDocumentStore(root string) (*DocumentStore, error) {
	dir := filepath.Join(root, "docs")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, errs.Wrap("os.MkdirAll", err)
	}

	return &DocumentStore{root: dir}, nil
}

func (s *DocumentStore) ownerDir(ownerID uuid.UUID) string {
	return filepath.Join(s.root, ownerID.String())
}

// Save writes into a temporary file and renames it, so a reader never sees
// half of a document.
func (s *DocumentStore) Save(_ context.Context, ownerID, documentID uuid.UUID, data []byte) error {
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

	return errs.Wrap("os.Rename", os.Rename(tmp.Name(), filepath.Join(dir, documentID.String())))
}

func (s *DocumentStore) Load(_ context.Context, ownerID, documentID uuid.UUID) ([]byte, error) {
	data, err := os.ReadFile(filepath.Join(s.ownerDir(ownerID), documentID.String()))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, document.ErrNotFound
	}
	if err != nil {
		return nil, errs.Wrap("os.ReadFile", err)
	}

	return data, nil
}

func (s *DocumentStore) Delete(_ context.Context, ownerID uuid.UUID, documentIDs []uuid.UUID) error {
	for _, id := range documentIDs {
		err := os.Remove(filepath.Join(s.ownerDir(ownerID), id.String()))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return errs.Wrap("os.Remove", err)
		}
	}

	return nil
}

func (s *DocumentStore) DeleteAll(_ context.Context, ownerID uuid.UUID) error {
	return errs.Wrap("os.RemoveAll", os.RemoveAll(s.ownerDir(ownerID)))
}
