package inmem

import (
	"bytes"
	"context"
	"slices"
	"sync"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/post/internal/post"
)

// DocumentRepository keeps the documents of people. Documents handed out
// never change, so sharing pointers with callers is safe.
type DocumentRepository struct {
	mu        sync.RWMutex
	documents map[uuid.UUID]*post.Document
}

func NewDocumentRepository() *DocumentRepository {
	return &DocumentRepository{
		documents: make(map[uuid.UUID]*post.Document),
	}
}

func (r *DocumentRepository) Save(ctx context.Context, d *post.Document) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.documents[d.ID()] = d

	return nil
}

func (r *DocumentRepository) Get(ctx context.Context, id uuid.UUID) (*post.Document, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	d, ok := r.documents[id]
	if !ok {
		return nil, post.ErrDocumentNotFound
	}

	return d, nil
}

func (r *DocumentRepository) GetMany(ctx context.Context, ids []uuid.UUID) ([]*post.Document, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var found []*post.Document
	for _, id := range ids {
		if d, ok := r.documents[id]; ok && !slices.Contains(found, d) {
			found = append(found, d)
		}
	}

	return found, nil
}

func (r *DocumentRepository) Update(ctx context.Context, d *post.Document) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.documents[d.ID()]; !ok {
		return post.ErrDocumentNotFound
	}

	r.documents[d.ID()] = d

	return nil
}

func (r *DocumentRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.documents[id]; !ok {
		return post.ErrDocumentNotFound
	}

	delete(r.documents, id)

	return nil
}

func (r *DocumentRepository) List(ctx context.Context, ownerID, before uuid.UUID, limit int) ([]*post.Document, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var found []*post.Document
	for _, d := range r.ownedBy(ownerID) {
		if before == uuid.Nil || bytes.Compare(idBytes(d), before[:]) < 0 {
			found = append(found, d)
		}
		if len(found) == limit {
			break
		}
	}

	return found, nil
}

func (r *DocumentRepository) Count(ctx context.Context, ownerID uuid.UUID) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.ownedBy(ownerID)), nil
}

// DeleteByUser deletes the documents of a user.
func (r *DocumentRepository) DeleteByUser(ctx context.Context, userID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for id, d := range r.documents {
		if d.OwnerID() == userID {
			delete(r.documents, id)
		}
	}

	return nil
}

// ownedBy returns the documents of ownerID, newest first; the caller
// holds the lock.
func (r *DocumentRepository) ownedBy(ownerID uuid.UUID) []*post.Document {
	var owned []*post.Document
	for _, d := range r.documents {
		if d.OwnerID() == ownerID {
			owned = append(owned, d)
		}
	}

	slices.SortFunc(owned, func(a, b *post.Document) int {
		return bytes.Compare(idBytes(b), idBytes(a))
	})

	return owned
}

func idBytes(d *post.Document) []byte {
	id := d.ID()
	return id[:]
}
