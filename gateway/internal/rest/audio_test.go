package rest_test

import (
	"context"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/rest"
)

var audioTime = time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)

// fakeAudio knows a1, Ann's (u1) uploaded song, and a2, a track of
// SoundCloud Bob (u2) added first.
type fakeAudio struct {
	token, owner, cursor, query, kept, forgot, deleted, listening, pageAudio string
	added                                                                    model.NewAudio
	edited                                                                   string
	playlist                                                                 string
	err                                                                      error
}

func (f *fakeAudio) known() []model.Audio {
	return []model.Audio{
		{ID: "a1", OwnerID: "u1", Kind: model.AudioFile, Artist: "Кино", Title: "Группа крови", Duration: 285, CreatedAt: audioTime},
		{ID: "a2", OwnerID: "u2", Kind: model.AudioSoundCloud, Link: "https://soundcloud.com/a/b", Title: "Песня", CreatedAt: audioTime},
	}
}

func (f *fakeAudio) Add(_ context.Context, token string, na model.NewAudio) (model.Audio, error) {
	f.token, f.added = token, na
	if na.FileID != "" {
		return model.Audio{ID: na.FileID, OwnerID: "u1", Kind: model.AudioFile, Artist: na.Artist, Title: na.Title, Duration: na.Duration, CreatedAt: audioTime, Added: true}, f.err
	}
	return model.Audio{ID: "a7", OwnerID: "u1", Kind: na.Kind, Link: na.Link, Cover: na.Cover, Artist: na.Artist, Title: na.Title, CreatedAt: audioTime, Added: true}, f.err
}

func (f *fakeAudio) List(_ context.Context, token, ownerID, cursor string, _ int) (model.AudioPage, error) {
	f.token, f.owner, f.cursor = token, ownerID, cursor
	return model.AudioPage{Audios: f.known(), NextCursor: "a2", Total: 2}, f.err
}

func (f *fakeAudio) Search(_ context.Context, token, query, cursor string, _ int) (model.AudioPage, error) {
	f.token, f.query, f.cursor = token, query, cursor
	return model.AudioPage{Audios: f.known()[:1]}, f.err
}

// GetMany says that the one a token names keeps every song: that tells
// whose the asking was.
func (f *fakeAudio) GetMany(_ context.Context, token string, ids []string) ([]model.Audio, error) {
	var found []model.Audio
	for _, s := range f.known() {
		if slices.Contains(ids, s.ID) {
			s.Added = token != ""
			found = append(found, s)
		}
	}
	return found, f.err
}

func (f *fakeAudio) Keep(_ context.Context, token, id string) error {
	f.token, f.kept = token, id
	return f.err
}

func (f *fakeAudio) Forget(_ context.Context, token, id string) error {
	f.token, f.forgot = token, id
	return f.err
}

func (f *fakeAudio) Edit(_ context.Context, token, id, artist, title string) (model.Audio, error) {
	f.token, f.edited = token, id+"="+artist+" — "+title
	s := f.known()[0]
	s.Artist, s.Title = artist, title
	return s, f.err
}

func (f *fakeAudio) Delete(_ context.Context, token, id string) error {
	f.token, f.deleted = token, id
	return f.err
}

func (f *fakeAudio) Listen(_ context.Context, token, id string) error {
	f.token, f.listening = token, "listen:"+id
	return f.err
}

func (f *fakeAudio) SetPageAudio(_ context.Context, token, id string) error {
	f.token, f.pageAudio = token, "page:"+id
	return f.err
}

func (f *fakeAudio) Music(_ context.Context, userID string) (model.Music, error) {
	f.owner = userID
	s := f.known()[1]
	return model.Music{Listening: &s, Count: 2}, f.err
}

func (f *fakeAudio) CreatePlaylist(_ context.Context, token, title string) (model.Playlist, error) {
	f.token, f.playlist = token, "create:"+title
	return model.Playlist{ID: "pl1", OwnerID: "u1", Title: title, CreatedAt: audioTime}, f.err
}

func (f *fakeAudio) RenamePlaylist(_ context.Context, token, id, title string) (model.Playlist, error) {
	f.token, f.playlist = token, "rename:"+id+"="+title
	return model.Playlist{ID: id, OwnerID: "u1", Title: title, CreatedAt: audioTime}, f.err
}

func (f *fakeAudio) DeletePlaylist(_ context.Context, token, id string) error {
	f.token, f.playlist = token, "delete:"+id
	return f.err
}

