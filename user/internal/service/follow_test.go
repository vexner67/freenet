package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/inmem"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

type follow struct{ follower, followee uuid.UUID }

// fakeNotifier records the follows told to the notification service, and
// fails with err when set.
type fakeNotifier struct {
	followed   []follow
	unfollowed []follow
	invited    []follow
	err        error
}

func (f *fakeNotifier) Invited(_ context.Context, invitee, inviter uuid.UUID) error {
	if f.err != nil {
		return f.err
	}
	f.invited = append(f.invited, follow{invitee, inviter})
	return nil
}

func (f *fakeNotifier) Followed(_ context.Context, follower, followee uuid.UUID) error {
	if f.err != nil {
		return f.err
	}
	f.followed = append(f.followed, follow{follower, followee})
	return nil
}

func (f *fakeNotifier) Unfollowed(_ context.Context, follower, followee uuid.UUID) error {
	if f.err != nil {
		return f.err
	}
	f.unfollowed = append(f.unfollowed, follow{follower, followee})
	return nil
}

func TestFollowService_Notifies(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	users := inmem.NewUserRepository()
	follows := inmem.NewFollowRepository(users)
	notifier := &fakeNotifier{}

	userSvc := service.NewUserService(users)
	svc := service.NewFollowService(service.FollowDeps{
		Users: users, Follows: follows, Lists: follows, Blocks: inmem.NewBlockRepository(follows), Tx: inmem.TxManager{}, Notifier: notifier,
	})

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

	// A notification that cannot be recorded fails the follow: the two
	// are saved together (in one transaction) or not at all.
	notifier.err = errors.New("outbox is down")
	if err = svc.Follow(ctx, bob, ann); !errors.Is(err, notifier.err) {
		t.Fatalf("follow with a broken outbox: %v, want its error", err)
	}
	if err = svc.Unfollow(ctx, bob, ann); !errors.Is(err, notifier.err) {
		t.Fatalf("unfollow with a broken outbox: %v, want its error", err)
	}
}

func TestFollowService_AcceptInvite(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	users := inmem.NewUserRepository()
	follows := inmem.NewFollowRepository(users)
	blocks := inmem.NewBlockRepository(follows)
	notifier := &fakeNotifier{}
	now := time.Now()

	userSvc := service.NewUserService(users)
	svc := service.NewFollowService(service.FollowDeps{
		Users: users, Follows: follows, Lists: follows, Blocks: blocks, Invites: users, Tx: inmem.TxManager{}, Notifier: notifier,
		Now: func() time.Time { return now },
	})

	signUp := func(email string) uuid.UUID {
		t.Helper()

		id, err := userSvc.GetOrCreateByEmail(ctx, mustEmail(t, email))
		if err != nil {
			t.Fatal(err)
		}

		return id
	}

	ann, bob, cat := signUp("ann@example.com"), signUp("bob@example.com"), signUp("cat@example.com")

	if err := svc.AcceptInvite(ctx, bob, bob); !errors.Is(err, user.ErrFollowSelf) {
		t.Fatalf("invited by oneself: %v", err)
	}
	if err := svc.AcceptInvite(ctx, bob, uuid.New()); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("invited by nobody: %v", err)
	}

	if _, err := blocks.Block(ctx, ann, cat); err != nil {
		t.Fatal(err)
	}
	if err := svc.AcceptInvite(ctx, cat, ann); !errors.Is(err, user.ErrFollowBlocked) {
		t.Fatalf("invited by one who blacklisted: %v", err)
	}

	if err := svc.AcceptInvite(ctx, bob, ann); err != nil {
		t.Fatalf("AcceptInvite: %v", err)
	}

	following, err := follows.IsFollowing(ctx, bob, ann)
	if err != nil || !following {
		t.Fatalf("the invited follows the inviter: %v, %v", following, err)
	}

	want := follow{follower: bob, followee: ann}
	if len(notifier.invited) != 1 || notifier.invited[0] != want || len(notifier.followed) != 0 {
		t.Fatalf("invited = %+v, followed = %+v, want one invitation %+v", notifier.invited, notifier.followed, want)
	}

	// Once: the next link names nobody new.
	if err := svc.AcceptInvite(ctx, bob, cat); !errors.Is(err, user.ErrAlreadyInvited) {
		t.Fatalf("invited twice: %v", err)
	}

	// An old page names nobody.
	now = now.Add(user.InviteFor + time.Hour)
	if err := svc.AcceptInvite(ctx, cat, bob); !errors.Is(err, user.ErrInviteExpired) {
		t.Fatalf("invited when old: %v", err)
	}

	if len(notifier.invited) != 1 {
		t.Fatalf("notified about failed invitations: %+v", notifier.invited)
	}
}
