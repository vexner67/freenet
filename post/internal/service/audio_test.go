package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/inmem"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
)

// fakeAudioKeeper records the songs media was told to delete, and in how
// many calls it was not in a transaction.
type fakeAudioKeeper struct {
	deleted   []photoCall
	outsideTx int
}

func (f *fakeAudioKeeper) DeleteAudio(ctx context.Context, owner uuid.UUID, ids []uuid.UUID) error {
	if ctx.Value(inTx{}) == nil {
		f.outsideTx++
	}

	f.deleted = append(f.deleted, photoCall{owner, ids})

	return nil
}

func newMusic(t *testing.T) (*service.AudioService, *service.PlaylistService, *fakeAudioKeeper, inmem.Music) {
	t.Helper()

	m := inmem.NewMusic()
	keeper := &fakeAudioKeeper{}
	audio := service.NewAudioService(m.Audios, m.Lists, m.Profiles, fakeTx{}, keeper)

	return audio, service.NewPlaylistService(m.Playlists, m.Lists, audio), keeper, m
}

func uploadSong(t *testing.T, svc *service.AudioService, owner uuid.UUID, title post.AudioName) *post.Audio {
	t.Helper()

	a, err := svc.AddFile(context.Background(), uuid.Must(uuid.NewV7()), post.AudioDraft{OwnerID: owner, Artist: "Кино", Title: title, Duration: 200})
	if err != nil {
		t.Fatalf("AddFile: %v", err)
	}

	return a
}

func addTrack(t *testing.T, svc *service.AudioService, owner uuid.UUID, link string) *post.Audio {
	t.Helper()

	source, err := post.NewAudioSource(post.AudioYouTube, link, "")
	if err != nil {
		t.Fatal(err)
	}

	a, err := svc.AddTrack(context.Background(), post.AudioDraft{OwnerID: owner, Title: "Клип"}, source)
	if err != nil {
		t.Fatalf("AddTrack: %v", err)
	}

	return a
}

func TestAudioLibrary(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc, _, _, _ := newMusic(t)
	ann, bob := uuid.New(), uuid.New()

	first := uploadSong(t, svc, ann, "Звезда")
	second := uploadSong(t, svc, ann, "Кукушка")

	page, err := svc.Library(ctx, bob, ann, uuid.Nil, 1)
	if err != nil || len(page.Audios) != 1 || page.Audios[0].ID() != second.ID() || page.Total != 2 || page.NextCursor == uuid.Nil {
		t.Fatalf("Library = %+v, %v", page, err)
	}
	if page.Added[second.ID()] {
		t.Fatal("bob keeps nothing yet")
	}

	next, err := svc.Library(ctx, bob, ann, page.NextCursor, 1)
	if err != nil || len(next.Audios) != 1 || next.Audios[0].ID() != first.ID() || next.NextCursor != uuid.Nil {
		t.Fatalf("the next page = %+v, %v", next, err)
	}

	// Bob keeps a song of Ann's, twice is fine.
	for range 2 {
		if err = svc.Keep(ctx, bob, first.ID()); err != nil {
			t.Fatalf("Keep: %v", err)
		}
	}
	if page, _ = svc.Library(ctx, bob, ann, uuid.Nil, 10); !page.Added[first.ID()] || page.Added[second.ID()] {
		t.Fatalf("what bob keeps: %v", page.Added)
	}
	if err = svc.Keep(ctx, bob, uuid.New()); !errors.Is(err, post.ErrAudioNotFound) {
		t.Fatalf("a missing song: %v", err)
	}

	if err = svc.Forget(ctx, bob, first.ID()); err != nil {
		t.Fatal(err)
	}
	if page, _ = svc.Library(ctx, bob, bob, uuid.Nil, 10); page.Total != 0 {
		t.Fatalf("after Forget: %+v", page)
	}
}

