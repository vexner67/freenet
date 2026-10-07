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
	"github.com/vexner67/freenet/post/internal/notifyclient"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/postgres"
	"github.com/vexner67/freenet/post/internal/service"
)

// failingRecorder is an outbox that cannot record.
type failingRecorder struct{}

func (failingRecorder) Add(context.Context, string, []byte) error {
	return errors.New("outbox is down")
}

// A like and its notification are saved in one transaction: both or none.
func TestLikeAndItsNotification(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool := newPool(t)
	posts := postgres.NewPostRepository(pool)
	likes := postgres.NewLikeRepository(pool)
	tx := pgpool.NewTxManager(pool)

	ann, bob := uuid.New(), uuid.New()
	p, err := post.New(ann, ann, post.RestoreContent("привет", nil, uuid.Nil), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = posts.Save(ctx, p); err != nil {
		t.Fatal(err)
	}

	recorded := func(requestID string) int {
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM outbox WHERE request_id = $1`, requestID).Scan(&n); err != nil {
			t.Fatalf("count outbox: %v", err)
		}
		return n
	}

	failing := service.NewPostService(posts, likes, tx, notifyclient.NewOutbox(failingRecorder{}))
	if _, err = failing.Like(ctx, bob, p.ID()); err == nil {
		t.Fatal("Like with a broken outbox = nil, want an error")
	}
	if liked, _ := likes.Liked(ctx, bob, []uuid.UUID{p.ID()}); liked[p.ID()] {
		t.Fatal("the like stayed although its notification was not recorded")
	}

	svc := service.NewPostService(posts, likes, tx, notifyclient.NewOutbox(outbox.NewPostgres(pool, notifyclient.Stream)))
	reqCtx := reqid.ContextWithID(ctx, uuid.NewString())

	if n, err := svc.Like(reqCtx, bob, p.ID()); err != nil || n != 1 {
		t.Fatalf("Like = %d, %v", n, err)
	}
	if n, err := svc.Unlike(reqCtx, bob, p.ID()); err != nil || n != 0 {
		t.Fatalf("Unlike = %d, %v", n, err)
	}
	if got := recorded(reqid.FromContext(reqCtx)); got != 2 {
		t.Errorf("recorded %d messages, want the like and its retraction", got)
	}
}
