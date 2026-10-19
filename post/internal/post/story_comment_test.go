package post_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/post"
)

func TestStoryComment(t *testing.T) {
	t.Parallel()

	ann, bob, eve := uuid.New(), uuid.New(), uuid.New()
	now := time.Now()
	story, err := post.NewStory(ann, uuid.New(), "", now)
	if err != nil {
		t.Fatal(err)
	}

	if _, err = post.NewStoryComment(story.ID(), bob, "", now); !errors.Is(err, post.ErrCommentEmpty) {
		t.Fatalf("an empty comment: %v", err)
	}

	long := post.CommentText(strings.Repeat("я", post.MaxStoryCommentLength+1))
	if _, err = post.NewStoryComment(story.ID(), bob, long, now); !errors.Is(err, post.ErrStoryCommentTooLong) {
		t.Fatalf("a long comment: %v", err)
	}
	if _, err = post.NewStoryComment(story.ID(), bob, long[:len(long)-len("я")], now); err != nil {
		t.Fatalf("a comment of the longest: %v", err)
	}

	c, err := post.NewStoryComment(story.ID(), bob, "красиво", now)
	if err != nil || c.StoryID() != story.ID() || c.AuthorID() != bob || c.Text() != "красиво" {
		t.Fatalf("NewStoryComment = %+v, %v", c, err)
	}

	for _, tt := range []struct {
		name string
		who  uuid.UUID
		want error
	}{
		{"its author", bob, nil},
		{"the author of the story", ann, nil},
		{"another", eve, post.ErrCannotDeleteStoryComment},
	} {
		if err := c.CheckDelete(tt.who, story); !errors.Is(err, tt.want) {
			t.Errorf("%s: CheckDelete = %v, want %v", tt.name, err, tt.want)
		}
	}

	if story.Notifies(ann) || !story.Notifies(bob) {
		t.Fatal("the author hears of their own, or not of another's")
	}
}
