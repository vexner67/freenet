package service

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/post/internal/post"
)

// MaxPhotosAtOnce is openvk's: "Вы можете загружать до 10 фотографий за
// один раз".
const MaxPhotosAtOnce = 10

var ErrTooManyAlbumPhotos = errors.New("at most 10 photos are added at once")

// AlbumRepository keeps the photo albums of people and communities.
type AlbumRepository interface {
	Save(ctx context.Context, a *post.Album) error
	// Get and Update return post.ErrAlbumNotFound.
	Get(ctx context.Context, id uuid.UUID) (*post.Album, error)
	// Update saves the name, the description and the time of the change.
	Update(ctx context.Context, a *post.Album) error
	// Delete deletes an album with its photos, returning the photos;
	// post.ErrAlbumNotFound.
	Delete(ctx context.Context, id uuid.UUID) ([]*post.AlbumPhoto, error)
	// List returns up to limit albums of an owner, newest first, with IDs
	// below before (uuid.Nil: from the first); Count counts them all.
	List(ctx context.Context, ownerID, before uuid.UUID, limit int) ([]*post.Album, error)
	Count(ctx context.Context, ownerID uuid.UUID) (int, error)
}

// AlbumPhotoRepository keeps the photos of albums.
type AlbumPhotoRepository interface {
	// Add puts photos in an album, counting them and dating the album in
	// one statement: post.ErrAlbumNotFound when the album is gone,
	// post.ErrDuplicatePhoto when a photo is in an album already.
	Add(ctx context.Context, albumID uuid.UUID, photos []*post.AlbumPhoto, now time.Time) error
	// Get, Describe and Delete return post.ErrAlbumPhotoNotFound.
	Get(ctx context.Context, id uuid.UUID) (*post.AlbumPhoto, error)
	// Describe saves the description.
	Describe(ctx context.Context, p *post.AlbumPhoto) error
	// Delete removes a photo, uncounting it and dating its album.
	Delete(ctx context.Context, id uuid.UUID, now time.Time) error
	// List returns up to limit photos of an album, oldest first, with IDs
	// above after (uuid.Nil: from the first).
	List(ctx context.Context, albumID, after uuid.UUID, limit int) ([]*post.AlbumPhoto, error)
	// Covers returns the first photo of each album that has any, as
	// openvk shows an album without a cover of its own.
	Covers(ctx context.Context, albumIDs []uuid.UUID) ([]*post.AlbumPhoto, error)
}

// AlbumPage is one screen of albums with the cover of each, by album, if
// it has photos; NextCursor is uuid.Nil on the last.
type AlbumPage struct {
	Albums     []*post.Album
	Covers     map[uuid.UUID]*post.AlbumPhoto
	NextCursor uuid.UUID
	Total      int
}

// AlbumPhotoPage is one screen of an album, oldest first; NextCursor is
// uuid.Nil on the last, Total counts the photos of the album.
type AlbumPhotoPage struct {
	Photos     []*post.AlbumPhoto
	NextCursor uuid.UUID
	Total      int
}

// CreateAlbumInput is an album to create by RequesterID for OwnerID: the
// requester or a community they run.
type CreateAlbumInput struct {
	OwnerID     uuid.UUID
	RequesterID uuid.UUID
	Name        post.AlbumName
	Description post.Description
}

// NewAlbumPhoto is a photo to add to an album: the media service has it
// uploaded by the requester, which the caller checks.
type NewAlbumPhoto struct {
	ID          uuid.UUID
	Description post.Description
}

// AlbumService keeps the photo albums of people and communities
// ("Альбомы" of openvk). Who runs a community the service asks itself
// (WallModerators).
type AlbumService struct {
	albums     AlbumRepository
	photos     AlbumPhotoRepository
	tx         TxManager
	media      PhotoKeeper
	moderators WallModerators
	now        func() time.Time
}

// AlbumDeps are what AlbumService works with.
type AlbumDeps struct {
	Albums     AlbumRepository
	Photos     AlbumPhotoRepository
	Tx         TxManager
	Media      PhotoKeeper
	Moderators WallModerators
}

func NewAlbumService(d AlbumDeps) *AlbumService {
	return &AlbumService{
		albums:     d.Albums,
		photos:     d.Photos,
		tx:         d.Tx,
		media:      d.Media,
		moderators: d.Moderators,
		now:        time.Now,
	}
}

// Create creates an album: one's own, or one of a community its admins
// create ("Создать альбом").
func (s *AlbumService) Create(ctx context.Context, in CreateAlbumInput) (*post.Album, error) {
	ok, err := moderates(ctx, s.moderators, in.OwnerID, in.RequesterID, in.OwnerID == in.RequesterID)
	if err != nil {
		return nil, errs.Wrap("moderates", err)
	}

	if !ok {
		return nil, post.ErrCannotChangeAlbum
	}

	a, err := post.NewAlbum(in.OwnerID, in.Name, in.Description, s.now())
	if err != nil {
		return nil, errs.Wrap("post.NewAlbum", err)
	}

	if err = s.albums.Save(ctx, a); err != nil {
		return nil, errs.Wrap("s.albums.Save", err)
	}

	return a, nil
}

