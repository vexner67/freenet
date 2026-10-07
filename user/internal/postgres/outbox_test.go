package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/outbox"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/pkg/reqid"
	"github.com/vexner67/freenet/user/internal/notifyclient"
	"github.com/vexner67/freenet/user/internal/postgres"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

// failingRecorder is an outbox that cannot record.
type failingRecorder struct{}

func (failingRecorder) Add(context.Context, string, []byte) error {
	return errors.New("outbox is down")
}

// A follow, a block and their notifications are saved in one transaction:
// all or none.
func TestFollowsAndTheirNotifications(t *testing.T) {
	repo, pool := newRepo(t)
	follows := postgres.NewFollowRepository(pool)
	blocks := postgres.NewBlockRepository(pool)
	tx := pgpool.NewTxManager(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	newUser := func() uuid.UUID {
		t.Helper()

		u, err := user.New(user.Email(unique("ob") + "@example.com"))
		if err != nil {
			t.Fatal(err)
		}
		if err = repo.Save(ctx, u); err != nil {
			t.Fatal(err)
		}

		return u.ID()
	}

	services := func(r notifyclient.Recorder) (*service.FollowService, *service.BlockService) {
		n := notifyclient.NewOutbox(r)
		return service.NewFollowService(service.FollowDeps{
				Users: repo, Follows: follows, Lists: follows, Blocks: blocks, Tx: tx, Notifier: n,
			}),
			service.NewBlockService(repo, blocks, tx, n)
	}

	recorded := func(requestID string) int {
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM outbox WHERE request_id = $1`, requestID).Scan(&n); err != nil {
			t.Fatalf("count outbox: %v", err)
		}
		return n
	}

	ann, bob := newUser(), newUser()
	reqCtx := reqid.ContextWithID(ctx, uuid.NewString())

	followSvc, blockSvc := services(outbox.NewPostgres(pool, notifyclient.Stream))
	if err := followSvc.Follow(reqCtx, bob, ann); err != nil {
		t.Fatalf("Follow: %v", err)
	}

	failingFollows, failingBlocks := services(failingRecorder{})

	if err := failingBlocks.Block(ctx, ann, bob); err == nil {
		t.Fatal("Block with a broken outbox = nil, want an error")
	}
	if byAnn, _, _ := blocks.Blocks(ctx, ann, bob); byAnn {
		t.Fatal("the block stayed although the retraction of the follow it dropped was not recorded")
	}
	if following, _ := follows.IsFollowing(ctx, bob, ann); !following {
		t.Fatal("the follow is gone although the block failed")
	}

	if err := failingFollows.Unfollow(ctx, bob, ann); err == nil {
		t.Fatal("Unfollow with a broken outbox = nil, want an error")
	}
	if following, _ := follows.IsFollowing(ctx, bob, ann); !following {
		t.Fatal("the unfollow stayed although its retraction was not recorded")
	}

	if err := blockSvc.Block(reqCtx, ann, bob); err != nil {
		t.Fatalf("Block: %v", err)
	}
	if got := recorded(reqid.FromContext(reqCtx)); got != 2 {
		t.Errorf("recorded %d messages, want the follow and its retraction by the block", got)
	}
}
