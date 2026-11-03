package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/post/internal/post"
)

type PlaylistRepository interface {
	Save(ctx context.Context, p *post.Playlist) error
	// Get, Update and Delete return post.ErrPlaylistNotFound; a deleted
	// playlist takes its list of songs along.
	Get(ctx context.Context, id uuid.UUID) (*post.Playlist, error)
	Update(ctx context.Context, p *post.Playlist) error
	Delete(ctx context.Context, id uuid.UUID) error
	// List returns up to limit playlists of an owner, newest first, with
	// IDs below before (uuid.Nil: from the newest); Count counts them.
	List(ctx context.Context, ownerID, before uuid.UUID, limit int) ([]*post.Playlist, error)
	Count(ctx context.Context, ownerID uuid.UUID) (int, error)
}

// PlaylistPage is one screen of the playlists of a person.
type PlaylistPage struct {
	Playlists  []*post.Playlist
	NextCursor uuid.UUID
	Total      int
}

// PlaylistService keeps the playlists of openvk: lists of songs a person
// makes, which anyone listens to and only the owner changes.
type PlaylistService struct {
	playlists PlaylistRepository
	lists     AudioListRepository
	audio     *AudioService
	now       func() time.Time
}

func NewPlaylistService(playlists PlaylistRepository, lists AudioListRepository, audio *AudioService) *PlaylistService {
	return &PlaylistService{playlists: playlists, lists: lists, audio: audio, now: time.Now}
}

// Create makes an empty playlist of ownerID.
func (s *PlaylistService) Create(ctx context.Context, ownerID uuid.UUID, title post.PlaylistTitle) (*post.Playlist, error) {
	p, err := post.NewPlaylist(ownerID, title, s.now())
	if err != nil {
		return nil, errs.Wrap("post.NewPlaylist", err)
	}

	if err = s.playlists.Save(ctx, p); err != nil {
		return nil, errs.Wrap("s.playlists.Save", err)
	}

	return p, nil
}

func (s *PlaylistService) Rename(ctx context.Context, requesterID, id uuid.UUID, title post.PlaylistTitle) (*post.Playlist, error) {
	p, err := s.playlists.Get(ctx, id)
	if err != nil {
		return nil, errs.Wrap("s.playlists.Get", err)
	}

	renamed, err := p.Renamed(requesterID, title)
	if err != nil {
		return nil, errs.Wrap("p.Renamed", err)
	}

	if err = s.playlists.Update(ctx, renamed); err != nil {
		return nil, errs.Wrap("s.playlists.Update", err)
	}

	return renamed, nil
}

// Delete deletes a playlist of its owner; the songs stay.
func (s *PlaylistService) Delete(ctx context.Context, requesterID, id uuid.UUID) error {
	if _, err := s.owned(ctx, requesterID, id); err != nil {
		return err
	}

	return errs.Wrap("s.playlists.Delete", s.playlists.Delete(ctx, id))
}

// List pages the playlists of ownerID, newest first.
func (s *PlaylistService) List(ctx context.Context, ownerID, cursor uuid.UUID, size int) (PlaylistPage, error) {
	size = pageSize(size)

	found, err := s.playlists.List(ctx, ownerID, cursor, size+1)
	if err != nil {
		return PlaylistPage{}, errs.Wrap("s.playlists.List", err)
	}

	page := PlaylistPage{Playlists: found}
	if len(found) > size {
		page.Playlists = found[:size]
		page.NextCursor = found[size-1].ID()
	}

	if page.Total, err = s.playlists.Count(ctx, ownerID); err != nil {
		return PlaylistPage{}, errs.Wrap("s.playlists.Count", err)
	}

	return page, nil
}

// Get is a playlist with a page of its songs, the last added first.
func (s *PlaylistService) Get(ctx context.Context, viewerID, id, cursor uuid.UUID, size int) (*post.Playlist, AudioPage, error) {
	p, err := s.playlists.Get(ctx, id)
	if err != nil {
		return nil, AudioPage{}, errs.Wrap("s.playlists.Get", err)
	}

	page, err := s.audio.listPage(ctx, id, cursor, size)
	if err != nil {
		return nil, AudioPage{}, err
	}

	if page.Added, err = s.audio.added(ctx, viewerID, page.Audios); err != nil {
		return nil, AudioPage{}, err
	}

	return p, page, nil
}

// Add puts a song of anyone in a playlist of its owner, once.
func (s *PlaylistService) Add(ctx context.Context, requesterID, id, audioID uuid.UUID) error {
	if _, err := s.owned(ctx, requesterID, id); err != nil {
		return err
	}

	n, err := s.lists.Count(ctx, id)
	if err != nil {
		return errs.Wrap("s.lists.Count", err)
	}

	if n >= post.MaxPlaylistAudios {
		return post.ErrPlaylistFull
	}

	return errs.Wrap("s.lists.Add", s.lists.Add(ctx, id, audioID))
}

// Remove takes a song out of a playlist of its owner.
func (s *PlaylistService) Remove(ctx context.Context, requesterID, id, audioID uuid.UUID) error {
	if _, err := s.owned(ctx, requesterID, id); err != nil {
		return err
	}

	return errs.Wrap("s.lists.Remove", s.lists.Remove(ctx, id, audioID))
}

func (s *PlaylistService) owned(ctx context.Context, requesterID, id uuid.UUID) (*post.Playlist, error) {
	p, err := s.playlists.Get(ctx, id)
	if err != nil {
		return nil, errs.Wrap("s.playlists.Get", err)
	}

	if !p.CanBeChangedBy(requesterID) {
		return nil, post.ErrCannotChangePlaylist
	}

	return p, nil
}
