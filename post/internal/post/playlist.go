package post

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	maxPlaylistTitleLen = 128
	// MaxPlaylistAudios is how many songs a playlist keeps.
	MaxPlaylistAudios = 1000
)

var (
	ErrPlaylistNotFound     = errors.New("playlist not found")
	ErrPlaylistTitleEmpty   = errors.New("playlist title is empty")
	ErrPlaylistTitleTooLong = errors.New("playlist title is too long")
	ErrCannotChangePlaylist = errors.New("only the owner can change the playlist")
	ErrPlaylistFull         = errors.New("a playlist has at most 1000 audios")
)

// PlaylistTitle is what a playlist is called: one line, trimmed.
type PlaylistTitle string

func NewPlaylistTitle(s string) (PlaylistTitle, error) {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return "", ErrPlaylistTitleEmpty
	}

	if utf8.RuneCountInString(s) > maxPlaylistTitleLen {
		return "", ErrPlaylistTitleTooLong
	}

	return PlaylistTitle(s), nil
}

func (t PlaylistTitle) String() string {
	return string(t)
}

// Playlist is a list of songs a person makes (openvk's playlists): anyone
// reads it, only its owner changes it.
type Playlist struct {
	id        uuid.UUID
	ownerID   uuid.UUID
	title     PlaylistTitle
	createdAt time.Time
}

func NewPlaylist(ownerID uuid.UUID, title PlaylistTitle, now time.Time) (*Playlist, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}

	return &Playlist{id: id, ownerID: ownerID, title: title, createdAt: now.UTC().Truncate(time.Microsecond)}, nil
}

// StoredPlaylist is a playlist as storage keeps it.
type StoredPlaylist struct {
	ID        uuid.UUID
	OwnerID   uuid.UUID
	Title     PlaylistTitle
	CreatedAt time.Time
}

func RestorePlaylist(s StoredPlaylist) *Playlist {
	return &Playlist{id: s.ID, ownerID: s.OwnerID, title: s.Title, createdAt: s.CreatedAt}
}

// Renamed is p called title, by its owner only.
func (p *Playlist) Renamed(requesterID uuid.UUID, title PlaylistTitle) (*Playlist, error) {
	if !p.CanBeChangedBy(requesterID) {
		return nil, ErrCannotChangePlaylist
	}

	renamed := *p
	renamed.title = title

	return &renamed, nil
}

// CanBeChangedBy: only the owner renames, fills or deletes a playlist.
func (p *Playlist) CanBeChangedBy(userID uuid.UUID) bool {
	return userID == p.ownerID
}

func (p *Playlist) ID() uuid.UUID        { return p.id }
func (p *Playlist) OwnerID() uuid.UUID   { return p.ownerID }
func (p *Playlist) Title() PlaylistTitle { return p.title }
func (p *Playlist) CreatedAt() time.Time { return p.createdAt }
