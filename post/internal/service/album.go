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
	// SystemAlbum returns the album of a kind the site keeps for an
	// owner, post.ErrAlbumNotFound while there is none; Save of a second
	// one returns post.ErrSystemAlbumExists.
	SystemAlbum(ctx context.Context, ownerID uuid.UUID, kind post.AlbumKind) (*post.Album, error)
	// SetCover makes a photo of the album show it;
	// post.ErrAlbumPhotoNotFound when the photo is not in the album.
	SetCover(ctx context.Context, albumID, photoID uuid.UUID) error
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
	// Move puts a photo in another album, counting it there and not in
	// the one it leaves, which it no longer covers, in one statement:
	// post.ErrAlbumPhotoNotFound, post.ErrAlbumNotFound when the album it
	// goes to is gone.
	Move(ctx context.Context, id, albumID uuid.UUID, now time.Time) error
	// List returns up to limit photos of an album, oldest first, with IDs
	// above after (uuid.Nil: from the first).
	List(ctx context.Context, albumID, after uuid.UUID, limit int) ([]*post.AlbumPhoto, error)
	// Covers returns the first photo of each album that has any, as
	// openvk shows an album without a cover of its own.
	Covers(ctx context.Context, albumIDs []uuid.UUID) ([]*post.AlbumPhoto, error)
}

// AlbumPhotoLikeRepository keeps the likes of photos as LikeRepository
// keeps those of posts; post.ErrAlbumPhotoNotFound for a missing photo.
type AlbumPhotoLikeRepository interface {
	Like(ctx context.Context, photoID, userID uuid.UUID) (int, error)
	Unlike(ctx context.Context, photoID, userID uuid.UUID) (int, error)
	Liked(ctx context.Context, userID uuid.UUID, photoIDs []uuid.UUID) (map[uuid.UUID]bool, error)
	Likers(ctx context.Context, photoID uuid.UUID, after *LikerCursor, limit int) ([]Liker, error)
}

// PhotoCommentRepository keeps the comments of photos, counted in the
// photo by the statements that add and remove them.
type PhotoCommentRepository interface {
	// Save returns post.ErrAlbumPhotoNotFound when the photo is gone.
	Save(ctx context.Context, c *post.PhotoComment) error
	// Get and Delete return post.ErrCommentNotFound.
	Get(ctx context.Context, id uuid.UUID) (*post.PhotoComment, error)
	Delete(ctx context.Context, id uuid.UUID) error
	// List returns up to limit comments of a photo, oldest first, with IDs
	// above after (uuid.Nil: from the first).
	List(ctx context.Context, photoID, after uuid.UUID, limit int) ([]*post.PhotoComment, error)
}

// PhotoCommentPage is one screen of the comments of a photo, oldest
// first; Total counts them all.
type PhotoCommentPage struct {
	Comments   []*post.PhotoComment
	NextCursor uuid.UUID
	Total      int
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
	likes      AlbumPhotoLikeRepository
	comments   PhotoCommentRepository
	now        func() time.Time
}

// AlbumDeps are what AlbumService works with.
type AlbumDeps struct {
	Albums     AlbumRepository
	Photos     AlbumPhotoRepository
	Tx         TxManager
	Media      PhotoKeeper
	Moderators WallModerators
	Likes      AlbumPhotoLikeRepository
	Comments   PhotoCommentRepository
}

