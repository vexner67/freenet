package post_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/vexner67/freenet/post/internal/post"
)

func TestNewStoryText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want post.StoryText
		err  error
	}{
		{name: "empty", in: "  ", want: ""},
		{name: "trimmed", in: "  Утро в лесу \r\n", want: "Утро в лесу"},
		{name: "longest", in: strings.Repeat("я", 200), want: post.StoryText(strings.Repeat("я", 200))},
		{name: "too long", in: strings.Repeat("я", 201), err: post.ErrStoryTextTooLong},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := post.NewStoryText(tt.in)
			if got != tt.want || !errors.Is(err, tt.err) {
				t.Errorf("NewStoryText() = %q, %v, want %q, %v", got, err, tt.want, tt.err)
			}
		})
	}
}

func TestNewStory(t *testing.T) {
	t.Parallel()

	ann, bob, photo := uuid.New(), uuid.New(), uuid.New()
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)

	if _, err := post.NewStory(ann, uuid.Nil, "", now); !errors.Is(err, post.ErrStoryWithoutPhoto) {
		t.Fatalf("a story without a photo: %v", err)
	}

	s, err := post.NewStory(ann, photo, "Утро", now)
	if err != nil {
		t.Fatal(err)
	}

	if s.ID() == uuid.Nil || s.AuthorID() != ann || s.PhotoID() != photo || s.Text() != "Утро" || !s.CreatedAt().Equal(now) {
		t.Fatalf("the story = %+v", s)
	}
	if !s.ExpiresAt().Equal(now.Add(24*time.Hour)) || !s.Live(now.Add(24*time.Hour-time.Second)) || s.Live(now.Add(24*time.Hour)) {
		t.Fatalf("a story lives a day: expires at %v", s.ExpiresAt())
	}
	if !post.LiveSince(now.Add(24 * time.Hour)).Equal(now) {
		t.Fatal("LiveSince is a day before")
	}

	if err = s.CheckDelete(bob); !errors.Is(err, post.ErrCannotDeleteStory) || s.CheckDelete(ann) != nil {
		t.Fatalf("CheckDelete: %v", err)
	}
	if err = s.CheckViewers(bob); !errors.Is(err, post.ErrNotStoryAuthor) || s.CheckViewers(ann) != nil {
		t.Fatalf("CheckViewers: %v", err)
	}
	if s.CountsView(ann) || s.CountsView(uuid.Nil) || !s.CountsView(bob) {
		t.Fatal("only the views of others are counted")
	}
}
