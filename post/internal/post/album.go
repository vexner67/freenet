package post

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	// maxAlbumNameLen is openvk's, which counts bytes; we count
	// characters, as everywhere.
	maxAlbumNameLen = 36
	// maxDescriptionLen is openvk's for a photo; an album, which openvk
	// does not limit, gets the same.
	maxDescriptionLen = 255
)

var (
	ErrAlbumNameEmpty         = errors.New("album name is empty")
	ErrAlbumNameTooLong       = errors.New("album name is too long")
	ErrDescriptionTooLong     = errors.New("description is too long")
	ErrAlbumNotFound          = errors.New("album not found")
	ErrCannotChangeAlbum      = errors.New("only the owner of the album can change it")
	ErrAlbumPhotoNotFound     = errors.New("photo not found")
	ErrCannotDescribePhoto    = errors.New("only who uploaded the photo can describe it")
	ErrCannotDeleteAlbumPhoto = errors.New("only who uploaded the photo or the owner of the album can delete it")
	ErrNoAlbumPhotos          = errors.New("no photos to add")
)

// AlbumName is what an album is called: one line, trimmed.
type AlbumName string

func NewAlbumName(s string) (AlbumName, error) {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return "", ErrAlbumNameEmpty
	}

	if utf8.RuneCountInString(s) > maxAlbumNameLen {
		return "", ErrAlbumNameTooLong
	}

	return AlbumName(s), nil
}

func (n AlbumName) String() string {
	return string(n)
}

// Description tells what an album or a photo is: trimmed, with "\n" line
// breaks, possibly empty.
type Description string

func NewDescription(s string) (Description, error) {
	s, err := cleanText(s, maxDescriptionLen, nil, ErrDescriptionTooLong)
	if err != nil {
		return "", err
	}

	return Description(s), nil
}

func (d Description) String() string {
	return string(d)
}

// Album is a photo album of a person or a community ("Альбомы" of
// openvk): ownerID is either, as the wall of a post. The person, or the
// admins of the community, name it and add photos to it.
type Album struct {
	id          uuid.UUID
	ownerID     uuid.UUID
	name        AlbumName
	description Description
	createdAt   time.Time
	// updatedAt is when it was last renamed or had photos added or
	// deleted ("Обновлён" of openvk).
	updatedAt time.Time
	photos    int
}

// NewAlbum creates an album of ownerID. Whether the caller may create
// one for a community is the community's, which the caller checks.
func NewAlbum(ownerID uuid.UUID, name AlbumName, description Description, now time.Time) (*Album, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	// PostgreSQL keeps microseconds; trim now so an album reads back equal.
	now = now.UTC().Truncate(time.Microsecond)

	return &Album{id: id, ownerID: ownerID, name: name, description: description, createdAt: now, updatedAt: now}, nil
}

// StoredAlbum is an album as storage keeps it.
type StoredAlbum struct {
	ID          uuid.UUID
	OwnerID     uuid.UUID
	Name        AlbumName
	Description Description
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Photos      int
}

// RestoreAlbum rebuilds an album from storage without validation or new
// IDs.
func RestoreAlbum(s StoredAlbum) *Album {
	return &Album{
		id: s.ID, ownerID: s.OwnerID, name: s.Name, description: s.Description,
		createdAt: s.CreatedAt, updatedAt: s.UpdatedAt, photos: s.Photos,
	}
}

func (a *Album) Stored() StoredAlbum {
	return StoredAlbum{
		ID: a.id, OwnerID: a.ownerID, Name: a.name, Description: a.description,
		CreatedAt: a.createdAt, UpdatedAt: a.updatedAt, Photos: a.photos,
	}
}

// CanBeChangedBy: its owner or, admin, whoever runs the community that
// owns it renames it, deletes it and adds photos to it, as in openvk.
func (a *Album) CanBeChangedBy(userID uuid.UUID, admin bool) bool {
	return admin || userID == a.ownerID
}

// Renamed returns a with a new name and description, as of now.
func (a *Album) Renamed(name AlbumName, description Description, now time.Time) *Album {
	renamed := *a
	renamed.name = name
	renamed.description = description
	renamed.updatedAt = now.UTC().Truncate(time.Microsecond)

	return &renamed
}

func (a *Album) ID() uuid.UUID {
	return a.id
}

// OwnerID is the person or the community the album belongs to.
func (a *Album) OwnerID() uuid.UUID {
	return a.ownerID
}

func (a *Album) Name() AlbumName {
	return a.name
}

func (a *Album) Description() Description {
	return a.description
}

func (a *Album) CreatedAt() time.Time {
	return a.createdAt
}

func (a *Album) UpdatedAt() time.Time {
	return a.updatedAt
}

// Photos counts the photos in the album.
func (a *Album) Photos() int {
	return a.photos
}

// AlbumPhoto is a photo in an album: the media service keeps the image
// under its ID, which the album photo shares, and under who uploaded it
// (authorID). Its UUIDv7 ID grows with the time of the upload, so the
// photos of an album are ordered and paged by ID, oldest first, as openvk
// shows them.
type AlbumPhoto struct {
	id          uuid.UUID
	albumID     uuid.UUID
	authorID    uuid.UUID
	description Description
	createdAt   time.Time
}

// NewAlbumPhoto puts photoID, uploaded by authorID, in album a.
func NewAlbumPhoto(a *Album, photoID, authorID uuid.UUID, description Description, now time.Time) (*AlbumPhoto, error) {
	if photoID == uuid.Nil {
		return nil, ErrInvalidPhoto
	}

	return &AlbumPhoto{
		id:          photoID,
		albumID:     a.id,
		authorID:    authorID,
		description: description,
		// PostgreSQL keeps microseconds; trim now so it reads back equal.
		createdAt: now.UTC().Truncate(time.Microsecond),
	}, nil
}

// StoredAlbumPhoto is a photo of an album as storage keeps it.
type StoredAlbumPhoto struct {
	ID          uuid.UUID
	AlbumID     uuid.UUID
	AuthorID    uuid.UUID
	Description Description
	CreatedAt   time.Time
}

// RestoreAlbumPhoto rebuilds a photo from storage without validation.
func RestoreAlbumPhoto(s StoredAlbumPhoto) *AlbumPhoto {
	return &AlbumPhoto{id: s.ID, albumID: s.AlbumID, authorID: s.AuthorID, description: s.Description, createdAt: s.CreatedAt}
}

// Described returns p with a new description by requesterID: only who
// uploaded it, as in openvk.
func (p *AlbumPhoto) Described(requesterID uuid.UUID, description Description) (*AlbumPhoto, error) {
	if requesterID != p.authorID {
		return nil, ErrCannotDescribePhoto
	}

	described := *p
	described.description = description

	return &described, nil
}

// CanBeDeletedBy: who uploaded it or, owner, whoever may change its
// album (Album.CanBeChangedBy) — openvk's "Удалить" and the cross on the
// photo in the album.
func (p *AlbumPhoto) CanBeDeletedBy(userID uuid.UUID, owner bool) bool {
	return owner || userID == p.authorID
}

func (p *AlbumPhoto) ID() uuid.UUID {
	return p.id
}

func (p *AlbumPhoto) AlbumID() uuid.UUID {
	return p.albumID
}

// AuthorID is who uploaded the photo; the media service keeps the image
// under them.
func (p *AlbumPhoto) AuthorID() uuid.UUID {
	return p.authorID
}

func (p *AlbumPhoto) Description() Description {
	return p.description
}

func (p *AlbumPhoto) CreatedAt() time.Time {
	return p.createdAt
}
