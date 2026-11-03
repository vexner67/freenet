package s3

import (
	"context"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/media/internal/audio"
	"github.com/vexner67/freenet/media/internal/document"
)

// FileStore lays objects out as <folder>/<owner>/<file>: documents in
// docs, audio in audio.
type FileStore struct {
	bucket *Bucket
	folder string
	// missing is what Load returns for a file that is not there.
	missing error
}

func NewDocumentStore(bucket *Bucket) *FileStore {
	return &FileStore{bucket: bucket, folder: "docs", missing: document.ErrNotFound}
}

func NewAudioStore(bucket *Bucket) *FileStore {
	return &FileStore{bucket: bucket, folder: "audio", missing: audio.ErrNotFound}
}

func (s *FileStore) key(ownerID, fileID uuid.UUID) string {
	return s.folder + "/" + ownerID.String() + "/" + fileID.String()
}

func (s *FileStore) Save(ctx context.Context, ownerID, fileID uuid.UUID, data []byte) error {
	return s.bucket.put(ctx, s.key(ownerID, fileID), data)
}

func (s *FileStore) Load(ctx context.Context, ownerID, fileID uuid.UUID) ([]byte, error) {
	data, ok, err := s.bucket.get(ctx, s.key(ownerID, fileID))
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, s.missing
	}

	return data, nil
}

func (s *FileStore) Delete(ctx context.Context, ownerID uuid.UUID, fileIDs []uuid.UUID) error {
	keys := make([]string, len(fileIDs))
	for i, id := range fileIDs {
		keys[i] = s.key(ownerID, id)
	}

	return s.bucket.remove(ctx, keys...)
}

func (s *FileStore) DeleteAll(ctx context.Context, ownerID uuid.UUID) error {
	objects, err := s.bucket.list(ctx, s.folder+"/"+ownerID.String()+"/")
	if err != nil {
		return err
	}

	keys := make([]string, len(objects))
	for i, o := range objects {
		keys[i] = o.key
	}

	return s.bucket.remove(ctx, keys...)
}
