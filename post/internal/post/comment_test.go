package post_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/post"
)

func TestNewCommentText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  post.CommentText
		err   error
	}{
		{"plain", "согласен", "согласен", nil},
		{"trimmed, windows line breaks", "  раз\r\nдва \n", "раз\nдва", nil},
		{"emoji count as one character", strings.Repeat("👍", 1000), post.CommentText(strings.Repeat("👍", 1000)), nil},
		{"empty", "", "", post.ErrCommentEmpty},
		{"only spaces and breaks", " \n\t\r\n ", "", post.ErrCommentEmpty},
		{"too long", strings.Repeat("я", 1001), "", post.ErrCommentTooLong},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := post.NewCommentText(tt.input)
			if !errors.Is(err, tt.err) || got != tt.want {
				t.Fatalf("NewCommentText = %q, %v; want %q, %v", got, err, tt.want, tt.err)
			}
		})
	}
}

func TestComment(t *testing.T) {
	t.Parallel()

	wallOwner, postAuthor, commenter, stranger := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	now := time.Date(2026, 10, 5, 12, 0, 0, 123456789, time.FixedZone("MSK", 3*3600))

	p, err := post.New(wallOwner, postAuthor, textOnly(t, "привет"), now)
	if err != nil {
		t.Fatal(err)
	}

	c, err := post.NewComment(p.ID(), commenter, "и тебе", now)
	if err != nil {
		t.Fatal(err)
	}

	if c.PostID() != p.ID() || c.AuthorID() != commenter || c.Text() != "и тебе" {
		t.Fatalf("comment = %v %v %q", c.PostID(), c.AuthorID(), c.Text())
	}
	if c.CreatedAt().Location() != time.UTC || c.CreatedAt().Nanosecond() != 123456000 {
		t.Fatalf("createdAt = %v, want UTC with microseconds", c.CreatedAt())
	}

	if !c.CanBeDeletedBy(commenter, p) || !c.CanBeDeletedBy(postAuthor, p) || !c.CanBeDeletedBy(wallOwner, p) {
		t.Fatal("the commenter, the author of the post and the owner of the wall may delete")
	}
	if c.CanBeDeletedBy(stranger, p) {
		t.Fatal("a stranger may not delete")
	}

	later, err := post.NewComment(p.ID(), commenter, "ещё", now)
	if err != nil {
		t.Fatal(err)
	}
	if later.ID().String() <= c.ID().String() {
		t.Fatal("IDs must grow, comments are ordered by them")
	}
}
