package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/postgres"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestBlockRepository(t *testing.T) {
	repo, pool := newRepo(t)
	follows := postgres.NewFollowRepository(pool)
	blocks := postgres.NewBlockRepository(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	newUser := func() uuid.UUID {
		t.Helper()

		u, err := user.New(user.Email(unique("bl") + "@example.com"))
		if err != nil {
			t.Fatal(err)
		}
		if err = repo.Save(ctx, u); err != nil {
			t.Fatal(err)
		}

		return u.ID()
	}

	ann, bob, cat := newUser(), newUser(), newUser()

	for _, e := range []service.Edge{{Follower: ann, Followee: bob}, {Follower: bob, Followee: ann}, {Follower: cat, Followee: ann}} {
		if err := follows.Follow(ctx, e.Follower, e.Followee); err != nil {
			t.Fatal(err)
		}
	}

	dropped, err := blocks.Block(ctx, ann, bob)
	if err != nil || len(dropped) != 2 {
		t.Fatalf("Block = %v, %v; want both follows dropped", dropped, err)
	}
	if followers, following, _ := follows.Counts(ctx, ann); followers != 1 || following != 0 {
		t.Fatalf("ann's follows = %d, %d; want cat's only", followers, following)
	}

	if dropped, err = blocks.Block(ctx, ann, bob); err != nil || len(dropped) != 0 {
		t.Fatalf("Block twice = %v, %v", dropped, err)
	}

	if byAnn, byBob, err := blocks.Blocks(ctx, ann, bob); err != nil || !byAnn || byBob {
		t.Fatalf("Blocks(ann, bob) = %v, %v, %v", byAnn, byBob, err)
	}
	if byBob, byAnn, _ := blocks.Blocks(ctx, bob, ann); byBob || !byAnn {
		t.Fatalf("Blocks(bob, ann) = %v, %v", byBob, byAnn)
	}

	if _, err = blocks.Block(ctx, ann, cat); err != nil {
		t.Fatal(err)
	}

	page, err := blocks.ListBlocked(ctx, ann, uuid.Nil, 1)
	if err != nil || len(page) != 1 || page[0].User.ID() != cat {
		t.Fatalf("first page = %v, %v; want cat, the newest", page, err)
	}
	rest, err := blocks.ListBlocked(ctx, ann, page[0].BlockID, 10)
	if err != nil || len(rest) != 1 || rest[0].User.ID() != bob {
		t.Fatalf("after cursor = %v, %v", rest, err)
	}
	if ids, err := blocks.BlockedIDs(ctx, ann, 10); err != nil || len(ids) != 2 || ids[0] != cat || ids[1] != bob {
		t.Fatalf("BlockedIDs = %v, %v; want cat, bob", ids, err)
	}
	if ids, err := blocks.BlockedIDs(ctx, ann, 1); err != nil || len(ids) != 1 || ids[0] != cat {
		t.Fatalf("BlockedIDs, one = %v, %v", ids, err)
	}
	if n, err := blocks.CountBlocked(ctx, ann); err != nil || n != 2 {
		t.Fatalf("CountBlocked = %d, %v", n, err)
	}

	if err = blocks.Unblock(ctx, ann, bob); err != nil {
		t.Fatal(err)
	}
	if byAnn, _, _ := blocks.Blocks(ctx, ann, bob); byAnn {
		t.Fatal("still blocked after Unblock")
	}

	if _, err = blocks.Block(ctx, ann, uuid.New()); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("block a missing user: %v, want ErrNotFound", err)
	}
}
