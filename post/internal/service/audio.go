package service

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/post/internal/post"
)

const (
	// listeningFor is how long "слушает: …" stays under a name after the
	// song started: longer than most songs, shorter than an evening.
	listeningFor = 10 * time.Minute
	// maxAudioQueryLen bounds a search of songs.
	maxAudioQueryLen = 100
)

type AudioRepository interface {
	// Save adds an uploaded song. SaveTrack adds a track of another site,
	// or, when someone added it first, returns that one.
	Save(ctx context.Context, a *post.Audio) error
	SaveTrack(ctx context.Context, a *post.Audio) (*post.Audio, error)
	// Get, Update and Delete return post.ErrAudioNotFound; a deleted song
	// leaves every list and post.
	Get(ctx context.Context, id uuid.UUID) (*post.Audio, error)
	Update(ctx context.Context, a *post.Audio) error
	Delete(ctx context.Context, id uuid.UUID) error
	// GetMany returns the songs that exist, in no particular order.
	GetMany(ctx context.Context, ids []uuid.UUID) ([]*post.Audio, error)
	// Search returns up to limit songs whose artist or title has query
	// (all for ""), newest first, with IDs below before (uuid.Nil: from
	// the newest).
	Search(ctx context.Context, query string, before uuid.UUID, limit int) ([]*post.Audio, error)
}

// AudioEntry is a song in a list, ID the place it was added at.
type AudioEntry struct {
	ID    uuid.UUID
	Audio *post.Audio
}

// AudioListRepository keeps lists of songs: the library of a person,
// under their ID, and playlists, under theirs.
type AudioListRepository interface {
	// Add puts a song at the top of a list once; post.ErrAudioNotFound
	// when the song is gone. Remove is fine with a song not there.
	Add(ctx context.Context, listID, audioID uuid.UUID) error
	Remove(ctx context.Context, listID, audioID uuid.UUID) error
	// Entries returns up to limit songs of a list, the last added first,
	// added before before (uuid.Nil: from the last); Count counts them.
	Entries(ctx context.Context, listID, before uuid.UUID, limit int) ([]AudioEntry, error)
	Count(ctx context.Context, listID uuid.UUID) (int, error)
	// Contains tells which of the songs the list has.
	Contains(ctx context.Context, listID uuid.UUID, audioIDs []uuid.UUID) (map[uuid.UUID]bool, error)
}

// MusicProfile is what a page says of music: the song its owner listens
// to now and since when, the song of the page; uuid.Nil for none.
type MusicProfile struct {
	Listening   uuid.UUID
	ListeningAt time.Time
	PageAudio   uuid.UUID
}

type MusicRepository interface {
	// SetListening and SetPageAudio return post.ErrAudioNotFound when
	// the song is gone; uuid.Nil clears them.
	SetListening(ctx context.Context, userID, audioID uuid.UUID, at time.Time) error
	SetPageAudio(ctx context.Context, userID, audioID uuid.UUID) error
	Profile(ctx context.Context, userID uuid.UUID) (MusicProfile, error)
}

// AudioKeeper records, inside the transaction of the change, that the
// media service has to delete the bytes of deleted songs.
type AudioKeeper interface {
	DeleteAudio(ctx context.Context, ownerID uuid.UUID, audioIDs []uuid.UUID) error
}

// AudioPage is one screen of songs; Added tells which of them the viewer
// keeps; NextCursor is uuid.Nil on the last, Total counts all the pages
// (not for a search).
type AudioPage struct {
	Audios     []*post.Audio
	Added      map[uuid.UUID]bool
	NextCursor uuid.UUID
	Total      int
}

// Music is what a page shows of music: the song its owner listens to now
// and the song of the page, nil for none, and how many songs they keep.
type Music struct {
	Listening *post.Audio
	PageAudio *post.Audio
	Count     int
}

// AudioService keeps "Аудиозаписи" of openvk: songs people upload or add
// from SoundCloud and YouTube, the library of each person, "слушает" and
// the song of the page. Anyone reads the library of anyone.
type AudioService struct {
	audios AudioRepository
	lists  AudioListRepository
	music  MusicRepository
	tx     TxManager
	keeper AudioKeeper
	now    func() time.Time
}

func NewAudioService(audios AudioRepository, lists AudioListRepository, music MusicRepository, tx TxManager, keeper AudioKeeper) *AudioService {
	return &AudioService{audios: audios, lists: lists, music: music, tx: tx, keeper: keeper, now: time.Now}
}