func (f *fakeAudio) Playlists(_ context.Context, ownerID, cursor string, _ int) (model.PlaylistPage, error) {
	f.owner, f.cursor = ownerID, cursor
	return model.PlaylistPage{Playlists: []model.Playlist{{ID: "pl1", OwnerID: ownerID, Title: "Дорога", CreatedAt: audioTime}}, Total: 1}, f.err
}

func (f *fakeAudio) Playlist(_ context.Context, token, id, cursor string, _ int) (model.Playlist, model.AudioPage, error) {
	f.token, f.cursor = token, cursor
	return model.Playlist{ID: id, OwnerID: "u1", Title: "Дорога", CreatedAt: audioTime}, model.AudioPage{Audios: f.known()[:1], Total: 1}, f.err
}

func (f *fakeAudio) AddToPlaylist(_ context.Context, token, playlistID, audioID string) error {
	f.token, f.playlist = token, "add:"+playlistID+"+"+audioID
	return f.err
}

func (f *fakeAudio) RemoveFromPlaylist(_ context.Context, token, playlistID, audioID string) error {
	f.token, f.playlist = token, "remove:"+playlistID+"-"+audioID
	return f.err
}

// fakeTracks knows a playable track and one kept to its site.
type fakeTracks struct {
	asked string
}

func (f *fakeTracks) Track(_ context.Context, link string) (model.Track, error) {
	f.asked = link
	switch link {
	case "https://soundcloud.com/kino/song":
		return model.Track{Kind: model.AudioSoundCloud, Link: link, Title: "Группа крови", Artist: "Кино", Cover: "https://i1.sndcdn.com/a.jpg", Playable: true}, nil
	case "https://soundcloud.com/kino/locked":
		return model.Track{Kind: model.AudioSoundCloud, Link: link}, nil
	case "https://example.com/a":
		return model.Track{}, failure.New(failure.InvalidArgument, "this is not a link to a track")
	default:
		return model.Track{}, context.DeadlineExceeded
	}
}

type audioBody struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	URL      string `json:"url"`
	Link     string `json:"link"`
	Cover    string `json:"cover"`
	Artist   string `json:"artist"`
	Title    string `json:"title"`
	Duration int    `json:"duration"`
	Added    bool   `json:"added"`
}

func TestUploadAudio(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("POST", "/api/v1/me/audio?artist=%D0%9A%D0%B8%D0%BD%D0%BE&title=Song&duration=285", "ID3data", "Authorization", "Bearer tok")
	got := decode[audioBody](t, w)
	if w.Code != http.StatusCreated || string(h.media.uploaded) != "ID3data" {
		t.Fatalf("%d %+v", w.Code, got)
	}
	if h.audio.added != (model.NewAudio{FileID: "a9", Artist: "Кино", Title: "Song", Duration: 285}) {
		t.Fatalf("added = %+v", h.audio.added)
	}
	if got.URL != "/media/audio/a9" || got.Kind != "file" || !got.Added {
		t.Fatalf("song = %+v", got)
	}

	// The post service refuses it: media lets the file go.
	h = newHarness(rest.Config{})
	h.audio.err = failure.New(failure.InvalidArgument, "artist or title is too long")
	if w = h.do("POST", "/api/v1/me/audio", "ID3", "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest {
		t.Fatalf("refused: %d %s", w.Code, w.Body)
	}
	if !slices.Equal(h.media.droppedAudio, []string{"u1/a9"}) {
		t.Fatalf("dropped = %v", h.media.droppedAudio)
	}

	for name, path := range map[string]string{"empty": "/api/v1/me/audio", "a bad duration": "/api/v1/me/audio?duration=x"} {
		body := ""
		if name != "empty" {
			body = "ID3"
		}
		if w = newHarness(rest.Config{}).do("POST", path, body, "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", name, w.Code)
		}
	}
	if w = newHarness(rest.Config{}).do("POST", "/api/v1/me/audio", "ID3"); w.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", w.Code)
	}
}

