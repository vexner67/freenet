package postgres_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/postgres"
)

func TestCommentRepository_Photos(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool := newPool(t)
	posts := postgres.NewPostRepository(pool)
	comments := postgres.NewCommentRepository(pool)
	ann, bob := uuid.New(), uuid.New()
	photo := uuid.New()

	save := func(wall, author uuid.UUID) *post.Post {
		t.Helper()

		p, err := post.New(wall, author, post.RestoreContent("запись", nil, uuid.Nil), time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err = posts.Save(ctx, p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	comment := func(p *post.Post, author uuid.UUID, content post.CommentContent) *post.Comment {
		t.Helper()

		c, err := post.NewComment(p.ID(), author, content, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err = comments.Save(ctx, c); err != nil {
			t.Fatalf("Save: %v", err)
		}
		return c
	}

	annsPost, onAnnsWall, bobsPost := save(ann, ann), save(ann, bob), save(bob, bob)

	photoOnly := comment(annsPost, bob, post.RestoreCommentContent("", post.Photos{photo}))
	got, err := comments.Get(ctx, photoOnly.ID())
	if err != nil || got.Text() != "" || !slices.Equal(got.Photos(), post.Photos{photo}) {
		t.Fatalf("a comment of photos only = %+v, %v", got, err)
	}
	if plain, _ := comments.Get(ctx, comment(annsPost, ann, post.RestoreCommentContent("текст", nil)).ID()); plain.Photos() != nil {
		t.Fatalf("a comment without photos reads back %v", plain.Photos())
	}

	onWall := comment(onAnnsWall, bob, post.RestoreCommentContent("у Анны", nil))
	onBobs := comment(bobsPost, ann, post.RestoreCommentContent("у Пети", nil))

	// The comments of a post to delete, inside a transaction, as the
	// service runs it; rolled back, they stay for the purge below.
	rollback := errors.New("rollback")
	err = pgpool.NewTxManager(pool).WithinTx(ctx, func(ctx context.Context) error {
		deleted, err := comments.DeleteByPost(ctx, annsPost.ID())
		if err != nil || len(deleted) != 2 {
			t.Errorf("DeleteByPost = %d comments, %v", len(deleted), err)
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}

	// Purging ann: the comments on her posts and her wall, whoever wrote
	// them; hers on bob's post stay for DeleteByUser.
	onHers, err := comments.DeleteOnPostsOf(ctx, ann)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]uuid.UUID, len(onHers))
	for i, c := range onHers {
		ids[i] = c.ID()
	}
	if len(ids) != 3 || !slices.Contains(ids, photoOnly.ID()) || !slices.Contains(ids, onWall.ID()) {
		t.Fatalf("DeleteOnPostsOf = %v", ids)
	}
	if _, err = comments.Get(ctx, onBobs.ID()); err != nil {
		t.Fatalf("ann's comment on bob's post: %v", err)
	}
}
