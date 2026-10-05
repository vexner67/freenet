package notifyclient

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	notificationv1 "github.com/vexner67/freenet/notification/api/notification/v1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

type fakeAPI struct {
	notificationv1.NotificationServiceClient
	notified  *notificationv1.Event
	retracted *notificationv1.Event
	ctxErr    error
	err       error
}

func (f *fakeAPI) Notify(ctx context.Context, req *notificationv1.NotifyRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	f.notified, f.ctxErr = req.GetEvent(), ctx.Err()
	return &emptypb.Empty{}, f.err
}

func (f *fakeAPI) Retract(_ context.Context, req *notificationv1.RetractRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	f.retracted = req.GetEvent()
	return &emptypb.Empty{}, f.err
}

func TestClient(t *testing.T) {
	t.Parallel()

	api := &fakeAPI{}
	c := New(api, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	follower, followee := uuid.New(), uuid.New()

	// The request is over, the notification still goes.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	c.Followed(ctx, follower, followee)
	if api.ctxErr != nil {
		t.Fatalf("the call inherited the cancellation: %v", api.ctxErr)
	}
	if e := api.notified; e.GetKind() != notificationv1.Kind_KIND_FOLLOW || e.GetRecipientId() != followee.String() ||
		e.GetActorId() != follower.String() || e.GetPostId() != "" {
		t.Fatalf("follow event = %v", e)
	}

	c.Unfollowed(context.Background(), follower, followee)
	if e := api.retracted; e.GetRecipientId() != followee.String() || e.GetActorId() != follower.String() {
		t.Fatalf("unfollow event = %v", e)
	}
}

func TestClient_FailureIsLogged(t *testing.T) {
	t.Parallel()

	var logs strings.Builder
	c := New(&fakeAPI{err: errors.New("unavailable")}, time.Second, slog.New(slog.NewTextHandler(&logs, nil)))

	c.Followed(context.Background(), uuid.New(), uuid.New())

	if !strings.Contains(logs.String(), "notification not delivered") {
		t.Fatalf("logs = %q", logs.String())
	}
}
