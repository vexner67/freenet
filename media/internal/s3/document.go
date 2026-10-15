package s3

import (
	"context"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/media/internal/document"
)

// DocumentStore lays objects out as docs/<owner>/<document>.
type DocumentStore struct {
	bucket *Bucket
}

func NewDocumentStore(bucket *Bucket) *DocumentStore {
	return &DocumentStore{bucket: bucket}
}

func documentKey(ownerID, documentID uuid.UUID) string {
	return "docs/" + ownerID.String() + "/" + documentID.String()
}

func (s *DocumentStore) Save(ctx context.Context, ownerID, documentID uuid.UUID, data []byte) error {
	return s.bucket.put(ctx, documentKey(ownerID, documentID), data)
}

func (s *DocumentStore) Load(ctx context.Context, ownerID, documentID uuid.UUID) ([]byte, error) {
	data, ok, err := s.bucket.get(ctx, documentKey(ownerID, documentID))
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, document.ErrNotFound
	}

	return data, nil
}

func (s *DocumentStore) Delete(ctx context.Context, ownerID uuid.UUID, documentIDs []uuid.UUID) error {
	keys := make([]string, len(documentIDs))
	for i, id := range documentIDs {
		keys[i] = documentKey(ownerID, id)
	}

	return s.bucket.remove(ctx, keys...)
}

func (s *DocumentStore) DeleteAll(ctx context.Context, ownerID uuid.UUID) error {
	objects, err := s.bucket.list(ctx, "docs/"+ownerID.String()+"/")
	if err != nil {
		return err
	}

	keys := make([]string, len(objects))
	for i, o := range objects {
		keys[i] = o.key
	}

	return s.bucket.remove(ctx, keys...)
}
