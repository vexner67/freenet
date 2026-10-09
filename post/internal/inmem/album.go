package inmem

import (
	"bytes"
	"context"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/post/internal/post"
)

// AlbumRepository keeps albums and their photos, as the two tables do in
// PostgreSQL: a photo counts in its album and dates it, a deleted album
// takes its photos. Albums are stored as StoredAlbum and handed out anew,
// so callers never share them; photos are immutable.
type AlbumRepository struct {
	mu     sync.RWMutex
	albums map[uuid.UUID]post.StoredAlbum
	photos map[uuid.UUID]*post.AlbumPhoto
}

func NewAlbumRepository() *AlbumRepository {
	return &AlbumRepository{
		albums: make(map[uuid.UUID]post.StoredAlbum),
		photos: make(map[uuid.UUID]*post.AlbumPhoto),
	}
}

// Photos is the same repository as service.AlbumPhotoRepository.
func (r *AlbumRepository) Photos() *AlbumPhotoRepository {
	return &AlbumPhotoRepository{r: r}
}

func (r *AlbumRepository) Save(ctx context.Context, a *post.Album) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.albums[a.ID()] = a.Stored()

	return nil
}

func (r *AlbumRepository) Get(ctx context.Context, id uuid.UUID) (*post.Album, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	s, ok := r.albums[id]
	if !ok {
		return nil, post.ErrAlbumNotFound
	}

	return post.RestoreAlbum(s), nil
}

func (r *AlbumRepository) Update(ctx context.Context, a *post.Album) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	s, ok := r.albums[a.ID()]
	if !ok {
		return post.ErrAlbumNotFound
	}

	s.Name, s.Description, s.UpdatedAt = a.Name(), a.Description(), a.UpdatedAt()
	r.albums[a.ID()] = s

	return nil
}

func (r *AlbumRepository) Delete(ctx context.Context, id uuid.UUID) ([]*post.AlbumPhoto, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.albums[id]; !ok {
		return nil, post.ErrAlbumNotFound
	}

	return r.deleteAlbum(id), nil
}

// deleteAlbum removes an album with its photos, returning them; the lock
// is held.
func (r *AlbumRepository) deleteAlbum(id uuid.UUID) []*post.AlbumPhoto {
	delete(r.albums, id)

	var gone []*post.AlbumPhoto
	for pid, p := range r.photos {
		if p.AlbumID() == id {
			delete(r.photos, pid)
			gone = append(gone, p)
		}
	}

	return gone
}

func (r *AlbumRepository) List(ctx context.Context, ownerID, before uuid.UUID, limit int) ([]*post.Album, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var found []post.StoredAlbum
	for _, s := range r.albums {
		if s.OwnerID == ownerID && (before == uuid.Nil || bytes.Compare(s.ID[:], before[:]) < 0) {
			found = append(found, s)
		}
	}

	slices.SortFunc(found, func(a, b post.StoredAlbum) int { return bytes.Compare(b.ID[:], a.ID[:]) })

	albums := make([]*post.Album, 0, min(len(found), limit))
	for _, s := range found[:min(len(found), limit)] {
		albums = append(albums, post.RestoreAlbum(s))
	}

	return albums, nil
}

func (r *AlbumRepository) Count(ctx context.Context, ownerID uuid.UUID) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, errs.Wrap("ctx.Err", err)
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	n := 0
	for _, s := range r.albums {
		if s.OwnerID == ownerID {
			n++
		}
	}

	return n, nil
}

// DeleteByUser deletes the albums a user owns with their photos and the
// photos they put in others' albums, uncounting those.
func (r *AlbumRepository) DeleteByUser(ctx context.Context, userID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for id, s := range r.albums {
		if s.OwnerID == userID {
			r.deleteAlbum(id)
		}
	}

	for id, p := range r.photos {
		if p.AuthorID() == userID {
			delete(r.photos, id)
			r.recount(p.AlbumID(), -1, time.Time{})
		}
	}

	return nil
}

