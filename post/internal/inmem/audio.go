package inmem

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
)

// music is what the repositories of songs share, as the tables of
// PostgreSQL share their foreign keys: a deleted song leaves the lists and
// the pages, a deleted playlist takes its list. Songs and playlists handed
// out never change, so sharing pointers with callers is safe.
type music struct {
	mu        sync.RWMutex
	audios    map[uuid.UUID]*post.Audio
	lists     map[uuid.UUID][]service.AudioEntry // by list, the order added
	profiles  map[uuid.UUID]service.MusicProfile
	playlists map[uuid.UUID]*post.Playlist
}

// Music are the repositories of "Аудиозаписи" over one store.
type Music struct {
	Audios    *AudioRepository
	Lists     *AudioListRepository
	Profiles  *MusicRepository
	Playlists *PlaylistRepository
}

func NewMusic() Music {
	m := &music{
		audios:    make(map[uuid.UUID]*post.Audio),
		lists:     make(map[uuid.UUID][]service.AudioEntry),
		profiles:  make(map[uuid.UUID]service.MusicProfile),
		playlists: make(map[uuid.UUID]*post.Playlist),
	}

	return Music{Audios: &AudioRepository{m}, Lists: &AudioListRepository{m}, Profiles: &MusicRepository{m}, Playlists: &PlaylistRepository{m}}
}

type AudioRepository struct{ m *music }

func (r *AudioRepository) Save(ctx context.Context, a *post.Audio) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.m.mu.Lock()
	defer r.m.mu.Unlock()

	r.m.audios[a.ID()] = a

	return nil
}

func (r *AudioRepository) SaveTrack(ctx context.Context, a *post.Audio) (*post.Audio, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.m.mu.Lock()
	defer r.m.mu.Unlock()

	for _, known := range r.m.audios {
		if known.Source().Link() == a.Source().Link() {
			return known, nil
		}
	}
	r.m.audios[a.ID()] = a

	return a, nil
}

func (r *AudioRepository) Get(ctx context.Context, id uuid.UUID) (*post.Audio, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.m.mu.RLock()
	defer r.m.mu.RUnlock()

	a, ok := r.m.audios[id]
	if !ok {
		return nil, post.ErrAudioNotFound
	}

	return a, nil
}

func (r *AudioRepository) Update(ctx context.Context, a *post.Audio) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.m.mu.Lock()
	defer r.m.mu.Unlock()

	if _, ok := r.m.audios[a.ID()]; !ok {
		return post.ErrAudioNotFound
	}
	r.m.audios[a.ID()] = a

	return nil
}

func (r *AudioRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.m.mu.Lock()
	defer r.m.mu.Unlock()

	if _, ok := r.m.audios[id]; !ok {
		return post.ErrAudioNotFound
	}
	r.m.forget(id)

	return nil
}

// forget takes song id out of everything; the caller holds the lock.
func (m *music) forget(id uuid.UUID) {
	delete(m.audios, id)

	for listID, entries := range m.lists {
		m.lists[listID] = slices.DeleteFunc(entries, func(e service.AudioEntry) bool { return e.Audio.ID() == id })
	}

	for userID, p := range m.profiles {
		if p.Listening == id {
			p.Listening, p.ListeningAt = uuid.Nil, time.Time{}
		}
		if p.PageAudio == id {
			p.PageAudio = uuid.Nil
		}
		m.profiles[userID] = p
	}
}

func (r *AudioRepository) GetMany(ctx context.Context, ids []uuid.UUID) ([]*post.Audio, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.m.mu.RLock()
	defer r.m.mu.RUnlock()

	var found []*post.Audio
	for _, id := range ids {
		if a, ok := r.m.audios[id]; ok {
			found = append(found, a)
		}
	}

	return found, nil
}

func (r *AudioRepository) Search(ctx context.Context, query string, before uuid.UUID, limit int) ([]*post.Audio, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.m.mu.RLock()
	defer r.m.mu.RUnlock()

	query = strings.ToLower(query)

	var found []*post.Audio
	for _, a := range r.m.audios {
		name := strings.ToLower(a.Artist().String() + " " + a.Title().String())
		if (before == uuid.Nil || a.ID().String() < before.String()) && strings.Contains(name, query) {
			found = append(found, a)
		}
	}

	slices.SortFunc(found, func(a, b *post.Audio) int { return strings.Compare(b.ID().String(), a.ID().String()) })

	return found[:min(limit, len(found))], nil
}

// DeleteByUser deletes what a user keeps of music: the songs they uploaded,
// their library and playlists, their page; tracks of other sites stay for
// those who keep them.
func (r *AudioRepository) DeleteByUser(ctx context.Context, userID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.m.mu.Lock()
	defer r.m.mu.Unlock()

	for id, a := range r.m.audios {
		if a.OwnerID() == userID && a.Source().Kind() == post.AudioFile {
			r.m.forget(id)
		}
	}

	for id, p := range r.m.playlists {
		if p.OwnerID() == userID {
			delete(r.m.playlists, id)
			delete(r.m.lists, id)
		}
	}

	delete(r.m.lists, userID)
	delete(r.m.profiles, userID)

	return nil
}

type AudioListRepository struct{ m *music }

func (r *AudioListRepository) Add(ctx context.Context, listID, audioID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.m.mu.Lock()
	defer r.m.mu.Unlock()

	a, ok := r.m.audios[audioID]
	if !ok {
		return post.ErrAudioNotFound
	}

	entries := r.m.lists[listID]
	if slices.ContainsFunc(entries, func(e service.AudioEntry) bool { return e.Audio.ID() == audioID }) {
		return nil
	}

	id, err := uuid.NewV7()
	if err != nil {
		return errs.Wrap("uuid.NewV7", err)
	}
	r.m.lists[listID] = append(entries, service.AudioEntry{ID: id, Audio: a})

	return nil
}

