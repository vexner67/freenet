package postgres_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/postgres"
)

// Erasing a user deletes their posts, those on their wall, their likes and
// comments, and keeps the counters of the posts that stay right.
func TestDeleteByUser(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool := newPool(t)
	posts := postgres.NewPostRepository(pool)
	likes := postgres.NewLikeRepository(pool)
	comments := postgres.NewCommentRepository(pool)

	ann, bob := uuid.New(), uuid.New()

	save := func(wall, author uuid.UUID, content post.Content) *post.Post {
		t.Helper()

		p, err := post.New(wall, author, content, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err = posts.Save(ctx, p); err != nil {
			t.Fatal(err)
		}
		return p
	}

	bobsPost := save(bob, bob, post.RestoreContent("запись Петра", nil, uuid.Nil))
	annsPost := save(ann, ann, post.RestoreContent("запись Анны", nil, uuid.Nil))
	onAnnsWall := save(ann, bob, post.RestoreContent("Пётр у Анны", nil, uuid.Nil))
	annsRepost := save(ann, ann, post.RestoreContent("", nil, bobsPost.ID()))
	bobsRepostOfAnn := save(bob, bob, post.RestoreContent("", nil, annsPost.ID()))

	for _, who := range []uuid.UUID{ann, bob} {
		if _, err := likes.Like(ctx, bobsPost.ID(), who); err != nil {
			t.Fatal(err)
		}
	}
	c, err := post.NewComment(bobsPost.ID(), ann, "привет", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = comments.Save(ctx, c); err != nil {
		t.Fatal(err)
	}

	if err = likes.DeleteByUser(ctx, ann); err != nil {
		t.Fatalf("likes.DeleteByUser: %v", err)
	}
	if err = comments.DeleteByUser(ctx, ann); err != nil {
		t.Fatalf("comments.DeleteByUser: %v", err)
	}
	deleted, err := posts.DeleteByUser(ctx, ann)
	if err != nil {
		t.Fatalf("posts.DeleteByUser: %v", err)
	}

	want := []uuid.UUID{annsPost.ID(), onAnnsWall.ID(), annsRepost.ID()}
	if len(deleted) != len(want) {
		t.Fatalf("deleted %v, want %v", deleted, want)
	}
	for _, id := range want {
		if !slices.Contains(deleted, id) {
			t.Errorf("%s was not deleted", id)
		}
	}

	kept, err := posts.Get(ctx, bobsPost.ID())
	if err != nil {
		t.Fatal(err)
	}
	if got := kept.Counters(); got.Likes != 1 || got.Comments != 0 || got.Reposts != 0 {
		t.Errorf("bob's post counters = %+v, want bob's like only", got)
	}

	// Bob's repost of ann's post outlives it, as any repost does.
	if _, err = posts.Get(ctx, bobsRepostOfAnn.ID()); err != nil {
		t.Errorf("bob's repost of ann's post: %v", err)
	}
	if _, err = posts.Get(ctx, annsPost.ID()); !errors.Is(err, post.ErrNotFound) {
		t.Errorf("ann's post: %v", err)
	}

	if again, err := posts.DeleteByUser(ctx, ann); err != nil || len(again) != 0 {
		t.Errorf("second DeleteByUser = %v, %v", again, err)
	}
}
