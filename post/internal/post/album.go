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
	// ErrSystemAlbum: the site keeps the album ("Фотографии со
	// страницы"); nobody renames or deletes it or adds photos by hand.
	ErrSystemAlbum = errors.New("this album is kept by the site")
	// ErrSystemAlbumExists: the owner has their album of this kind.
	ErrSystemAlbumExists = errors.New("the album of this kind exists")
	// ErrCannotMovePhoto: a photo moves to another album of the same
	// owner, one made by people.
	ErrCannotMovePhoto = errors.New("a photo moves between the albums of one owner")
)

// AlbumKind tells an album someone made ("") from one the site keeps.
type AlbumKind string

const (
	AlbumRegular AlbumKind = ""
	// AlbumAvatars is openvk's "Фотографии со страницы": every photo of
	// the page, one album an owner.
	AlbumAvatars AlbumKind = "avatars"
	// AlbumWall is openvk's "Фотографии со стены": the photos of the posts
	// a person wrote on their wall, one album an owner.
	AlbumWall AlbumKind = "wall"
)

// systemAlbumNames are how openvk calls them.
var systemAlbumNames = map[AlbumKind]AlbumName{
	AlbumAvatars: "Фотографии со страницы",
	AlbumWall:    "Фотографии со стены",
}

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
	kind      AlbumKind
	// cover is the photo chosen to show the album ("Обложка альбома"),
	// uuid.Nil while its first photo does.
	cover uuid.UUID
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

// NewSystemAlbum creates the album of ownerID the site keeps: "Фотографии
// со страницы" or "Фотографии со стены".
func NewSystemAlbum(ownerID uuid.UUID, kind AlbumKind, now time.Time) (*Album, error) {
	name, ok := systemAlbumNames[kind]
	if !ok {
		return nil, ErrSystemAlbum
	}

	a, err := NewAlbum(ownerID, name, "", now)
	if err != nil {
		return nil, err
	}

	a.kind = kind

	return a, nil
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
	Kind        AlbumKind
	Cover       uuid.UUID
}

// RestoreAlbum rebuilds an album from storage without validation or new
// IDs.
func RestoreAlbum(s StoredAlbum) *Album {
	return &Album{
		id: s.ID, ownerID: s.OwnerID, name: s.Name, description: s.Description,
		createdAt: s.CreatedAt, updatedAt: s.UpdatedAt, photos: s.Photos, kind: s.Kind, cover: s.Cover,
	}
}

func (a *Album) Stored() StoredAlbum {
	return StoredAlbum{
		ID: a.id, OwnerID: a.ownerID, Name: a.name, Description: a.description,
		CreatedAt: a.createdAt, UpdatedAt: a.updatedAt, Photos: a.photos, Kind: a.kind, Cover: a.cover,
	}
}

func (a *Album) Kind() AlbumKind {
	return a.kind
}

// CheckManual refuses to rename, delete or fill by hand an album the site
// keeps, as openvk.
func (a *Album) CheckManual() error {
	if a.kind != AlbumRegular {
		return ErrSystemAlbum
	}

	return nil
}

// CanBeChangedBy: its owner or, admin, whoever runs the community that
// owns it renames it, deletes it and adds photos to it, as in openvk.
func (a *Album) CanBeChangedBy(userID uuid.UUID, admin bool) bool {
	return admin || userID == a.ownerID
}

// Cover is the photo chosen to show the album, uuid.Nil for its first.
func (a *Album) Cover() uuid.UUID {
	return a.cover
}

// CheckCover tells whether p may show the album ("Обложка альбома"): a
// photo of the album itself.
func (a *Album) CheckCover(p *AlbumPhoto) error {
	if p.albumID != a.id {
		return ErrAlbumPhotoNotFound
	}

	return nil
}

// CheckMove tells whether a photo of a may go to the album to: another
// album of the same owner that people fill by hand. Out of an album the
// site keeps a photo moves as out of any.
func (a *Album) CheckMove(to *Album) error {
	if to.id == a.id || to.ownerID != a.ownerID {
		return ErrCannotMovePhoto
	}

	return to.CheckManual()
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
	// likes and comments count them, as a post counts its.
	likes    int
	comments int
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
	Likes       int
	Comments    int
}

// RestoreAlbumPhoto rebuilds a photo from storage without validation.
func RestoreAlbumPhoto(s StoredAlbumPhoto) *AlbumPhoto {
	return &AlbumPhoto{
		id: s.ID, albumID: s.AlbumID, authorID: s.AuthorID, description: s.Description, createdAt: s.CreatedAt,
		likes: s.Likes, comments: s.Comments,
	}
}

// Likes and Comments count the likes and the comments of the photo.
func (p *AlbumPhoto) Likes() int {
	return p.likes
}

func (p *AlbumPhoto) Comments() int {
	return p.comments
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