// AddFile keeps an MP3 the media service has just stored for its owner,
// in their library.
func (s *AudioService) AddFile(ctx context.Context, id uuid.UUID, d post.AudioDraft) (*post.Audio, error) {
	a, err := post.NewAudioFile(id, d, s.now())
	if err != nil {
		return nil, errs.Wrap("post.NewAudioFile", err)
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.audios.Save(ctx, a); err != nil {
			return errs.Wrap("s.audios.Save", err)
		}

		return errs.Wrap("s.lists.Add", s.lists.Add(ctx, d.OwnerID, a.ID()))
	})
	if err != nil {
		return nil, errs.Wrap("s.tx.WithinTx", err)
	}

	return a, nil
}

// AddTrack puts a track of another site in the library of d.OwnerID: the
// song of the first who added it, or a new one.
func (s *AudioService) AddTrack(ctx context.Context, d post.AudioDraft, source post.AudioSource) (*post.Audio, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return nil, errs.Wrap("uuid.NewV7", err)
	}

	a, err := post.NewAudioTrack(id, d, source, s.now())
	if err != nil {
		return nil, errs.Wrap("post.NewAudioTrack", err)
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if a, err = s.audios.SaveTrack(ctx, a); err != nil {
			return errs.Wrap("s.audios.SaveTrack", err)
		}

		return errs.Wrap("s.lists.Add", s.lists.Add(ctx, d.OwnerID, a.ID()))
	})
	if err != nil {
		return nil, errs.Wrap("s.tx.WithinTx", err)
	}

	return a, nil
}

// Library pages the songs ownerID keeps, the last added first.
func (s *AudioService) Library(ctx context.Context, viewerID, ownerID, cursor uuid.UUID, size int) (AudioPage, error) {
	page, err := s.listPage(ctx, ownerID, cursor, size)
	if err != nil {
		return AudioPage{}, err
	}

	if page.Added, err = s.added(ctx, viewerID, page.Audios); err != nil {
		return AudioPage{}, err
	}

	return page, nil
}

func (s *AudioService) listPage(ctx context.Context, listID, cursor uuid.UUID, size int) (AudioPage, error) {
	size = pageSize(size)

	// One extra tells whether there is a next page.
	entries, err := s.lists.Entries(ctx, listID, cursor, size+1)
	if err != nil {
		return AudioPage{}, errs.Wrap("s.lists.Entries", err)
	}

	var page AudioPage
	if len(entries) > size {
		entries = entries[:size]
		page.NextCursor = entries[size-1].ID
	}

	page.Audios = make([]*post.Audio, len(entries))
	for i, e := range entries {
		page.Audios[i] = e.Audio
	}

	if page.Total, err = s.lists.Count(ctx, listID); err != nil {
		return AudioPage{}, errs.Wrap("s.lists.Count", err)
	}

	return page, nil
}

// Search pages the songs of everyone whose artist or title has query,
// newest first; an empty query is "Новое" of openvk, all of them.
func (s *AudioService) Search(ctx context.Context, viewerID uuid.UUID, query string, cursor uuid.UUID, size int) (AudioPage, error) {
	size = pageSize(size)
	query = strings.Join(strings.Fields(query), " ")
	if utf8.RuneCountInString(query) > maxAudioQueryLen {
		query = string([]rune(query)[:maxAudioQueryLen])
	}

	found, err := s.audios.Search(ctx, query, cursor, size+1)
	if err != nil {
		return AudioPage{}, errs.Wrap("s.audios.Search", err)
	}

	page := AudioPage{Audios: found}
	if len(found) > size {
		page.Audios = found[:size]
		page.NextCursor = found[size-1].ID()
	}

	if page.Added, err = s.added(ctx, viewerID, page.Audios); err != nil {
		return AudioPage{}, err
	}

	return page, nil
}

// added tells which of the songs the viewer keeps; none for a guest.
func (s *AudioService) added(ctx context.Context, viewerID uuid.UUID, audios []*post.Audio) (map[uuid.UUID]bool, error) {
	if viewerID == uuid.Nil || len(audios) == 0 {
		return map[uuid.UUID]bool{}, nil
	}

	ids := make([]uuid.UUID, len(audios))
	for i, a := range audios {
		ids[i] = a.ID()
	}

	added, err := s.lists.Contains(ctx, viewerID, ids)

	return added, errs.Wrap("s.lists.Contains", err)
}