func TestAddTrack(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("POST", "/api/v1/me/audio/links", `{"link":"https://soundcloud.com/kino/song"}`, "Authorization", "Bearer tok")
	got := decode[audioBody](t, w)
	if w.Code != http.StatusCreated || got.Link != "https://soundcloud.com/kino/song" || got.URL != "" || got.Cover != "https://i1.sndcdn.com/a.jpg" {
		t.Fatalf("%d %+v", w.Code, got)
	}
	if h.audio.added.Kind != model.AudioSoundCloud || h.audio.added.Title != "Группа крови" || h.audio.added.Artist != "Кино" {
		t.Fatalf("added = %+v", h.audio.added)
	}

	for link, code := range map[string]int{
		"https://soundcloud.com/kino/locked": http.StatusConflict,
		"https://example.com/a":              http.StatusBadRequest,
		"https://soundcloud.com/kino/slow":   http.StatusServiceUnavailable,
	} {
		h = newHarness(rest.Config{})
		if w = h.do("POST", "/api/v1/me/audio/links", `{"link":"`+link+`"}`, "Authorization", "Bearer tok"); w.Code != code || h.audio.added.Kind != "" {
			t.Errorf("%s: %d %s", link, w.Code, w.Body)
		}
	}
}

func TestAudioLibrary(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("GET", "/api/v1/users/ann/audio?cursor=a5", "")
	got := decode[struct {
		Audios     []audioBody `json:"audios"`
		NextCursor string      `json:"nextCursor"`
		Total      int         `json:"total"`
	}](t, w)
	if w.Code != http.StatusOK || len(got.Audios) != 2 || got.Total != 2 || h.audio.owner != "u1" || h.audio.cursor != "a5" {
		t.Fatalf("%d %+v", w.Code, got)
	}
	if w = h.do("GET", "/api/v1/users/cat/audio", ""); w.Code != http.StatusForbidden {
		t.Fatalf("a deleted page: %d", w.Code)
	}

	if w = h.do("GET", "/api/v1/audio?q=кино", ""); w.Code != http.StatusOK || h.audio.query != "кино" {
		t.Fatalf("search: %d %q", w.Code, h.audio.query)
	}

	for _, tt := range []struct {
		method, path, body string
		check              func() bool
	}{
		{"PUT", "/api/v1/me/audio/a2", "", func() bool { return h.audio.kept == "a2" }},
		{"DELETE", "/api/v1/me/audio/a2", "", func() bool { return h.audio.forgot == "a2" }},
		{"DELETE", "/api/v1/audio/a1", "", func() bool { return h.audio.deleted == "a1" }},
		{"PUT", "/api/v1/me/listening", `{"audio":"a2"}`, func() bool { return h.audio.listening == "listen:a2" }},
		{"DELETE", "/api/v1/me/listening", "", func() bool { return h.audio.listening == "listen:" }},
		{"PUT", "/api/v1/me/page-audio", `{"audio":"a1"}`, func() bool { return h.audio.pageAudio == "page:a1" }},
		{"DELETE", "/api/v1/me/page-audio", "", func() bool { return h.audio.pageAudio == "page:" }},
		{"PUT", "/api/v1/playlists/pl1/audio/a2", "", func() bool { return h.audio.playlist == "add:pl1+a2" }},
		{"DELETE", "/api/v1/playlists/pl1/audio/a2", "", func() bool { return h.audio.playlist == "remove:pl1-a2" }},
		{"DELETE", "/api/v1/playlists/pl1", "", func() bool { return h.audio.playlist == "delete:pl1" }},
	} {
		if w = h.do(tt.method, tt.path, tt.body, "Authorization", "Bearer tok"); w.Code != http.StatusNoContent || !tt.check() || h.audio.token != "tok" {
			t.Errorf("%s %s: %d %s", tt.method, tt.path, w.Code, w.Body)
		}
		if w = h.do(tt.method, tt.path, tt.body); w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s anonymous: %d", tt.method, tt.path, w.Code)
		}
	}
	if w = h.do("PUT", "/api/v1/me/listening", `{}`, "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest {
		t.Errorf("listening to none: %d", w.Code)
	}

	edited := decode[audioBody](t, h.do("PATCH", "/api/v1/audio/a1", `{"artist":"Кино","title":"Звезда"}`, "Authorization", "Bearer tok"))
	if h.audio.edited != "a1=Кино — Звезда" || edited.Title != "Звезда" {
		t.Fatalf("edited %q %+v", h.audio.edited, edited)
	}

	music := decode[struct {
		Listening *audioBody `json:"listening"`
		PageAudio *audioBody `json:"pageAudio"`
		Count     int        `json:"count"`
	}](t, h.do("GET", "/api/v1/users/ann/music", ""))
	if music.Listening == nil || music.Listening.ID != "a2" || music.PageAudio != nil || music.Count != 2 {
		t.Fatalf("music = %+v", music)
	}
}