// Get returns an album with its cover, nil while it has no photos.
func (s *AlbumService) Get(ctx context.Context, id uuid.UUID) (*post.Album, *post.AlbumPhoto, error) {
	a, err := s.albums.Get(ctx, id)
	if err != nil {
		return nil, nil, errs.Wrap("s.albums.Get", err)
	}

	covers, err := s.photos.Covers(ctx, []uuid.UUID{id})
	if err != nil {
		return nil, nil, errs.Wrap("s.photos.Covers", err)
	}

	if len(covers) == 0 {
		return a, nil, nil
	}

	return a, covers[0], nil
}

// List pages the albums of a person or a community, newest first.
func (s *AlbumService) List(ctx context.Context, ownerID, before uuid.UUID, size int) (AlbumPage, error) {
	if size <= 0 || size > MaxPageSize {
		size = DefaultPageSize
	}

	// One extra tells whether there is a next page.
	found, err := s.albums.List(ctx, ownerID, before, size+1)
	if err != nil {
		return AlbumPage{}, errs.Wrap("s.albums.List", err)
	}

	page := AlbumPage{Albums: found, Covers: make(map[uuid.UUID]*post.AlbumPhoto)}
	if len(found) > size {
		page.Albums = found[:size]
		page.NextCursor = found[size-1].ID()
	}

	if page.Total, err = s.albums.Count(ctx, ownerID); err != nil {
		return AlbumPage{}, errs.Wrap("s.albums.Count", err)
	}

	ids := make([]uuid.UUID, 0, len(page.Albums))
	for _, a := range page.Albums {
		ids = append(ids, a.ID())
	}

	covers, err := s.photos.Covers(ctx, ids)
	if err != nil {
		return AlbumPage{}, errs.Wrap("s.photos.Covers", err)
	}

	for _, p := range covers {
		page.Covers[p.AlbumID()] = p
	}

	return page, nil
}

// Change renames an album ("Редактировать альбом"): its owner or the
// admins of the community that owns it.
func (s *AlbumService) Change(ctx context.Context, requesterID, id uuid.UUID, name post.AlbumName, description post.Description) (*post.Album, error) {
	a, err := s.changeable(ctx, requesterID, id)
	if err != nil {
		return nil, err
	}

	renamed := a.Renamed(name, description, s.now())
	if err = s.albums.Update(ctx, renamed); err != nil {
		return nil, errs.Wrap("s.albums.Update", err)
	}

	return renamed, nil
}

// Delete deletes an album with its photos ("удалить альбом"): its owner
// or the admins of the community that owns it.
func (s *AlbumService) Delete(ctx context.Context, requesterID, id uuid.UUID) error {
	if _, err := s.changeable(ctx, requesterID, id); err != nil {
		return err
	}

	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		photos, err := s.albums.Delete(ctx, id)
		if err != nil {
			return errs.Wrap("s.albums.Delete", err)
		}

		return errs.Wrap("s.deleteFiles", s.deleteFiles(ctx, photos))
	})
	if err != nil {
		return errs.Wrap("s.tx.WithinTx", err)
	}

	return nil
}

// AddPhotos puts photos the requester uploaded in an album ("Добавить
// фотографии"), up to MaxPhotosAtOnce: the owner of the album or the
// admins of the community that owns it. Media keeps them published.
func (s *AlbumService) AddPhotos(ctx context.Context, requesterID, albumID uuid.UUID, in []NewAlbumPhoto) ([]*post.AlbumPhoto, error) {
	if len(in) == 0 {
		return nil, post.ErrNoAlbumPhotos
	}

	if len(in) > MaxPhotosAtOnce {
		return nil, ErrTooManyAlbumPhotos
	}

	a, err := s.changeable(ctx, requesterID, albumID)
	if err != nil {
		return nil, err
	}

	now := s.now()
	photos := make([]*post.AlbumPhoto, 0, len(in))
	ids := make([]uuid.UUID, 0, len(in))
	seen := make(map[uuid.UUID]bool, len(in))

	for _, p := range in {
		if seen[p.ID] {
			return nil, post.ErrDuplicatePhoto
		}
		seen[p.ID] = true

		photo, err := post.NewAlbumPhoto(a, p.ID, requesterID, p.Description, now)
		if err != nil {
			return nil, errs.Wrap("post.NewAlbumPhoto", err)
		}

		photos = append(photos, photo)
		ids = append(ids, p.ID)
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.photos.Add(ctx, albumID, photos, now); err != nil {
			return errs.Wrap("s.photos.Add", err)
		}

		return errs.Wrap("s.media.Publish", s.media.Publish(ctx, requesterID, ids))
	})
	if err != nil {
		return nil, errs.Wrap("s.tx.WithinTx", err)
	}

	return photos, nil
}