func TestAudioTracksAreShared(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc, _, _, _ := newMusic(t)
	ann, bob := uuid.New(), uuid.New()

	first := addTrack(t, svc, ann, "https://youtu.be/dQw4w9WgXcQ")
	again := addTrack(t, svc, bob, "https://www.youtube.com/watch?v=dQw4w9WgXcQ")
	if again.ID() != first.ID() {
		t.Fatal("the same track is two songs")
	}

	page, err := svc.Library(ctx, bob, bob, uuid.Nil, 10)
	if err != nil || page.Total != 1 || !page.Added[first.ID()] {
		t.Fatalf("bob's library = %+v, %v", page, err)
	}

	// Nobody owns a shared track to edit or delete it.
	if _, err = svc.Edit(ctx, ann, first.ID(), "", "Моё"); !errors.Is(err, post.ErrCannotChangeAudio) {
		t.Fatalf("Edit a track: %v", err)
	}
	if err = svc.Delete(ctx, ann, first.ID()); !errors.Is(err, post.ErrCannotChangeAudio) {
		t.Fatalf("Delete a track: %v", err)
	}
}

func TestAudioEditAndDelete(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc, playlists, keeper, _ := newMusic(t)
	ann, bob := uuid.New(), uuid.New()
	song := uploadSong(t, svc, ann, "Звезда")

	edited, err := svc.Edit(ctx, ann, song.ID(), "Кино", "Звезда по имени Солнце")
	if err != nil || edited.Title() != "Звезда по имени Солнце" {
		t.Fatalf("Edit = %v, %v", edited, err)
	}
	if _, err = svc.Edit(ctx, bob, song.ID(), "", ""); !errors.Is(err, post.ErrCannotChangeAudio) {
		t.Fatalf("Edit by another: %v", err)
	}

	// Bob keeps it and puts it on his page and in a playlist.
	if err = svc.Keep(ctx, bob, song.ID()); err != nil {
		t.Fatal(err)
	}
	if err = svc.SetPageAudio(ctx, bob, song.ID()); err != nil {
		t.Fatal(err)
	}
	list, err := playlists.Create(ctx, bob, "Лучшее")
	if err != nil {
		t.Fatal(err)
	}
	if err = playlists.Add(ctx, bob, list.ID(), song.ID()); err != nil {
		t.Fatal(err)
	}

	if err = svc.Delete(ctx, bob, song.ID()); !errors.Is(err, post.ErrCannotChangeAudio) {
		t.Fatalf("Delete by another: %v", err)
	}
	if err = svc.Delete(ctx, ann, song.ID()); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(keeper.deleted) != 1 || keeper.deleted[0].owner != ann || keeper.outsideTx != 0 {
		t.Fatalf("media was told %+v, outside a transaction %d times", keeper.deleted, keeper.outsideTx)
	}

	// It is gone from everything.
	if page, _ := svc.Library(ctx, bob, bob, uuid.Nil, 10); page.Total != 0 {
		t.Fatalf("bob's library: %+v", page)
	}
	if m, _ := svc.Music(ctx, bob); m.PageAudio != nil {
		t.Fatal("the song of the page stays")
	}
	if _, page, _ := playlists.Get(ctx, bob, list.ID(), uuid.Nil, 10); page.Total != 0 {
		t.Fatalf("the playlist: %+v", page)
	}
}

func TestAudioSearch(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc, _, _, _ := newMusic(t)
	ann := uuid.New()
	star := uploadSong(t, svc, ann, "Звезда по имени Солнце")
	uploadSong(t, svc, ann, "Кукушка")

	page, err := svc.Search(ctx, ann, "  ЗВЕЗДА ", uuid.Nil, 10)
	if err != nil || len(page.Audios) != 1 || page.Audios[0].ID() != star.ID() || !page.Added[star.ID()] {
		t.Fatalf("Search = %+v, %v", page, err)
	}

	if page, _ = svc.Search(ctx, uuid.Nil, "", uuid.Nil, 10); len(page.Audios) != 2 {
		t.Fatalf("all, for a guest: %+v", page)
	}
}

func TestAudioMusicOfThePage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc, _, _, _ := newMusic(t)
	ann := uuid.New()
	song := uploadSong(t, svc, ann, "Звезда")

	if err := svc.Listen(ctx, ann, song.ID()); err != nil {
		t.Fatal(err)
	}
	m, err := svc.Music(ctx, ann)
	if err != nil || m.Listening == nil || m.Listening.ID() != song.ID() || m.Count != 1 {
		t.Fatalf("Music = %+v, %v", m, err)
	}

	if err = svc.Listen(ctx, ann, uuid.Nil); err != nil {
		t.Fatal(err)
	}
	if m, _ = svc.Music(ctx, ann); m.Listening != nil {
		t.Fatal("still listening after the song stopped")
	}

	if err = svc.SetPageAudio(ctx, ann, uuid.New()); !errors.Is(err, post.ErrAudioNotFound) {
		t.Fatalf("a missing song on the page: %v", err)
	}
}

