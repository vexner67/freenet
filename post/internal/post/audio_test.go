package post_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/post"
)

func TestNewAudioName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want post.AudioName
		err  error
	}{
		{name: "trimmed to one line", in: "  Кино \n Группа  крови ", want: "Кино Группа крови"},
		{name: "empty is unknown", in: "  ", want: ""},
		{name: "at the limit", in: strings.Repeat("я", 128), want: post.AudioName(strings.Repeat("я", 128))},
		{name: "too long", in: strings.Repeat("я", 129), err: post.ErrAudioNameTooLong},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := post.NewAudioName(tt.in)
			if !errors.Is(err, tt.err) || got != tt.want {
				t.Fatalf("NewAudioName = %q, %v; want %q, %v", got, err, tt.want, tt.err)
			}
		})
	}
}

func TestNewAudioDuration(t *testing.T) {
	t.Parallel()

	for _, seconds := range []int{0, 1, post.MaxAudioSeconds} {
		if _, err := post.NewAudioDuration(seconds); err != nil {
			t.Fatalf("%d: %v", seconds, err)
		}
	}

	for _, seconds := range []int{-1, post.MaxAudioSeconds + 1} {
		if _, err := post.NewAudioDuration(seconds); !errors.Is(err, post.ErrInvalidDuration) {
			t.Fatalf("%d: %v", seconds, err)
		}
	}
}

func TestNewAudioSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		kind post.AudioKind
		link string
		want string
		err  error
	}{
		{name: "soundcloud", kind: post.AudioSoundCloud, link: "https://soundcloud.com/kino/gruppa-krovi?si=abc", want: "https://soundcloud.com/kino/gruppa-krovi"},
		{name: "soundcloud on the phone", kind: post.AudioSoundCloud, link: "https://m.soundcloud.com/kino/gruppa-krovi/", want: "https://soundcloud.com/kino/gruppa-krovi"},
		{name: "a private soundcloud track", kind: post.AudioSoundCloud, link: "https://soundcloud.com/kino/demo/s-AbC12", want: "https://soundcloud.com/kino/demo/s-AbC12"},
		{name: "an artist is not a track", kind: post.AudioSoundCloud, link: "https://soundcloud.com/kino", err: post.ErrAudioLink},
		{name: "another site as soundcloud", kind: post.AudioSoundCloud, link: "https://evil.com/kino/gruppa-krovi", err: post.ErrAudioLink},
		{name: "youtube", kind: post.AudioYouTube, link: "https://www.youtube.com/watch?v=dQw4w9WgXcQ&t=10", want: "https://www.youtube.com/watch?v=dQw4w9WgXcQ"},
		{name: "youtu.be", kind: post.AudioYouTube, link: "https://youtu.be/dQw4w9WgXcQ?si=x", want: "https://www.youtube.com/watch?v=dQw4w9WgXcQ"},
		{name: "youtube music", kind: post.AudioYouTube, link: "https://music.youtube.com/watch?v=dQw4w9WgXcQ", want: "https://www.youtube.com/watch?v=dQw4w9WgXcQ"},
		{name: "shorts", kind: post.AudioYouTube, link: "https://youtube.com/shorts/dQw4w9WgXcQ", want: "https://www.youtube.com/watch?v=dQw4w9WgXcQ"},
		{name: "a bad video id", kind: post.AudioYouTube, link: "https://youtu.be/short", err: post.ErrAudioLink},
		{name: "a script", kind: post.AudioYouTube, link: "javascript:alert(1)", err: post.ErrAudioLink},
		{name: "a file is not a link", kind: post.AudioFile, link: "https://soundcloud.com/a/b", err: post.ErrAudioLink},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := post.NewAudioSource(tt.kind, tt.link, "")
			if !errors.Is(err, tt.err) || got.Link() != tt.want {
				t.Fatalf("NewAudioSource = %q, %v; want %q, %v", got.Link(), err, tt.want, tt.err)
			}
		})
	}

	if _, err := post.NewAudioSource(post.AudioYouTube, "https://youtu.be/dQw4w9WgXcQ", "http://i.ytimg.com/a.jpg"); !errors.Is(err, post.ErrAudioCover) {
		t.Fatalf("a cover over http: %v", err)
	}
}

func TestAudioEdited(t *testing.T) {
	t.Parallel()

	ann, bob := uuid.New(), uuid.New()
	song, err := post.NewAudioFile(uuid.New(), post.AudioDraft{OwnerID: ann, Artist: "Кино", Title: "Звезда"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	edited, err := song.Edited(ann, "Кино", "Звезда по имени Солнце")
	if err != nil || edited.Title() != "Звезда по имени Солнце" {
		t.Fatalf("Edited = %v, %v", edited, err)
	}
	if _, err = song.Edited(bob, "", ""); !errors.Is(err, post.ErrCannotChangeAudio) {
		t.Fatalf("by another: %v", err)
	}

	source, err := post.NewAudioSource(post.AudioYouTube, "https://youtu.be/dQw4w9WgXcQ", "")
	if err != nil {
		t.Fatal(err)
	}
	track, err := post.NewAudioTrack(uuid.New(), post.AudioDraft{OwnerID: ann}, source, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = track.Edited(ann, "", ""); !errors.Is(err, post.ErrCannotChangeAudio) {
		t.Fatalf("a shared track: %v", err)
	}
}

func TestNewAudios(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	if _, err := post.NewAudios([]uuid.UUID{id, id}); !errors.Is(err, post.ErrDuplicateAudio) {
		t.Fatalf("twice: %v", err)
	}
	if _, err := post.NewAudios(make([]uuid.UUID, 11)); !errors.Is(err, post.ErrTooManyAudios) {
		t.Fatalf("eleven: %v", err)
	}
	if _, err := post.NewAudios([]uuid.UUID{uuid.Nil}); !errors.Is(err, post.ErrInvalidAudio) {
		t.Fatalf("nil: %v", err)
	}
}

func TestAudioIsContentEnough(t *testing.T) {
	t.Parallel()

	content, err := post.NewAttached("", nil, nil, post.Audios{uuid.New()})
	if err != nil || len(content.Audios()) != 1 {
		t.Fatalf("NewAttached = %v, %v", content, err)
	}
	if _, err = post.NewAttached("", nil, nil, nil); !errors.Is(err, post.ErrTextEmpty) {
		t.Fatalf("nothing: %v", err)
	}
}

func TestPlaylist(t *testing.T) {
	t.Parallel()

	if _, err := post.NewPlaylistTitle("  "); !errors.Is(err, post.ErrPlaylistTitleEmpty) {
		t.Fatalf("empty: %v", err)
	}
	if _, err := post.NewPlaylistTitle(strings.Repeat("я", 129)); !errors.Is(err, post.ErrPlaylistTitleTooLong) {
		t.Fatalf("too long: %v", err)
	}

	ann := uuid.New()
	p, err := post.NewPlaylist(ann, "В дорогу", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Renamed(uuid.New(), "Чужой"); !errors.Is(err, post.ErrCannotChangePlaylist) {
		t.Fatalf("by another: %v", err)
	}
	if renamed, err := p.Renamed(ann, "Домой"); err != nil || renamed.Title() != "Домой" {
		t.Fatalf("Renamed = %v, %v", renamed, err)
	}
}