func NewAlbumService(d AlbumDeps) *AlbumService {
	return &AlbumService{
		albums:     d.Albums,
		photos:     d.Photos,
		tx:         d.Tx,
		media:      d.Media,
		moderators: d.Moderators,
		likes:      d.Likes,
		comments:   d.Comments,
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

	if err = a.CheckManual(); err != nil {
		return nil, errs.Wrap("a.CheckManual", err)
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
	a, err := s.changeable(ctx, requesterID, id)
	if err != nil {
		return err
	}

	if err = a.CheckManual(); err != nil {
		return errs.Wrap("a.CheckManual", err)
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
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

	if err = a.CheckManual(); err != nil {
		return nil, errs.Wrap("a.CheckManual", err)
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

// AddAvatar puts a new photo of a page, which requesterID uploaded, in
// its "Фотографии со страницы" (made with the first one), as openvk does
// with every new photo of a person or a community: ownerID is the
// requester or a community they run. Media keeps it published.
func (s *AlbumService) AddAvatar(ctx context.Context, requesterID, ownerID, photoID uuid.UUID) (*post.AlbumPhoto, error) {
	ok, err := moderates(ctx, s.moderators, ownerID, requesterID, ownerID == requesterID)
	if err != nil {
		return nil, errs.Wrap("moderates", err)
	}

	if !ok {
		return nil, post.ErrCannotChangeAlbum
	}

	a, err := s.systemAlbum(ctx, ownerID, post.AlbumAvatars)
	if err != nil {
		return nil, errs.Wrap("s.systemAlbum", err)
	}

	now := s.now()

	photo, err := post.NewAlbumPhoto(a, photoID, requesterID, "", now)
	if err != nil {
		return nil, errs.Wrap("post.NewAlbumPhoto", err)
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.photos.Add(ctx, a.ID(), []*post.AlbumPhoto{photo}, now); err != nil {
			return errs.Wrap("s.photos.Add", err)
		}

		return errs.Wrap("s.media.Publish", s.media.Publish(ctx, requesterID, []uuid.UUID{photoID}))
	})
	if err != nil {
		return nil, errs.Wrap("s.tx.WithinTx", err)
	}

	return photo, nil
}

// systemAlbum gets or makes the album of a kind the site keeps for
// ownerID; two first photos at once make one album, by the unique index.
// It runs outside a transaction, which the refused second Save would
// abort.
func (s *AlbumService) systemAlbum(ctx context.Context, ownerID uuid.UUID, kind post.AlbumKind) (*post.Album, error) {
	a, err := s.albums.SystemAlbum(ctx, ownerID, kind)
	if !errors.Is(err, post.ErrAlbumNotFound) {
		return a, errs.Wrap("s.albums.SystemAlbum", err)
	}

	if a, err = post.NewSystemAlbum(ownerID, kind, s.now()); err != nil {
		return nil, errs.Wrap("post.NewSystemAlbum", err)
	}

	err = s.albums.Save(ctx, a)
	if errors.Is(err, post.ErrSystemAlbumExists) {
		a, err = s.albums.SystemAlbum(ctx, ownerID, kind)
		return a, errs.Wrap("s.albums.SystemAlbum", err)
	}
	if err != nil {
		return nil, errs.Wrap("s.albums.Save", err)
	}

	return a, nil
}

// WallAlbum gets or makes "Фотографии со стены" of ownerID. Call it
// before the transaction that keeps photos there (see systemAlbum).
func (s *AlbumService) WallAlbum(ctx context.Context, ownerID uuid.UUID) (*post.Album, error) {
	a, err := s.systemAlbum(ctx, ownerID, post.AlbumWall)
	if err != nil {
		return nil, errs.Wrap("s.systemAlbum", err)
	}

	return a, nil
}

// KeepWallPhotos puts the photos of a post its owner wrote in a, their
// "Фотографии со стены", in the transaction of ctx: as copies under new
// IDs, which media makes, so that the album keeps a photo when the post
// is deleted and the post keeps it when the album loses it, as in openvk.
func (s *AlbumService) KeepWallPhotos(ctx context.Context, a *post.Album, photoIDs []uuid.UUID, nsfw bool) error {
	now := s.now()
	photos := make([]*post.AlbumPhoto, 0, len(photoIDs))
	copies := make([]PhotoCopy, 0, len(photoIDs))

	for _, from := range photoIDs {
		// UUIDv7 of one process grow: the copies keep the order of the post.
		to, err := uuid.NewV7()
		if err != nil {
			return errs.Wrap("uuid.NewV7", err)
		}

		photo, err := post.NewAlbumPhoto(a, to, a.OwnerID(), "", now)
		if err != nil {
			return errs.Wrap("post.NewAlbumPhoto", err)
		}

		photos = append(photos, photo.Marked(nsfw))
		copies = append(copies, PhotoCopy{From: from, To: to})
	}

	if err := s.photos.Add(ctx, a.ID(), photos, now); err != nil {
		return errs.Wrap("s.photos.Add", err)
	}

	return errs.Wrap("s.media.Copy", s.media.Copy(ctx, a.OwnerID(), copies))
}

// SetCover makes a photo of an album show it ("Обложка альбома"): the
// owner of the album or the admins of the community that owns it.
func (s *AlbumService) SetCover(ctx context.Context, requesterID, albumID, photoID uuid.UUID) error {
	a, err := s.changeable(ctx, requesterID, albumID)
	if err != nil {
		return err
	}

	p, err := s.photos.Get(ctx, photoID)
	if err != nil {
		return errs.Wrap("s.photos.Get", err)
	}

	if err = a.CheckCover(p); err != nil {
		return errs.Wrap("a.CheckCover", err)
	}

	if err = s.albums.SetCover(ctx, albumID, photoID); err != nil {
		return errs.Wrap("s.albums.SetCover", err)
	}

	return nil
}

// MovePhoto puts a photo in another album of the same owner: the owner
// of both or the admins of the community that owns them.
func (s *AlbumService) MovePhoto(ctx context.Context, requesterID, photoID, albumID uuid.UUID) error {
	_, from, err := s.Photo(ctx, photoID)
	if err != nil {
		return errs.Wrap("s.Photo", err)
	}

	// Who may change the one may change the other: they have one owner.
	to, err := s.changeable(ctx, requesterID, albumID)
	if err != nil {
		return err
	}

	if err = from.CheckMove(to); err != nil {
		return errs.Wrap("from.CheckMove", err)
	}

	if err = s.photos.Move(ctx, photoID, albumID, s.now()); err != nil {
		return errs.Wrap("s.photos.Move", err)
	}

	return nil
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

// LikePhoto and UnlikePhoto are the heart under a photo, as under a post:
// idempotent, they return how many like it now.
func (s *AlbumService) LikePhoto(ctx context.Context, userID, photoID uuid.UUID) (int, error) {
	likes, err := s.likes.Like(ctx, photoID, userID)
	if err != nil {
		return 0, errs.Wrap("s.likes.Like", err)
	}

	return likes, nil
}

func (s *AlbumService) UnlikePhoto(ctx context.Context, userID, photoID uuid.UUID) (int, error) {
	likes, err := s.likes.Unlike(ctx, photoID, userID)
	if err != nil {
		return 0, errs.Wrap("s.likes.Unlike", err)
	}

	return likes, nil
}

// LikedPhotos tells which of photoIDs userID likes; none for uuid.Nil.
func (s *AlbumService) LikedPhotos(ctx context.Context, userID uuid.UUID, photoIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	if userID == uuid.Nil || len(photoIDs) == 0 {
		return nil, nil
	}

	liked, err := s.likes.Liked(ctx, userID, photoIDs)
	if err != nil {
		return nil, errs.Wrap("s.likes.Liked", err)
	}

	return liked, nil
}

// PhotoLikers pages who like a photo, newest first ("Понравилось N
// людям"); post.ErrAlbumPhotoNotFound for a missing photo.
func (s *AlbumService) PhotoLikers(ctx context.Context, photoID uuid.UUID, cursor *LikerCursor, size int) (LikerPage, error) {
	if size <= 0 || size > MaxPageSize {
		size = DefaultPageSize
	}

	p, err := s.photos.Get(ctx, photoID)
	if err != nil {
		return LikerPage{}, errs.Wrap("s.photos.Get", err)
	}

	found, err := s.likes.Likers(ctx, photoID, cursor, size+1)
	if err != nil {
		return LikerPage{}, errs.Wrap("s.likes.Likers", err)
	}

	return likerPage(found, size, p.Likes()), nil
}

// CommentPhoto adds a comment under a photo. Who may (a blacklist) the
// caller has checked; post.ErrAlbumPhotoNotFound for a missing photo.
func (s *AlbumService) CommentPhoto(ctx context.Context, authorID, photoID uuid.UUID, text post.CommentText) (*post.PhotoComment, error) {
	c, err := post.NewPhotoComment(photoID, authorID, text, s.now())
	if err != nil {
		return nil, errs.Wrap("post.NewPhotoComment", err)
	}

	if err = s.comments.Save(ctx, c); err != nil {
		return nil, errs.Wrap("s.comments.Save", err)
	}

	return c, nil
}

// DeletePhotoComment deletes a comment: by its author, who uploaded the
// photo, the owner of the album or the admins of its community.
func (s *AlbumService) DeletePhotoComment(ctx context.Context, requesterID, id uuid.UUID) error {
	c, err := s.comments.Get(ctx, id)
	if err != nil {
		return errs.Wrap("s.comments.Get", err)
	}

	photo, album, err := s.Photo(ctx, c.PhotoID())
	if err != nil {
		return errs.Wrap("s.Photo", err)
	}

	ok, err := moderates(ctx, s.moderators, album.OwnerID(), requesterID,
		c.CanBeDeletedBy(requesterID, photo, album.CanBeChangedBy(requesterID, false)))
	if err != nil {
		return errs.Wrap("moderates", err)
	}

	if !ok {
		return post.ErrCannotDeletePhotoComment
	}

	if err = s.comments.Delete(ctx, id); err != nil {
		return errs.Wrap("s.comments.Delete", err)
	}

	return nil
}

// PhotoComments pages the comments of a photo, oldest first, as openvk
// shows them; post.ErrAlbumPhotoNotFound for a missing photo.
func (s *AlbumService) PhotoComments(ctx context.Context, photoID, after uuid.UUID, size int) (PhotoCommentPage, error) {
	if size <= 0 || size > MaxPageSize {
		size = DefaultPageSize
	}

	p, err := s.photos.Get(ctx, photoID)
	if err != nil {
		return PhotoCommentPage{}, errs.Wrap("s.photos.Get", err)
	}

	found, err := s.comments.List(ctx, photoID, after, size+1)
	if err != nil {
		return PhotoCommentPage{}, errs.Wrap("s.comments.List", err)
	}

	page := PhotoCommentPage{Comments: found, Total: p.Comments()}
	if len(found) > size {
		page.Comments = found[:size]
		page.NextCursor = found[size-1].ID()
	}

	return page, nil
}
