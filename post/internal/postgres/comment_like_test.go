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

func TestCommentRepository_RepliesAndLikes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool := newPool(t)
	posts := postgres.NewPostRepository(pool)
	comments := postgres.NewCommentRepository(pool)
	likes := postgres.NewCommentLikeRepository(pool)
	ann, bob := uuid.New(), uuid.New()
	content := post.RestoreCommentContent("текст", nil)

	p, err := post.New(ann, ann, post.RestoreContent("запись", nil, uuid.Nil), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = posts.Save(ctx, p); err != nil {
		t.Fatal(err)
	}

	parent, err := post.NewComment(p.ID(), ann, content, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = comments.Save(ctx, parent); err != nil {
		t.Fatal(err)
	}
	reply, err := post.NewReply(p.ID(), parent, bob, content, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = comments.Save(ctx, reply); err != nil {
		t.Fatalf("Save reply: %v", err)
	}
	if got, err := comments.Get(ctx, reply.ID()); err != nil || got.ReplyTo() != parent.ID() {
		t.Fatalf("reply reads back answering %v, %v", got.ReplyTo(), err)
	}

	// An answer to a comment that is gone.
	gone, err := post.NewComment(p.ID(), ann, content, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	orphan, err := post.NewReply(p.ID(), gone, bob, content, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = comments.Save(ctx, orphan); !errors.Is(err, post.ErrCommentNotFound) {
		t.Fatalf("Save of an answer to nothing: %v", err)
	}

	for _, who := range []uuid.UUID{ann, bob, bob} { // bob twice: counted once
		if _, err = likes.Like(ctx, parent.ID(), who); err != nil {
			t.Fatalf("Like: %v", err)
		}
	}
	if got, _ := comments.Get(ctx, parent.ID()); got.Likes() != 2 {
		t.Fatalf("likes = %d, want 2", got.Likes())
	}
	liked, err := likes.Liked(ctx, bob, []uuid.UUID{parent.ID(), reply.ID()})
	if err != nil || !liked[parent.ID()] || liked[reply.ID()] {
		t.Fatalf("bob likes %v, %v", liked, err)
	}
	if n, err := likes.Unlike(ctx, parent.ID(), ann); err != nil || n != 1 {
		t.Fatalf("Unlike = %d, %v", n, err)
	}
	if _, err = likes.Like(ctx, uuid.New(), ann); !errors.Is(err, post.ErrCommentNotFound) {
		t.Fatalf("like a missing comment: %v", err)
	}

	// Erasing bob uncounts his likes.
	if err = likes.DeleteByUser(ctx, bob); err != nil {
		t.Fatal(err)
	}
	if got, _ := comments.Get(ctx, parent.ID()); got.Likes() != 0 {
		t.Fatalf("likes after erasing bob = %d", got.Likes())
	}

	// The comment answered goes: the reply stays, answering nothing.
	if err = comments.Delete(ctx, parent.ID()); err != nil {
		t.Fatal(err)
	}
	if got, err := comments.Get(ctx, reply.ID()); err != nil || got.ReplyTo() != uuid.Nil {
		t.Fatalf("the reply after its comment went: answers %v, %v", got.ReplyTo(), err)
	}
}
