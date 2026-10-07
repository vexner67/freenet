package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/inmem"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestPermissionService_CanPostOnWall(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	users := inmem.NewUserRepository()
	follows := inmem.NewFollowRepository(users)

	userSvc := service.NewUserService(users)
	follow := service.NewFollowService(service.FollowDeps{
		Users: users, Follows: follows, Lists: follows, Blocks: inmem.NewBlockRepository(follows), Tx: inmem.TxManager{}, Notifier: &fakeNotifier{},
	})
	svc := service.NewPermissionService(users, follows, inmem.NewBlockRepository(follows))

	ann, err := userSvc.GetOrCreateByEmail(ctx, mustEmail(t, "ann@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	bob, err := userSvc.GetOrCreateByEmail(ctx, mustEmail(t, "bob@example.com"))
	if err != nil {
		t.Fatal(err)
	}

	canPost := func(author, owner uuid.UUID) bool {
		t.Helper()

		ok, err := svc.CanPostOnWall(ctx, author, owner)
		if err != nil {
			t.Fatalf("CanPostOnWall: %v", err)
		}

		return ok
	}

	if !canPost(ann, ann) {
		t.Fatal("ann may post on her own wall")
	}
	if canPost(bob, ann) {
		t.Fatal("a new wall is open only to the people its owner follows")
	}

	// Bob following Ann does not open her wall; Ann following Bob does.
	if err = follow.Follow(ctx, bob, ann); err != nil {
		t.Fatal(err)
	}
	if canPost(bob, ann) {
		t.Fatal("following the owner must not open the wall")
	}
	if err = follow.Follow(ctx, ann, bob); err != nil {
		t.Fatal(err)
	}
	if !canPost(bob, ann) {
		t.Fatal("ann follows bob, so he may post on her wall")
	}

	if _, err = userSvc.UpdatePrivacy(ctx, ann, user.Privacy{WallWrite: user.WallWriteOnlyMe, MessagesWrite: user.MessagesWriteEveryone}); err != nil {
		t.Fatal(err)
	}
	if canPost(bob, ann) {
		t.Fatal(`"only me" closes the wall to everyone else`)
	}

	if _, err = svc.CanPostOnWall(ctx, bob, uuid.New()); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("unknown owner: %v, want ErrNotFound", err)
	}
}
