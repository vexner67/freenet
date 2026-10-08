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

// world is three users with all the services over shared repositories.
type world struct {
	ctx           context.Context
	users         *service.UserService
	follows       *service.FollowService
	blocks        *service.BlockService
	permissions   *service.PermissionService
	notifier      *fakeNotifier
	ann, bob, cat uuid.UUID
}

func newWorld(t *testing.T) world {
	t.Helper()

	users := inmem.NewUserRepository()
	followRepo := inmem.NewFollowRepository(users)
	blockRepo := inmem.NewBlockRepository(followRepo)
	notifier := &fakeNotifier{}

	w := world{
		ctx:   context.Background(),
		users: service.NewUserService(users),
		follows: service.NewFollowService(service.FollowDeps{
			Users: users, Follows: followRepo, Lists: followRepo, Blocks: blockRepo, Tx: inmem.TxManager{}, Notifier: notifier,
		}),
		blocks:      service.NewBlockService(users, blockRepo, inmem.TxManager{}, notifier),
		permissions: service.NewPermissionService(users, followRepo, blockRepo),
		notifier:    notifier,
	}

	for _, id := range []*uuid.UUID{&w.ann, &w.bob, &w.cat} {
		var err error
		if *id, err = w.users.GetOrCreateByEmail(w.ctx, mustEmail(t, uuid.NewString()+"@example.com")); err != nil {
			t.Fatal(err)
		}
	}

	return w
}

func TestBlockService_Block(t *testing.T) {
	t.Parallel()

	w := newWorld(t)

	for _, f := range []follow{{w.ann, w.bob}, {w.bob, w.ann}, {w.cat, w.ann}} {
		if err := w.follows.Follow(w.ctx, f.follower, f.followee); err != nil {
			t.Fatal(err)
		}
	}

	if err := w.blocks.Block(w.ctx, w.ann, w.bob); err != nil {
		t.Fatalf("Block: %v", err)
	}
	if err := w.blocks.Block(w.ctx, w.ann, w.bob); err != nil {
		t.Fatalf("Block twice: %v", err)
	}

	stats, err := w.follows.Stats(w.ctx, w.ann, uuid.Nil)
	if err != nil || stats.Followers != 1 || stats.Following != 0 {
		t.Fatalf("ann's follows after blocking bob = %+v, %v; want only cat left", stats, err)
	}
	if n := len(w.notifier.unfollowed); n != 2 {
		t.Fatalf("unfollowed = %+v, want both follows taken back", w.notifier.unfollowed)
	}

	// Neither may follow the other now.
	for _, f := range []follow{{w.bob, w.ann}, {w.ann, w.bob}} {
		if err = w.follows.Follow(w.ctx, f.follower, f.followee); !errors.Is(err, user.ErrFollowBlocked) {
			t.Fatalf("follow across a block: %v, want ErrFollowBlocked", err)
		}
	}

	rel, err := w.blocks.Relation(w.ctx, w.ann, w.bob)
	if err != nil || !rel.BlockedByViewer || rel.BlockedViewer {
		t.Fatalf("ann sees %+v, %v", rel, err)
	}
	if rel, _ = w.blocks.Relation(w.ctx, w.bob, w.ann); rel.BlockedByViewer || !rel.BlockedViewer {
		t.Fatalf("bob sees %+v", rel)
	}

	if err = w.blocks.Unblock(w.ctx, w.ann, w.bob); err != nil {
		t.Fatal(err)
	}
	if err = w.follows.Follow(w.ctx, w.bob, w.ann); err != nil {
		t.Fatalf("follow after unblocking: %v", err)
	}

	if err = w.blocks.Block(w.ctx, w.ann, w.ann); !errors.Is(err, user.ErrBlockSelf) {
		t.Fatalf("block yourself: %v", err)
	}
	if err = w.blocks.Block(w.ctx, w.ann, uuid.New()); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("block nobody: %v", err)
	}
}

func TestBlockService_List(t *testing.T) {
	t.Parallel()

	w := newWorld(t)

	for _, id := range []uuid.UUID{w.bob, w.cat} {
		if err := w.blocks.Block(w.ctx, w.ann, id); err != nil {
			t.Fatal(err)
		}
	}

	page, err := w.blocks.List(w.ctx, w.ann, uuid.Nil, 1)
	if err != nil || page.Total != 2 || len(page.Users) != 1 || page.Users[0].ID != w.cat || page.NextCursor == uuid.Nil {
		t.Fatalf("first page = %+v, %v; want cat, the newest", page, err)
	}

	last, err := w.blocks.List(w.ctx, w.ann, page.NextCursor, 1)
	if err != nil || len(last.Users) != 1 || last.Users[0].ID != w.bob || last.NextCursor != uuid.Nil {
		t.Fatalf("last page = %+v, %v", last, err)
	}

	if mine, _ := w.blocks.List(w.ctx, w.bob, uuid.Nil, 0); mine.Total != 0 {
		t.Fatalf("bob's blacklist = %+v, want empty", mine)
	}
}

func TestPermissionService_CanMessage(t *testing.T) {
	t.Parallel()

	w := newWorld(t)

	canMessage := func(sender, recipient uuid.UUID, wroteFirst bool) bool {
		t.Helper()

		ok, err := w.permissions.CanMessage(w.ctx, sender, recipient, wroteFirst)
		if err != nil {
			t.Fatalf("CanMessage: %v", err)
		}

		return ok
	}

	if !canMessage(w.bob, w.ann, false) {
		t.Fatal("messages are open by default")
	}

	if _, err := w.users.UpdatePrivacy(w.ctx, w.ann, user.Privacy{
		WallWrite: user.WallWriteFollowing, MessagesWrite: user.MessagesWriteFollowing, ClubsRead: user.ClubsReadEveryone,
	}); err != nil {
		t.Fatal(err)
	}
	if canMessage(w.bob, w.ann, false) {
		t.Fatal("ann takes messages from the people she follows only")
	}
	if !canMessage(w.bob, w.ann, true) {
		t.Fatal("ann wrote to bob first: he may answer")
	}
	if err := w.follows.Follow(w.ctx, w.ann, w.bob); err != nil {
		t.Fatal(err)
	}
	if !canMessage(w.bob, w.ann, false) {
		t.Fatal("ann follows bob")
	}

	// A block either way shuts messages, answers too, and the wall.
	if err := w.blocks.Block(w.ctx, w.bob, w.ann); err != nil {
		t.Fatal(err)
	}
	if canMessage(w.bob, w.ann, true) || canMessage(w.ann, w.bob, true) {
		t.Fatal("a block shuts messages both ways")
	}
	if ok, _ := w.permissions.CanPostOnWall(w.ctx, w.ann, w.bob); ok {
		t.Fatal("a block shuts the wall")
	}

	if _, err := w.permissions.CanMessage(w.ctx, w.bob, uuid.New(), false); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("unknown recipient: %v", err)
	}
}