// Photos pages the photos of an album, oldest first, as openvk shows
// them.
func (s *AlbumService) Photos(ctx context.Context, albumID, after uuid.UUID, size int) (AlbumPhotoPage, error) {
	a, err := s.albums.Get(ctx, albumID)
	if err != nil {
		return AlbumPhotoPage{}, errs.Wrap("s.albums.Get", err)
	}

	if size <= 0 || size > MaxPageSize {
		size = DefaultPageSize
	}

	found, err := s.photos.List(ctx, albumID, after, size+1)
	if err != nil {
		return AlbumPhotoPage{}, errs.Wrap("s.photos.List", err)
	}

	page := AlbumPhotoPage{Photos: found, Total: a.Photos()}
	if len(found) > size {
		page.Photos = found[:size]
		page.NextCursor = found[size-1].ID()
	}

	return page, nil
}

// Photo returns a photo of an album with the album.
func (s *AlbumService) Photo(ctx context.Context, id uuid.UUID) (*post.AlbumPhoto, *post.Album, error) {
	p, err := s.photos.Get(ctx, id)
	if err != nil {
		return nil, nil, errs.Wrap("s.photos.Get", err)
	}

	a, err := s.albums.Get(ctx, p.AlbumID())
	if err != nil {
		return nil, nil, errs.Wrap("s.albums.Get", err)
	}

	return p, a, nil
}

// Describe changes the description of a photo: who uploaded it only.
func (s *AlbumService) Describe(ctx context.Context, requesterID, id uuid.UUID, description post.Description) (*post.AlbumPhoto, error) {
	p, err := s.photos.Get(ctx, id)
	if err != nil {
		return nil, errs.Wrap("s.photos.Get", err)
	}

	described, err := p.Described(requesterID, description)
	if err != nil {
		return nil, errs.Wrap("p.Described", err)
	}

	if err = s.photos.Describe(ctx, described); err != nil {
		return nil, errs.Wrap("s.photos.Describe", err)
	}

	return described, nil
}

// DeletePhoto deletes a photo of an album with its image: who uploaded it
// or whoever may change the album.
func (s *AlbumService) DeletePhoto(ctx context.Context, requesterID, id uuid.UUID) error {
	p, a, err := s.Photo(ctx, id)
	if err != nil {
		return errs.Wrap("s.Photo", err)
	}

	if !p.CanBeDeletedBy(requesterID, false) {
		owner, err := moderates(ctx, s.moderators, a.OwnerID(), requesterID, a.CanBeChangedBy(requesterID, false))
		if err != nil {
			return errs.Wrap("moderates", err)
		}

		if !p.CanBeDeletedBy(requesterID, owner) {
			return post.ErrCannotDeleteAlbumPhoto
		}
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.photos.Delete(ctx, id, s.now()); err != nil {
			return errs.Wrap("s.photos.Delete", err)
		}

		return errs.Wrap("s.deleteFiles", s.deleteFiles(ctx, []*post.AlbumPhoto{p}))
	})
	if err != nil {
		return errs.Wrap("s.tx.WithinTx", err)
	}

	return nil
}

// changeable returns an album requesterID may change.
func (s *AlbumService) changeable(ctx context.Context, requesterID, id uuid.UUID) (*post.Album, error) {
	a, err := s.albums.Get(ctx, id)
	if err != nil {
		return nil, errs.Wrap("s.albums.Get", err)
	}

	ok, err := moderates(ctx, s.moderators, a.OwnerID(), requesterID, a.CanBeChangedBy(requesterID, false))
	if err != nil {
		return nil, errs.Wrap("moderates", err)
	}

	if !ok {
		return nil, post.ErrCannotChangeAlbum
	}

	return a, nil
}

// deleteFiles tells media to delete the images of photos, each under who
// uploaded it, MaxPhotosAtOnce a call, as media takes them.
func (s *AlbumService) deleteFiles(ctx context.Context, photos []*post.AlbumPhoto) error {
	var authors []uuid.UUID
	byAuthor := make(map[uuid.UUID][]uuid.UUID)
	for _, p := range photos {
		if _, ok := byAuthor[p.AuthorID()]; !ok {
			authors = append(authors, p.AuthorID())
		}
		byAuthor[p.AuthorID()] = append(byAuthor[p.AuthorID()], p.ID())
	}

	for _, authorID := range authors {
		for ids := range slices.Chunk(byAuthor[authorID], MaxPhotosAtOnce) {
			if err := s.media.Delete(ctx, authorID, ids); err != nil {
				return errs.Wrap("s.media.Delete", err)
			}
		}
	}

	return nil
}
