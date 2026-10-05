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
