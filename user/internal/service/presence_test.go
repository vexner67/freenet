package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/inmem"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestPresenceService(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	users := inmem.NewUserRepository()
	follows := inmem.NewFollowRepository(users)
	blocks := inmem.NewBlockRepository(follows)

	userSvc := service.NewUserService(users)
	presence := service.NewPresenceService(users)
	followSvc := service.NewFollowService(service.FollowDeps{
		Users: users, Follows: follows, Lists: follows, Blocks: blocks, Tx: inmem.TxManager{}, Notifier: &fakeNotifier{},
	})

	var ids []uuid.UUID
	for _, nick := range []string{"ann", "bob", "cat"} {
		id, err := userSvc.GetOrCreateByEmail(ctx, mustEmail(t, nick+"@example.com"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = userSvc.UpdateProfile(ctx, id, user.Profile{Name: "N", Nickname: user.Nickname(nick)}, ""); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	ann, bob, cat := ids[0], ids[1], ids[2]

	for _, followee := range []uuid.UUID{bob, cat} {
		if err := followSvc.Follow(ctx, ann, followee); err != nil {
			t.Fatal(err)
		}
	}

	if err := presence.MarkOnline(ctx, bob); err != nil {
		t.Fatalf("MarkOnline: %v", err)
	}

	page, err := followSvc.ListOnlineFollowing(ctx, ann, uuid.Nil, 0)
	if err != nil || page.Total != 1 || len(page.Users) != 1 || page.Users[0].ID != bob {
		t.Fatalf("ListOnlineFollowing = %+v, %v", page, err)
	}

	// The page of someone online says so; of someone never seen, nothing.
	perms := service.NewPermissionService(users, follows, blocks)
	if p, err := perms.ProfileFor(ctx, uuid.Nil, "bob"); err != nil || !p.Online || p.LastSeen.IsZero() {
		t.Fatalf("bob: %+v, %v", p, err)
	}
	if p, err := perms.ProfileFor(ctx, uuid.Nil, "cat"); err != nil || p.Online || !p.LastSeen.IsZero() {
		t.Fatalf("cat: %+v, %v", p, err)
	}
}
