package postgres_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/postgres"
)

func TestAudioRepository(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool := newPool(t)
	audios := postgres.NewAudioRepository(pool)
	lists, music := audios.AudioLists(), audios.Music()
	ann, bob := uuid.New(), uuid.New()
	// A word nobody else's songs have, for the search.
	word := post.AudioName("песня-" + uuid.NewString()[:8])

	song, err := post.NewAudioFile(uuid.Must(uuid.NewV7()), post.AudioDraft{OwnerID: ann, Artist: "Кино", Title: word, Duration: 215}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = audios.Save(ctx, song); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := audios.Get(ctx, song.ID())
	if err != nil || !reflect.DeepEqual(got, song) {
		t.Fatalf("Get = %+v, %v; want %+v", got, err, song)
	}
	if _, err = audios.Get(ctx, uuid.New()); !errors.Is(err, post.ErrAudioNotFound) {
		t.Fatalf("Get a missing one: %v", err)
	}

	// A track is one song whoever adds it.
	link := "https://soundcloud.com/kino/" + uuid.NewString()
	source, err := post.NewAudioSource(post.AudioSoundCloud, link, "https://i1.sndcdn.com/a.jpg")
	if err != nil {
		t.Fatal(err)
	}
	track, err := post.NewAudioTrack(uuid.Must(uuid.NewV7()), post.AudioDraft{OwnerID: ann, Title: "Трек"}, source, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	kept, err := audios.SaveTrack(ctx, track)
	if err != nil || !reflect.DeepEqual(kept, track) {
		t.Fatalf("SaveTrack = %+v, %v", kept, err)
	}
	again, err := post.NewAudioTrack(uuid.Must(uuid.NewV7()), post.AudioDraft{OwnerID: bob, Title: "Другой"}, source, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if kept, err = audios.SaveTrack(ctx, again); err != nil || kept.ID() != track.ID() {
		t.Fatalf("SaveTrack of a known address = %+v, %v", kept, err)
	}

	// Lists: once each, the last added on top.
	for _, id := range []uuid.UUID{song.ID(), track.ID(), song.ID()} {
		if err = lists.Add(ctx, bob, id); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}
	if err = lists.Add(ctx, bob, uuid.New()); !errors.Is(err, post.ErrAudioNotFound) {
		t.Fatalf("Add a missing song: %v", err)
	}
	entries, err := lists.Entries(ctx, bob, uuid.Nil, 10)
	if err != nil || len(entries) != 2 || entries[0].Audio.ID() != track.ID() || entries[1].Audio.ID() != song.ID() {
		t.Fatalf("Entries = %+v, %v", entries, err)
	}
	if rest, _ := lists.Entries(ctx, bob, entries[0].ID, 10); len(rest) != 1 || rest[0].Audio.ID() != song.ID() {
		t.Fatalf("Entries after the first = %+v", rest)
	}
	if n, _ := lists.Count(ctx, bob); n != 2 {
		t.Fatalf("Count = %d", n)
	}
	if has, _ := lists.Contains(ctx, bob, []uuid.UUID{song.ID(), uuid.New()}); len(has) != 1 || !has[song.ID()] {
		t.Fatalf("Contains = %v", has)
	}

	// Search: the words, case aside, and % is no wildcard.
	if found, _ := audios.Search(ctx, string(word[:5])+"%", uuid.Nil, 10); len(found) != 0 {
		t.Fatalf("a wildcard matched %d", len(found))
	}
	found, err := audios.Search(ctx, "КИНО "+word.String(), uuid.Nil, 10)
	if err != nil || len(found) != 1 || found[0].ID() != song.ID() {
		t.Fatalf("Search = %+v, %v", found, err)
	}

	edited, err := song.Edited(ann, "Кино", word+" (live)")
	if err != nil {
		t.Fatal(err)
	}
	if err = audios.Update(ctx, edited); err != nil {
		t.Fatal(err)
	}
	if got, _ = audios.Get(ctx, song.ID()); got.Title() != edited.Title() {
		t.Fatalf("after Update: %q", got.Title())
	}

	// The page of Bob: what he listens to and its song.
	if err = music.SetListening(ctx, bob, song.ID(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if err = music.SetPageAudio(ctx, bob, song.ID()); err != nil {
		t.Fatal(err)
	}
	if err = music.SetPageAudio(ctx, ann, uuid.New()); !errors.Is(err, post.ErrAudioNotFound) {
		t.Fatalf("a missing song on the page: %v", err)
	}
	profile, err := music.Profile(ctx, bob)
	if err != nil || profile.Listening != song.ID() || profile.PageAudio != song.ID() || profile.ListeningAt.IsZero() {
		t.Fatalf("Profile = %+v, %v", profile, err)
	}

	// A deleted song leaves the lists and the page.
	if err = audios.Delete(ctx, song.ID()); err != nil {
		t.Fatal(err)
	}
	if n, _ := lists.Count(ctx, bob); n != 1 {
		t.Fatalf("Count after Delete = %d", n)
	}
	if profile, _ = music.Profile(ctx, bob); profile.Listening != uuid.Nil || profile.PageAudio != uuid.Nil {
		t.Fatalf("Profile after Delete = %+v", profile)
	}
	if err = audios.Delete(ctx, song.ID()); !errors.Is(err, post.ErrAudioNotFound) {
		t.Fatalf("Delete twice: %v", err)
	}
}

func TestPlaylistRepository(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool := newPool(t)
	audios := postgres.NewAudioRepository(pool)
	lists := audios.AudioLists()
	playlists := postgres.NewPlaylistRepository(pool)
	ann := uuid.New()

	song, err := post.NewAudioFile(uuid.Must(uuid.NewV7()), post.AudioDraft{OwnerID: ann}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = audios.Save(ctx, song); err != nil {
		t.Fatal(err)
	}

	first, err := post.NewPlaylist(ann, "В дорогу", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	second, err := post.NewPlaylist(ann, "Домой", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []*post.Playlist{first, second} {
		if err = playlists.Save(ctx, p); err != nil {
			t.Fatal(err)
		}
	}

	got, err := playlists.Get(ctx, first.ID())
	if err != nil || !reflect.DeepEqual(got, first) {
		t.Fatalf("Get = %+v, %v", got, err)
	}
	if all, _ := playlists.List(ctx, ann, uuid.Nil, 10); len(all) != 2 || all[0].ID() != second.ID() {
		t.Fatalf("List = %+v", all)
	}
	if n, _ := playlists.Count(ctx, ann); n != 2 {
		t.Fatalf("Count = %d", n)
	}

	renamed, err := first.Renamed(ann, "Лето")
	if err != nil {
		t.Fatal(err)
	}
	if err = playlists.Update(ctx, renamed); err != nil {
		t.Fatal(err)
	}

	if err = lists.Add(ctx, first.ID(), song.ID()); err != nil {
		t.Fatal(err)
	}
	if err = playlists.Delete(ctx, first.ID()); err != nil {
		t.Fatal(err)
	}
	if n, _ := lists.Count(ctx, first.ID()); n != 0 {
		t.Fatalf("the songs of a deleted playlist: %d", n)
	}
	if _, err = playlists.Get(ctx, first.ID()); !errors.Is(err, post.ErrPlaylistNotFound) {
		t.Fatalf("Get a deleted one: %v", err)
	}

	// The deletion of the page takes the uploads, the lists and the page.
	if err = lists.Add(ctx, ann, song.ID()); err != nil {
		t.Fatal(err)
	}
	if err = audios.DeleteByUser(ctx, ann); err != nil {
		t.Fatal(err)
	}
	if _, err = audios.Get(ctx, song.ID()); !errors.Is(err, post.ErrAudioNotFound) {
		t.Fatalf("an upload after DeleteByUser: %v", err)
	}
	if n, _ := playlists.Count(ctx, ann); n != 0 {
		t.Fatalf("playlists after DeleteByUser: %d", n)
	}
}

func TestPostAudios(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool := newPool(t)
	posts := postgres.NewPostRepository(pool)
	ann := uuid.New()

	songs := post.Audios{uuid.New(), uuid.New()}
	content, err := post.NewAttached("", nil, nil, songs)
	if err != nil {
		t.Fatal(err)
	}
	p, err := post.New(ann, ann, content, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = posts.Save(ctx, p); err != nil {
		t.Fatalf("Save a post of songs only: %v", err)
	}

	got, err := posts.Get(ctx, p.ID())
	if err != nil || !reflect.DeepEqual(got.Content().Audios(), songs) {
		t.Fatalf("Get = %v, %v", got.Content().Audios(), err)
	}
}
