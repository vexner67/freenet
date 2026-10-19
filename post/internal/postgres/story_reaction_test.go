package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/postgres"
)

func TestStoryReactions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool := newPool(t)
	stories := postgres.NewStoryRepository(pool)
	likes, comments := postgres.NewStoryLikeRepository(pool), postgres.NewStoryCommentRepository(pool)
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()
	now := time.Now().UTC().Truncate(time.Microsecond)

	story, err := post.NewStory(ann, uuid.New(), "", now)
	if err != nil {
		t.Fatal(err)
	}
	if err = stories.Save(ctx, story); err != nil {
		t.Fatal(err)
	}

	for _, userID := range []uuid.UUID{bob, cat} {
		if added, err := likes.Like(ctx, story.ID(), userID, now); err != nil || !added {
			t.Fatalf("Like = %v, %v", added, err)
		}
	}
	if added, err := likes.Like(ctx, story.ID(), bob, now); err != nil || added {
		t.Fatalf("Like twice = %v, %v", added, err)
	}
	if _, err = likes.Like(ctx, uuid.New(), bob, now); !errors.Is(err, post.ErrStoryNotFound) {
		t.Fatalf("Like of no story: %v", err)
	}
	if liked, err := likes.Liked(ctx, bob, []uuid.UUID{story.ID()}); err != nil || !liked[story.ID()] {
		t.Fatalf("Liked = %v, %v", liked, err)
	}

	write := func(authorID uuid.UUID, text post.CommentText) *post.StoryComment {
		t.Helper()
		c, err := post.NewStoryComment(story.ID(), authorID, text, now)
		if err != nil {
			t.Fatal(err)
		}
		if err = comments.Save(ctx, c); err != nil {
			t.Fatalf("Save: %v", err)
		}
		return c
	}

	byBob, byCat := write(bob, "Красиво"), write(cat, "Где это?")
	if got, err := comments.Get(ctx, byBob.ID()); err != nil || got.Text() != "Красиво" || got.AuthorID() != bob || !got.CreatedAt().Equal(now) {
		t.Fatalf("Get = %+v, %v", got, err)
	}
	if c, _ := post.NewStoryComment(uuid.New(), bob, "x", now); !errors.Is(comments.Save(ctx, c), post.ErrStoryNotFound) {
		t.Fatal("a comment of no story is saved")
	}
	if got, err := comments.List(ctx, story.ID(), byBob.ID(), 10); err != nil || len(got) != 1 || got[0].ID() != byCat.ID() {
		t.Fatalf("List after bob's = %v, %v", got, err)
	}

	// A deleted page's likes and comments are not shown or counted.
	if err = postgres.NewHoldRepository(pool).Hold(ctx, cat); err != nil {
		t.Fatal(err)
	}
	if counts, err := likes.Count(ctx, []uuid.UUID{story.ID()}); err != nil || counts[story.ID()] != 1 {
		t.Fatalf("likes Count = %v, %v", counts, err)
	}
	if counts, err := comments.Count(ctx, []uuid.UUID{story.ID()}); err != nil || counts[story.ID()] != 1 {
		t.Fatalf("comments Count = %v, %v", counts, err)
	}
	if got, _ := comments.List(ctx, story.ID(), uuid.Nil, 10); len(got) != 1 || got[0].ID() != byBob.ID() {
		t.Fatalf("List without cat = %v", got)
	}

	if removed, err := likes.Unlike(ctx, story.ID(), bob); err != nil || !removed {
		t.Fatalf("Unlike = %v, %v", removed, err)
	}
	if err = comments.Delete(ctx, byBob.ID()); err != nil {
		t.Fatal(err)
	}
	if err = comments.Delete(ctx, byBob.ID()); !errors.Is(err, post.ErrStoryCommentNotFound) {
		t.Fatalf("Delete twice: %v", err)
	}

	// The likes and comments of a person go with them, and the rest with
	// the story.
	if err = stories.DeleteByUser(ctx, cat); err != nil {
		t.Fatal(err)
	}
	if _, err = comments.Get(ctx, byCat.ID()); !errors.Is(err, post.ErrStoryCommentNotFound) {
		t.Fatalf("cat's comment stays: %v", err)
	}
	write(bob, "Ещё")
	if _, err = likes.Like(ctx, story.ID(), bob, now); err != nil {
		t.Fatal(err)
	}
	if err = stories.Delete(ctx, story.ID()); err != nil {
		t.Fatal(err)
	}
	if got, _ := comments.List(ctx, story.ID(), uuid.Nil, 10); len(got) != 0 {
		t.Fatalf("comments outlive the story: %v", got)
	}
	if liked, _ := likes.Liked(ctx, bob, []uuid.UUID{story.ID()}); liked[story.ID()] {
		t.Fatal("a like outlives the story")
	}
}
