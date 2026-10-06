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

type follow struct{ follower, followee uuid.UUID }

// fakeNotifier records the follows told to the notification service.
type fakeNotifier struct {
	followed   []follow
	unfollowed []follow
}

func (f *fakeNotifier) Followed(_ context.Context, follower, followee uuid.UUID) {
	f.followed = append(f.followed, follow{follower, followee})
}

func (f *fakeNotifier) Unfollowed(_ context.Context, follower, followee uuid.UUID) {
	f.unfollowed = append(f.unfollowed, follow{follower, followee})
}

func TestFollowService_Notifies(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	users := inmem.NewUserRepository()
	follows := inmem.NewFollowRepository(users)
	notifier := &fakeNotifier{}

	userSvc := service.NewUserService(users)
	svc := service.NewFollowService(users, follows, follows, notifier)

	ann, err := userSvc.GetOrCreateByEmail(ctx, mustEmail(t, "ann@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	bob, err := userSvc.GetOrCreateByEmail(ctx, mustEmail(t, "bob@example.com"))
	if err != nil {
		t.Fatal(err)
	}

	if err = svc.Follow(ctx, bob, ann); err != nil {
		t.Fatal(err)
	}
	if err = svc.Unfollow(ctx, bob, ann); err != nil {
		t.Fatal(err)
	}

	want := follow{follower: bob, followee: ann}
	if len(notifier.followed) != 1 || notifier.followed[0] != want {
		t.Fatalf("followed = %+v, want %+v", notifier.followed, want)
	}
	if len(notifier.unfollowed) != 1 || notifier.unfollowed[0] != want {
		t.Fatalf("unfollowed = %+v, want %+v", notifier.unfollowed, want)
	}

	// Failed follows notify nobody.
	if err = svc.Follow(ctx, ann, ann); !errors.Is(err, user.ErrFollowSelf) {
		t.Fatalf("follow yourself: %v", err)
	}
	if err = svc.Follow(ctx, ann, uuid.New()); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("follow nobody: %v", err)
	}
	if err = svc.Unfollow(ctx, ann, ann); err != nil {
		t.Fatalf("unfollow yourself: %v", err)
	}
	if len(notifier.followed) != 1 || len(notifier.unfollowed) != 1 {
		t.Fatalf("notified about failed follows: %+v %+v", notifier.followed, notifier.unfollowed)
	}
}

func TestFollowService_CanPostOnWall(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	users := inmem.NewUserRepository()
	follows := inmem.NewFollowRepository(users)

	userSvc := service.NewUserService(users)
	svc := service.NewFollowService(users, follows, follows, &fakeNotifier{})

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
	if err = svc.Follow(ctx, bob, ann); err != nil {
		t.Fatal(err)
	}
	if canPost(bob, ann) {
		t.Fatal("following the owner must not open the wall")
	}
	if err = svc.Follow(ctx, ann, bob); err != nil {
		t.Fatal(err)
	}
	if !canPost(bob, ann) {
		t.Fatal("ann follows bob, so he may post on her wall")
	}

	if _, err = userSvc.UpdatePrivacy(ctx, ann, user.Privacy{WallWrite: user.WallWriteOnlyMe}); err != nil {
		t.Fatal(err)
	}
	if canPost(bob, ann) {
		t.Fatal(`"only me" closes the wall to everyone else`)
	}

	if _, err = svc.CanPostOnWall(ctx, bob, uuid.New()); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("unknown owner: %v, want ErrNotFound", err)
	}
}