func TestPlaylists(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("POST", "/api/v1/me/playlists", `{"title":"Дорога"}`, "Authorization", "Bearer tok")
	if w.Code != http.StatusCreated || h.audio.playlist != "create:Дорога" {
		t.Fatalf("create: %d %s", w.Code, w.Body)
	}
	if w = h.do("PATCH", "/api/v1/playlists/pl1", `{"title":"Лето"}`, "Authorization", "Bearer tok"); w.Code != http.StatusOK || h.audio.playlist != "rename:pl1=Лето" {
		t.Fatalf("rename: %d %s", w.Code, w.Body)
	}

	list := decode[struct {
		Playlists []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"playlists"`
		Total int `json:"total"`
	}](t, h.do("GET", "/api/v1/users/ann/playlists", ""))
	if len(list.Playlists) != 1 || list.Total != 1 || h.audio.owner != "u1" {
		t.Fatalf("list = %+v", list)
	}

	got := decode[struct {
		ID     string      `json:"id"`
		Title  string      `json:"title"`
		Audios []audioBody `json:"audios"`
		Total  int         `json:"total"`
	}](t, h.do("GET", "/api/v1/playlists/pl1", ""))
	if got.ID != "pl1" || got.Title != "Дорога" || len(got.Audios) != 1 || got.Total != 1 {
		t.Fatalf("playlist = %+v", got)
	}
}

func TestServeAudio(t *testing.T) {
	t.Parallel()

	h := newHarness(rest.Config{})
	w := h.do("GET", "/media/audio/a1", "")
	if w.Code != http.StatusOK || w.Body.String() != "ID3-song-bytes" || w.Header().Get("Content-Type") != "audio/mpeg" || w.Header().Get("Accept-Ranges") != "bytes" {
		t.Fatalf("%d %s %v", w.Code, w.Body, w.Header())
	}

	if w = h.do("GET", "/media/audio/a1", "", "Range", "bytes=0-2"); w.Code != http.StatusPartialContent || w.Body.String() != "ID3" {
		t.Fatalf("range: %d %s", w.Code, w.Body)
	}

	// A track of another site has no file here.
	for _, id := range []string{"a2", "nope"} {
		if w = h.do("GET", "/media/audio/"+id, ""); w.Code != http.StatusNotFound {
			t.Errorf("%s: %d", id, w.Code)
		}
	}
}

func TestAttachAudios(t *testing.T) {
	t.Parallel()

	filled := func() harness {
		h := newHarness(rest.Config{})
		h.users.profile = model.Profile{Name: "Ann", Nickname: "ann"}
		return h
	}

	h := filled()
	w := h.do("POST", "/api/v1/me/posts", `{"audios":["a2","a1"]}`, "Authorization", "Bearer tok")
	got := decode[struct {
		Audios []audioBody `json:"audios"`
	}](t, w)
	if w.Code != http.StatusCreated || !slices.Equal(h.posts.newPost.AudioIDs, []string{"a2", "a1"}) {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if len(got.Audios) != 2 || got.Audios[0].ID != "a2" || got.Audios[1].URL != "/media/audio/a1" {
		t.Fatalf("audios = %+v", got.Audios)
	}

	// Anyone's song, but one that is there.
	h = filled()
	if w = h.do("POST", "/api/v1/me/posts", `{"text":"x","audios":["nope"]}`, "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest || h.posts.created != "" {
		t.Fatalf("none: %d", w.Code)
	}
	h = filled()
	if w = h.do("PATCH", "/api/v1/posts/p1", `{"text":"x","audios":["a1"]}`, "Authorization", "Bearer tok"); w.Code != http.StatusBadRequest {
		t.Fatalf("by an edit: %d", w.Code)
	}

	h = filled()
	w = h.do("POST", "/api/v1/dialogs/bob/messages", `{"audios":["a2"]}`, "Authorization", "Bearer tok")
	msg := decode[map[string]any](t, w)
	if songs, _ := msg["audios"].([]any); w.Code != http.StatusCreated || len(songs) != 1 || msg["audioCount"] != float64(1) {
		t.Fatalf("message: %d %v", w.Code, msg)
	}

	h = filled()
	w = h.do("POST", "/api/v1/chats/c1/messages", `{"audios":["a1"]}`, "Authorization", "Bearer tok")
	if msg = decode[map[string]any](t, w); w.Code != http.StatusCreated || !slices.Equal(h.chats.draft.AudioIDs, []string{"a1"}) {
		t.Fatalf("chat message: %d %v", w.Code, msg)
	}
}
