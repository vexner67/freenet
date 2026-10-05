package postgres_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/postgres"
)

func TestLikesAndComments(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool := newPool(t)
	posts := postgres.NewPostRepository(pool)
	likes := postgres.NewLikeRepository(pool)
	comments := postgres.NewCommentRepository(pool)

	ann, bob := uuid.New(), uuid.New()

	p, err := post.New(ann, "привет", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = posts.Save(ctx, p); err != nil {
		t.Fatal(err)
	}

	t.Run("likes", func(t *testing.T) {
		if n, err := likes.Like(ctx, p.ID(), ann); err != nil || n != 1 {
			t.Fatalf("Like = %d, %v", n, err)
		}
		if n, err := likes.Like(ctx, p.ID(), ann); err != nil || n != 1 {
			t.Fatalf("second Like = %d, %v; a like counts once", n, err)
		}

		liked, err := likes.Liked(ctx, ann, []uuid.UUID{p.ID(), uuid.New()})
		if err != nil || len(liked) != 1 || !liked[p.ID()] {
			t.Fatalf("Liked = %v, %v", liked, err)
		}

		if n, err := likes.Unlike(ctx, p.ID(), ann); err != nil || n != 0 {
			t.Fatalf("Unlike = %d, %v", n, err)
		}
		if n, err := likes.Unlike(ctx, p.ID(), ann); err != nil || n != 0 {
			t.Fatalf("second Unlike = %d, %v", n, err)
		}

		if _, err := likes.Like(ctx, uuid.New(), ann); !errors.Is(err, post.ErrNotFound) {
			t.Fatalf("Like a missing post: %v", err)
		}
		if _, err := likes.Unlike(ctx, uuid.New(), ann); !errors.Is(err, post.ErrNotFound) {
			t.Fatalf("Unlike a missing post: %v", err)
		}
	})

	// The counter changes in the same statement as the like, so concurrent
	// likes neither get lost nor count twice.
	t.Run("concurrent likes", func(t *testing.T) {
		const users = 20

		var wg sync.WaitGroup
		for range users {
			user := uuid.New()
			for range 2 {
				wg.Go(func() {
					if _, err := likes.Like(ctx, p.ID(), user); err != nil {
						t.Error(err)
					}
				})
			}
		}
		wg.Wait()

		got, err := posts.Get(ctx, p.ID())
		if err != nil || got.Counters().Likes != users {
			t.Fatalf("likes = %v, %v; want %d", got.Counters(), err, users)
		}
	})

	t.Run("comments", func(t *testing.T) {
		var saved []*post.Comment
		for _, text := range []string{"раз", "два", "три"} {
			c, err := post.NewComment(p.ID(), bob, post.CommentText(text), time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if err = comments.Save(ctx, c); err != nil {
				t.Fatalf("Save: %v", err)
			}
			saved = append(saved, c)
		}

		got, err := comments.Get(ctx, saved[0].ID())
		if err != nil || *got != *saved[0] {
			t.Fatalf("round trip: got %+v, %v; want %+v", got, err, saved[0])
		}

		many, err := comments.GetMany(ctx, []uuid.UUID{saved[1].ID(), uuid.New()})
		if err != nil || len(many) != 1 || *many[0] != *saved[1] {
			t.Fatalf("GetMany = %v, %v", many, err)
		}

		first, err := comments.ListByPost(ctx, p.ID(), uuid.Nil, 2)
		if err != nil || len(first) != 2 || first[0].Text() != "раз" || first[1].Text() != "два" {
			t.Fatalf("first page: %v, %v", first, err)
		}

		rest, err := comments.ListByPost(ctx, p.ID(), first[1].ID(), 2)
		if err != nil || len(rest) != 1 || rest[0].Text() != "три" {
			t.Fatalf("after cursor: %v, %v", rest, err)
		}

		if counted, _ := posts.Get(ctx, p.ID()); counted.Counters().Comments != 3 {
			t.Fatalf("comment count = %d, want 3", counted.Counters().Comments)
		}

		if err = comments.Delete(ctx, saved[0].ID()); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if err = comments.Delete(ctx, saved[0].ID()); !errors.Is(err, post.ErrCommentNotFound) {
			t.Fatalf("second Delete: %v", err)
		}
		if _, err = comments.Get(ctx, saved[0].ID()); !errors.Is(err, post.ErrCommentNotFound) {
			t.Fatalf("Get after Delete: %v", err)
		}

		if counted, _ := posts.Get(ctx, p.ID()); counted.Counters().Comments != 2 {
			t.Fatalf("comment count = %d, want 2", counted.Counters().Comments)
		}

		orphan, err := post.NewComment(uuid.New(), bob, "куда?", time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err = comments.Save(ctx, orphan); !errors.Is(err, post.ErrNotFound) {
			t.Fatalf("Save on a missing post: %v", err)
		}
	})

	t.Run("deleting the post takes likes and comments", func(t *testing.T) {
		left, err := comments.ListByPost(ctx, p.ID(), uuid.Nil, 10)
		if err != nil || len(left) == 0 {
			t.Fatalf("comments before: %v, %v", left, err)
		}

		if err = posts.Delete(ctx, p.ID()); err != nil {
			t.Fatal(err)
		}

		if _, err = comments.Get(ctx, left[0].ID()); !errors.Is(err, post.ErrCommentNotFound) {
			t.Fatalf("comment after the post is deleted: %v", err)
		}

		liked, err := likes.Liked(ctx, ann, []uuid.UUID{p.ID()})
		if err != nil || len(liked) != 0 {
			t.Fatalf("likes after the post is deleted: %v, %v", liked, err)
		}
	})
}
