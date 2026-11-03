package oembed_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/gateway/internal/oembed"
)

func TestKind(t *testing.T) {
	t.Parallel()

	for link, want := range map[string]string{
		"https://soundcloud.com/a/b":                  model.AudioSoundCloud,
		"https://m.soundcloud.com/a/b":                model.AudioSoundCloud,
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ": model.AudioYouTube,
		"https://youtu.be/dQw4w9WgXcQ":                model.AudioYouTube,
		"https://example.com/a":                       "",
		"javascript:alert(1)":                         "",
	} {
		if got := oembed.Kind(link); got != want {
			t.Errorf("Kind(%q) = %q, want %q", link, got, want)
		}
	}
}

func TestTrack(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("url") {
		case "https://soundcloud.com/kino/song":
			_, _ = w.Write([]byte(`{"title":"Группа крови by Кино","author_name":"Кино","thumbnail_url":"https://i1.sndcdn.com/a.jpg"}`))
		case "https://soundcloud.com/kino/locked":
			w.WriteHeader(http.StatusForbidden)
		case "https://www.youtube.com/watch?v=dQw4w9WgXcQ":
			_, _ = w.Write([]byte(`{"title":"Never Gonna Give You Up","author_name":"Rick Astley","thumbnail_url":"http://i.ytimg.com/a.jpg"}`))
		case "https://www.youtube.com/watch?v=bbbbbbbbbbb":
			_, _ = w.Write([]byte(`{"title":"Кино - Группа крови (Official Video)","author_name":"Music Channel"}`))
		case "https://www.youtube.com/watch?v=aaaaaaaaaaa":
			w.WriteHeader(http.StatusUnauthorized)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	r := oembed.New(time.Second, oembed.Endpoints{SoundCloud: srv.URL + "/sc", YouTube: srv.URL + "/yt"})
	ctx := context.Background()

	track, err := r.Track(ctx, " https://soundcloud.com/kino/song ")
	if err != nil || !track.Playable || track.Title != "Группа крови" || track.Artist != "Кино" || track.Cover != "https://i1.sndcdn.com/a.jpg" || track.Kind != model.AudioSoundCloud {
		t.Fatalf("Track = %+v, %v", track, err)
	}

	// A cover over http is not shown on an https page.
	if track, err = r.Track(ctx, "https://www.youtube.com/watch?v=dQw4w9WgXcQ"); err != nil || track.Cover != "" || track.Title != "Never Gonna Give You Up" {
		t.Fatalf("Track of YouTube = %+v, %v", track, err)
	}

	if track, err = r.Track(ctx, "https://www.youtube.com/watch?v=bbbbbbbbbbb"); err != nil || track.Artist != "Кино" || track.Title != "Группа крови (Official Video)" {
		t.Fatalf("Track of a song on YouTube = %+v, %v", track, err)
	}

	for _, link := range []string{"https://soundcloud.com/kino/locked", "https://www.youtube.com/watch?v=aaaaaaaaaaa"} {
		if track, err = r.Track(ctx, link); err != nil || track.Playable {
			t.Errorf("Track of %s, kept to its site = %+v, %v", link, track, err)
		}
	}

	if _, err = r.Track(ctx, "https://soundcloud.com/kino/none"); !errors.Is(err, oembed.ErrNoTrack) {
		t.Errorf("Track of none: %v", err)
	}
	if _, err = r.Track(ctx, "https://example.com/a"); !errors.Is(err, oembed.ErrNotTrack) {
		t.Errorf("Track of another site: %v", err)
	}
}