// recount adds n photos to an album and, unless now is zero, dates it;
// the lock is held.
func (r *AlbumRepository) recount(albumID uuid.UUID, n int, now time.Time) {
	s, ok := r.albums[albumID]
	if !ok {
		return
	}

	s.Photos += n
	if !now.IsZero() {
		s.UpdatedAt = now.UTC().Truncate(time.Microsecond)
	}
	r.albums[albumID] = s
}

// AlbumPhotoRepository is the service.AlbumPhotoRepository side of
// AlbumRepository.
type AlbumPhotoRepository struct {
	r *AlbumRepository
}

func (p *AlbumPhotoRepository) Add(ctx context.Context, albumID uuid.UUID, photos []*post.AlbumPhoto, now time.Time) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	p.r.mu.Lock()
	defer p.r.mu.Unlock()

	if _, ok := p.r.albums[albumID]; !ok {
		return post.ErrAlbumNotFound
	}

	for _, photo := range photos {
		if _, ok := p.r.photos[photo.ID()]; ok {
			return post.ErrDuplicatePhoto
		}
	}

	for _, photo := range photos {
		p.r.photos[photo.ID()] = photo
	}
	p.r.recount(albumID, len(photos), now)

	return nil
}

func (p *AlbumPhotoRepository) Get(ctx context.Context, id uuid.UUID) (*post.AlbumPhoto, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	p.r.mu.RLock()
	defer p.r.mu.RUnlock()

	photo, ok := p.r.photos[id]
	if !ok {
		return nil, post.ErrAlbumPhotoNotFound
	}

	return photo, nil
}

func (p *AlbumPhotoRepository) Describe(ctx context.Context, photo *post.AlbumPhoto) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	p.r.mu.Lock()
	defer p.r.mu.Unlock()

	if _, ok := p.r.photos[photo.ID()]; !ok {
		return post.ErrAlbumPhotoNotFound
	}

	p.r.photos[photo.ID()] = photo

	return nil
}

func (p *AlbumPhotoRepository) Delete(ctx context.Context, id uuid.UUID, now time.Time) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	p.r.mu.Lock()
	defer p.r.mu.Unlock()

	photo, ok := p.r.photos[id]
	if !ok {
		return post.ErrAlbumPhotoNotFound
	}

	delete(p.r.photos, id)
	p.r.recount(photo.AlbumID(), -1, now)

	return nil
}

func (p *AlbumPhotoRepository) List(ctx context.Context, albumID, after uuid.UUID, limit int) ([]*post.AlbumPhoto, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	p.r.mu.RLock()
	defer p.r.mu.RUnlock()

	var found []*post.AlbumPhoto
	for _, photo := range p.r.photos {
		id := photo.ID()
		if photo.AlbumID() == albumID && bytes.Compare(id[:], after[:]) > 0 {
			found = append(found, photo)
		}
	}

	slices.SortFunc(found, compareAlbumPhotos)

	return found[:min(len(found), limit)], nil
}

func (p *AlbumPhotoRepository) Covers(ctx context.Context, albumIDs []uuid.UUID) ([]*post.AlbumPhoto, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	p.r.mu.RLock()
	defer p.r.mu.RUnlock()

	first := make(map[uuid.UUID]*post.AlbumPhoto)
	for _, photo := range p.r.photos {
		if !slices.Contains(albumIDs, photo.AlbumID()) {
			continue
		}

		if cur, ok := first[photo.AlbumID()]; !ok || compareAlbumPhotos(photo, cur) < 0 {
			first[photo.AlbumID()] = photo
		}
	}

	covers := make([]*post.AlbumPhoto, 0, len(first))
	for _, photo := range first {
		covers = append(covers, photo)
	}

	return covers, nil
}

func compareAlbumPhotos(a, b *post.AlbumPhoto) int {
	aID, bID := a.ID(), b.ID()

	return bytes.Compare(aID[:], bID[:])
}
