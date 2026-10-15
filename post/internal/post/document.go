package post

import (
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	// maxDocumentNameLen is how long the name of a document may be.
	maxDocumentNameLen = 128
	// maxDocuments is how many documents a post or a message attaches.
	maxDocuments = 10
)

var (
	ErrDocumentNotFound     = errors.New("document not found")
	ErrDocumentNameEmpty    = errors.New("document name is empty")
	ErrDocumentNameTooLong  = errors.New("document name is too long")
	ErrDocumentFormat       = errors.New("this file format is not allowed")
	ErrInvalidDocumentSize  = errors.New("invalid document size")
	ErrCannotChangeDocument = errors.New("only the owner can change the document")
	ErrTooManyDocuments     = errors.New("a post has at most 10 documents")
	ErrDuplicateDocument    = errors.New("the same document is attached twice")
	ErrInvalidDocument      = errors.New("invalid document id")
)

// documentFormats are the extensions openvk allows by default
// (docs.allowedFormats).
var documentFormats = []string{
	"jpg", "jpeg", "png", "gif", "webp", "psd", "aep", "docx", "doc", "odt", "txt", "md", "pptx", "ppt",
	"xls", "xlsx", "pdf", "djvu", "fb2", "ps", "apk", "zip", "7z", "mp4", "avi", "mp3", "flac",
}

// DocumentName is what a document is called: one line, trimmed, the name
// of the file at first.
type DocumentName string

func NewDocumentName(s string) (DocumentName, error) {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return "", ErrDocumentNameEmpty
	}

	if utf8.RuneCountInString(s) > maxDocumentNameLen {
		return "", ErrDocumentNameTooLong
	}

	return DocumentName(s), nil
}

func (n DocumentName) String() string {
	return string(n)
}

// DocumentExt is the format of a document: the extension of its file in
// lower case, one of those openvk allows.
type DocumentExt string

// NewDocumentExt takes the extension of a file name or the extension
// itself, in any case.
func NewDocumentExt(s string) (DocumentExt, error) {
	ext := strings.ToLower(strings.TrimSpace(s))
	if i := strings.LastIndexByte(ext, '.'); i >= 0 {
		ext = ext[i+1:]
	}

	if !slices.Contains(documentFormats, ext) {
		return "", ErrDocumentFormat
	}

	return DocumentExt(ext), nil
}

func (e DocumentExt) String() string {
	return string(e)
}

// Document is a file of "Мои Документы" (openvk's documents): its bytes
// are in the media service under the same ID and its owner, the name and
// the format here. Posts and messages attach it by ID; deleted, it is gone
// from them too.
type Document struct {
	id        uuid.UUID
	ownerID   uuid.UUID
	name      DocumentName
	ext       DocumentExt
	size      int64
	createdAt time.Time
}

// NewDocument adds the file id the media service keeps for ownerID; its
// UUIDv7 grows with time, so the documents of a person are paged by ID.
func NewDocument(id, ownerID uuid.UUID, name DocumentName, ext DocumentExt, size int64, now time.Time) (*Document, error) {
	if id == uuid.Nil {
		return nil, ErrInvalidDocument
	}

	if size <= 0 {
		return nil, ErrInvalidDocumentSize
	}

	return &Document{
		id: id, ownerID: ownerID, name: name, ext: ext, size: size,
		// PostgreSQL keeps microseconds; trim now so it reads back equal.
		createdAt: now.UTC().Truncate(time.Microsecond),
	}, nil
}

// StoredDocument is a document as storage keeps it.
type StoredDocument struct {
	ID        uuid.UUID
	OwnerID   uuid.UUID
	Name      DocumentName
	Ext       DocumentExt
	Size      int64
	CreatedAt time.Time
}

func RestoreDocument(s StoredDocument) *Document {
	return &Document{id: s.ID, ownerID: s.OwnerID, name: s.Name, ext: s.Ext, size: s.Size, createdAt: s.CreatedAt}
}

// Renamed is d called name, "Редактирование документа": by its owner only.
func (d *Document) Renamed(requesterID uuid.UUID, name DocumentName) (*Document, error) {
	if !d.CanBeChangedBy(requesterID) {
		return nil, ErrCannotChangeDocument
	}

	renamed := *d
	renamed.name = name

	return &renamed, nil
}

// CanBeChangedBy: only the owner renames or deletes a document.
func (d *Document) CanBeChangedBy(userID uuid.UUID) bool {
	return userID == d.ownerID
}

func (d *Document) ID() uuid.UUID        { return d.id }
func (d *Document) OwnerID() uuid.UUID   { return d.ownerID }
func (d *Document) Name() DocumentName   { return d.name }
func (d *Document) Ext() DocumentExt     { return d.ext }
func (d *Document) Size() int64          { return d.size }
func (d *Document) CreatedAt() time.Time { return d.createdAt }

// Documents are the IDs of the documents a post attaches, in the order
// they are shown. Whose they are is checked when the post is written: the
// gateway asks for them.
type Documents []uuid.UUID

func NewDocuments(ids []uuid.UUID) (Documents, error) {
	if len(ids) > maxDocuments {
		return nil, ErrTooManyDocuments
	}

	seen := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		if id == uuid.Nil {
			return nil, ErrInvalidDocument
		}
		if seen[id] {
			return nil, ErrDuplicateDocument
		}
		seen[id] = true
	}

	return Documents(ids), nil
}