// ByID returns the songs with these IDs that exist, at most MaxPageSize,
// in no particular order, and which of them viewerID keeps.
func (s *AudioService) ByID(ctx context.Context, viewerID uuid.UUID, ids []uuid.UUID) (AudioPage, error) {
	if len(ids) > MaxPageSize {
		return AudioPage{}, ErrTooManyIDs
	}

	if len(ids) == 0 {
		return AudioPage{Added: map[uuid.UUID]bool{}}, nil
	}

	found, err := s.audios.GetMany(ctx, ids)
	if err != nil {
		return AudioPage{}, errs.Wrap("s.audios.GetMany", err)
	}

	added, err := s.added(ctx, viewerID, found)
	if err != nil {
		return AudioPage{}, err
	}

	return AudioPage{Audios: found, Added: added}, nil
}

// Keep puts a song of anyone in the library of userID ("Добавить к
// себе"); twice is fine.
func (s *AudioService) Keep(ctx context.Context, userID, audioID uuid.UUID) error {
	return errs.Wrap("s.lists.Add", s.lists.Add(ctx, userID, audioID))
}

// Forget takes a song out of the library of userID; the song stays for
// those who keep it, and its owner may delete it.
func (s *AudioService) Forget(ctx context.Context, userID, audioID uuid.UUID) error {
	return errs.Wrap("s.lists.Remove", s.lists.Remove(ctx, userID, audioID))
}

// Edit writes the artist and the title of an uploaded song: its owner only.
func (s *AudioService) Edit(ctx context.Context, requesterID, id uuid.UUID, artist, title post.AudioName) (*post.Audio, error) {
	a, err := s.audios.Get(ctx, id)
	if err != nil {
		return nil, errs.Wrap("s.audios.Get", err)
	}

	edited, err := a.Edited(requesterID, artist, title)
	if err != nil {
		return nil, errs.Wrap("a.Edited", err)
	}

	if err = s.audios.Update(ctx, edited); err != nil {
		return nil, errs.Wrap("s.audios.Update", err)
	}

	return edited, nil
}

// Delete deletes an uploaded song for everyone: its owner only. The media
// service deletes its bytes after; libraries, playlists and posts lose it.
func (s *AudioService) Delete(ctx context.Context, requesterID, id uuid.UUID) error {
	a, err := s.audios.Get(ctx, id)
	if err != nil {
		return errs.Wrap("s.audios.Get", err)
	}

	if !a.CanBeChangedBy(requesterID) {
		return post.ErrCannotChangeAudio
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.audios.Delete(ctx, id); err != nil {
			return errs.Wrap("s.audios.Delete", err)
		}

		return errs.Wrap("s.keeper.DeleteAudio", s.keeper.DeleteAudio(ctx, a.OwnerID(), []uuid.UUID{id}))
	})

	return errs.Wrap("s.tx.WithinTx", err)
}

// Listen says that userID listens to a song now ("слушает" under the
// name for a while); uuid.Nil says nothing plays.
func (s *AudioService) Listen(ctx context.Context, userID, audioID uuid.UUID) error {
	return errs.Wrap("s.music.SetListening", s.music.SetListening(ctx, userID, audioID, s.now().UTC()))
}

// SetPageAudio makes a song the song of the page of userID; uuid.Nil
// takes it off.
func (s *AudioService) SetPageAudio(ctx context.Context, userID, audioID uuid.UUID) error {
	return errs.Wrap("s.music.SetPageAudio", s.music.SetPageAudio(ctx, userID, audioID))
}

// Music is what the page of userID shows of music.
func (s *AudioService) Music(ctx context.Context, userID uuid.UUID) (Music, error) {
	profile, err := s.music.Profile(ctx, userID)
	if err != nil {
		return Music{}, errs.Wrap("s.music.Profile", err)
	}

	var ids []uuid.UUID
	fresh := profile.Listening != uuid.Nil && s.now().Sub(profile.ListeningAt) < listeningFor
	if fresh {
		ids = append(ids, profile.Listening)
	}
	if profile.PageAudio != uuid.Nil {
		ids = append(ids, profile.PageAudio)
	}

	var m Music
	if len(ids) > 0 {
		found, err := s.audios.GetMany(ctx, ids)
		if err != nil {
			return Music{}, errs.Wrap("s.audios.GetMany", err)
		}

		for _, a := range found {
			if fresh && a.ID() == profile.Listening {
				m.Listening = a
			}
			if a.ID() == profile.PageAudio {
				m.PageAudio = a
			}
		}
	}

	if m.Count, err = s.lists.Count(ctx, userID); err != nil {
		return Music{}, errs.Wrap("s.lists.Count", err)
	}

	return m, nil
}

func pageSize(size int) int {
	if size <= 0 || size > MaxPageSize {
		return DefaultPageSize
	}

	return size
}
