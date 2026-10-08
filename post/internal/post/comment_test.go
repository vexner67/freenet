package post_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/post"
)

func TestNewCommentContent(t *testing.T) {
	t.Parallel()

	photo := uuid.New()

	if _, err := post.NewCommentContent("", nil); !errors.Is(err, post.ErrCommentEmpty) {
		t.Fatalf("neither text nor photos: %v", err)
	}

	c, err := post.NewCommentContent("", post.Photos{photo})
	if err != nil || c.Text() != "" || len(c.Photos()) != 1 {
		t.Fatalf("photos only = %+v, %v", c, err)
	}

	p, err := post.New(uuid.New(), uuid.New(), textOnly(t, "запись"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	comment, err := post.NewComment(p.ID(), uuid.New(), c, time.Now())
	if err != nil || len(comment.Photos()) != 1 || comment.Photos()[0] != photo {
		t.Fatalf("comment photos = %v, %v", comment.Photos(), err)
	}
}

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
		{"empty is fine, photos may say it all", "", "", nil},
		{"only spaces and breaks", " \n\t\r\n ", "", nil},
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

	c, err := post.NewComment(p.ID(), commenter, post.RestoreCommentContent("и тебе", nil), now)
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

	later, err := post.NewComment(p.ID(), commenter, post.RestoreCommentContent("ещё", nil), now)
	if err != nil {
		t.Fatal(err)
	}
	if later.ID().String() <= c.ID().String() {
		t.Fatal("IDs must grow, comments are ordered by them")
	}
}

func TestNewReply(t *testing.T) {
	t.Parallel()

	postID, other := uuid.New(), uuid.New()
	content := post.RestoreCommentContent("согласен", nil)

	parent, err := post.NewComment(postID, uuid.New(), content, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	reply, err := post.NewReply(postID, parent, uuid.New(), content, time.Now())
	if err != nil || reply.ReplyTo() != parent.ID() || reply.PostID() != postID || reply.Likes() != 0 {
		t.Fatalf("NewReply = %+v, %v", reply, err)
	}
	if parent.ReplyTo() != uuid.Nil {
		t.Fatal("a comment answers nothing")
	}

	if _, err = post.NewReply(other, parent, uuid.New(), content, time.Now()); !errors.Is(err, post.ErrReplyOtherPost) {
		t.Fatalf("a reply under another post: %v", err)
	}
}

func TestComment_ForWall(t *testing.T) {
	t.Parallel()

	club, admin := uuid.New(), uuid.New()
	p, err := post.New(club, admin, post.RestoreContent("новости", nil, uuid.Nil), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	c, err := post.NewComment(p.ID(), admin, post.RestoreCommentContent("ответ", nil), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if c.FromWall() || c.Notified() != admin {
		t.Fatalf("a comment = %+v", c)
	}

	forClub, err := c.ForWall(p)
	if err != nil || !forClub.FromWall() || forClub.Notified() != uuid.Nil || forClub.AuthorID() != admin || c.FromWall() {
		t.Fatalf("ForWall = %+v, %v", forClub, err)
	}

	own, _ := post.New(admin, admin, post.RestoreContent("моя", nil, uuid.Nil), time.Now())
	if _, err = c.ForWall(own); !errors.Is(err, post.ErrFromOwnWall) {
		t.Fatalf("for one's own wall: %v", err)
	}
}