func (r *AudioListRepository) Remove(ctx context.Context, listID, audioID uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.m.mu.Lock()
	defer r.m.mu.Unlock()

	r.m.lists[listID] = slices.DeleteFunc(r.m.lists[listID], func(e service.AudioEntry) bool { return e.Audio.ID() == audioID })

	return nil
}

func (r *AudioListRepository) Entries(ctx context.Context, listID, before uuid.UUID, limit int) ([]service.AudioEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.m.mu.RLock()
	defer r.m.mu.RUnlock()

	var found []service.AudioEntry
	entries := r.m.lists[listID]
	for i := len(entries) - 1; i >= 0 && len(found) < limit; i-- {
		e := entries[i]
		if before == uuid.Nil || e.ID.String() < before.String() {
			// The song as it is now, edited maybe after it was added.
			found = append(found, service.AudioEntry{ID: e.ID, Audio: r.m.audios[e.Audio.ID()]})
		}
	}

	return found, nil
}

func (r *AudioListRepository) Count(ctx context.Context, listID uuid.UUID) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, errs.Wrap("ctx.Err", err)
	}

	r.m.mu.RLock()
	defer r.m.mu.RUnlock()

	return len(r.m.lists[listID]), nil
}

func (r *AudioListRepository) Contains(ctx context.Context, listID uuid.UUID, audioIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.m.mu.RLock()
	defer r.m.mu.RUnlock()

	has := make(map[uuid.UUID]bool)
	for _, e := range r.m.lists[listID] {
		if slices.Contains(audioIDs, e.Audio.ID()) {
			has[e.Audio.ID()] = true
		}
	}

	return has, nil
}

type MusicRepository struct{ m *music }

func (r *MusicRepository) SetListening(ctx context.Context, userID, audioID uuid.UUID, at time.Time) error {
	return r.set(ctx, userID, audioID, func(p *service.MusicProfile) {
		p.Listening, p.ListeningAt = audioID, at
		if audioID == uuid.Nil {
			p.ListeningAt = time.Time{}
		}
	})
}

func (r *MusicRepository) SetPageAudio(ctx context.Context, userID, audioID uuid.UUID) error {
	return r.set(ctx, userID, audioID, func(p *service.MusicProfile) { p.PageAudio = audioID })
}

func (r *MusicRepository) set(ctx context.Context, userID, audioID uuid.UUID, change func(*service.MusicProfile)) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.m.mu.Lock()
	defer r.m.mu.Unlock()

	if _, ok := r.m.audios[audioID]; !ok && audioID != uuid.Nil {
		return post.ErrAudioNotFound
	}

	p := r.m.profiles[userID]
	change(&p)
	r.m.profiles[userID] = p

	return nil
}

func (r *MusicRepository) Profile(ctx context.Context, userID uuid.UUID) (service.MusicProfile, error) {
	if err := ctx.Err(); err != nil {
		return service.MusicProfile{}, errs.Wrap("ctx.Err", err)
	}

	r.m.mu.RLock()
	defer r.m.mu.RUnlock()

	return r.m.profiles[userID], nil
}

type PlaylistRepository struct{ m *music }

func (r *PlaylistRepository) Save(ctx context.Context, p *post.Playlist) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.m.mu.Lock()
	defer r.m.mu.Unlock()

	r.m.playlists[p.ID()] = p

	return nil
}

func (r *PlaylistRepository) Get(ctx context.Context, id uuid.UUID) (*post.Playlist, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.m.mu.RLock()
	defer r.m.mu.RUnlock()

	p, ok := r.m.playlists[id]
	if !ok {
		return nil, post.ErrPlaylistNotFound
	}

	return p, nil
}

func (r *PlaylistRepository) Update(ctx context.Context, p *post.Playlist) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.m.mu.Lock()
	defer r.m.mu.Unlock()

	if _, ok := r.m.playlists[p.ID()]; !ok {
		return post.ErrPlaylistNotFound
	}
	r.m.playlists[p.ID()] = p

	return nil
}

func (r *PlaylistRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return errs.Wrap("ctx.Err", err)
	}

	r.m.mu.Lock()
	defer r.m.mu.Unlock()

	if _, ok := r.m.playlists[id]; !ok {
		return post.ErrPlaylistNotFound
	}
	delete(r.m.playlists, id)
	delete(r.m.lists, id)

	return nil
}

func (r *PlaylistRepository) List(ctx context.Context, ownerID, before uuid.UUID, limit int) ([]*post.Playlist, error) {
	if err := ctx.Err(); err != nil {
		return nil, errs.Wrap("ctx.Err", err)
	}

	r.m.mu.RLock()
	defer r.m.mu.RUnlock()

	var found []*post.Playlist
	for _, p := range r.m.playlists {
		if p.OwnerID() == ownerID && (before == uuid.Nil || p.ID().String() < before.String()) {
			found = append(found, p)
		}
	}

	slices.SortFunc(found, func(a, b *post.Playlist) int { return strings.Compare(b.ID().String(), a.ID().String()) })

	return found[:min(limit, len(found))], nil
}

func (r *PlaylistRepository) Count(ctx context.Context, ownerID uuid.UUID) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, errs.Wrap("ctx.Err", err)
	}

	r.m.mu.RLock()
	defer r.m.mu.RUnlock()

	n := 0
	for _, p := range r.m.playlists {
		if p.OwnerID() == ownerID {
			n++
		}
	}

	return n, nil
}