func TestPlaylists(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc, playlists, _, _ := newMusic(t)
	ann, bob := uuid.New(), uuid.New()
	song := uploadSong(t, svc, ann, "Звезда")

	list, err := playlists.Create(ctx, ann, "В дорогу")
	if err != nil {
		t.Fatal(err)
	}

	if err = playlists.Add(ctx, bob, list.ID(), song.ID()); !errors.Is(err, post.ErrCannotChangePlaylist) {
		t.Fatalf("Add by another: %v", err)
	}
	for range 2 {
		if err = playlists.Add(ctx, ann, list.ID(), song.ID()); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}

	got, page, err := playlists.Get(ctx, bob, list.ID(), uuid.Nil, 10)
	if err != nil || got.Title() != "В дорогу" || page.Total != 1 || page.Added[song.ID()] {
		t.Fatalf("Get = %v, %+v, %v", got, page, err)
	}

	if _, err = playlists.Rename(ctx, ann, list.ID(), "Домой"); err != nil {
		t.Fatal(err)
	}
	if all, _ := playlists.List(ctx, ann, uuid.Nil, 10); all.Total != 1 || all.Playlists[0].Title() != "Домой" {
		t.Fatalf("List = %+v", all)
	}

	if err = playlists.Remove(ctx, ann, list.ID(), song.ID()); err != nil {
		t.Fatal(err)
	}
	if err = playlists.Delete(ctx, bob, list.ID()); !errors.Is(err, post.ErrCannotChangePlaylist) {
		t.Fatalf("Delete by another: %v", err)
	}
	if err = playlists.Delete(ctx, ann, list.ID()); err != nil {
		t.Fatal(err)
	}
	if _, _, err = playlists.Get(ctx, ann, list.ID(), uuid.Nil, 10); !errors.Is(err, post.ErrPlaylistNotFound) {
		t.Fatalf("a deleted playlist: %v", err)
	}
}

func TestPurgeService_Music(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := inmem.NewStore()
	svc, playlists, _, m := newMusic(t)
	purge := service.NewPurgeService(service.PurgeDeps{
		Posts: store.Posts(), Likes: store.Likes(), CommentLikes: store.CommentLikes(), Votes: store.Polls(), Sources: store.Sources(), Comments: store.Comments(), Suggestions: inmem.NewSuggestionRepository(),
		Topics: inmem.NewTopicRepository(), Albums: inmem.NewAlbumRepository(), Tx: fakeTx{}, Notifier: &fakeNotifier{}, Photos: &fakePhotos{},
		Postponed: inmem.NewPostponedRepository(), Audio: m.Audios,
	})
	ann, bob := uuid.New(), uuid.New()

	annsSong := uploadSong(t, svc, ann, "Звезда")
	track := addTrack(t, svc, ann, "https://youtu.be/dQw4w9WgXcQ")
	bobsSong := uploadSong(t, svc, bob, "Кукушка")
	list, err := playlists.Create(ctx, ann, "Моё")
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.Keep(ctx, bob, annsSong.ID()); err != nil {
		t.Fatal(err)
	}
	if err = svc.Keep(ctx, bob, track.ID()); err != nil {
		t.Fatal(err)
	}

	if err = purge.PurgeUser(ctx, ann); err != nil {
		t.Fatalf("PurgeUser: %v", err)
	}

	// Ann's upload is gone from Bob too; the track stays, as Bob's song.
	page, err := svc.Library(ctx, bob, bob, uuid.Nil, 10)
	if err != nil || page.Total != 2 || page.Added[annsSong.ID()] || !page.Added[track.ID()] || !page.Added[bobsSong.ID()] {
		t.Fatalf("bob's library = %+v, %v", page, err)
	}
	if page, _ = svc.Library(ctx, ann, ann, uuid.Nil, 10); page.Total != 0 {
		t.Fatalf("ann's library: %+v", page)
	}
	if _, _, err = playlists.Get(ctx, ann, list.ID(), uuid.Nil, 10); !errors.Is(err, post.ErrPlaylistNotFound) {
		t.Fatalf("ann's playlist: %v", err)
	}
}
