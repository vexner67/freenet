package notifyclient

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	notificationv1 "github.com/vexner67/freenet/notification/api/notification/v1"
	"github.com/vexner67/freenet/pkg/outbox"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type fakeAPI struct {
	notificationv1.NotificationServiceClient
	notified  *notificationv1.Event
	retracted *notificationv1.Event
	err       error
}

func (f *fakeAPI) Notify(_ context.Context, req *notificationv1.NotifyRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	f.notified = req.GetEvent()
	return &emptypb.Empty{}, f.err
}

func (f *fakeAPI) Retract(_ context.Context, req *notificationv1.RetractRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	f.retracted = req.GetEvent()
	return &emptypb.Empty{}, f.err
}

// deliverAll delivers what Outbox recorded, as the relay would.
func deliverAll(t *testing.T, queue *outbox.Memory, d *Deliverer) []error {
	t.Helper()

	messages, _ := queue.Head(context.Background(), 10)

	var errs []error
	for _, m := range messages {
		errs = append(errs, d.Deliver(context.Background(), m))
	}

	return errs
}

func TestOutboxAndDeliverer(t *testing.T) {
	t.Parallel()

	queue := outbox.NewMemory()
	o := NewOutbox(queue)
	follower, followee := uuid.New(), uuid.New()

	if err := o.Followed(context.Background(), follower, followee); err != nil {
		t.Fatal(err)
	}

	api := &fakeAPI{}
	if errs := deliverAll(t, queue, NewDeliverer(api, time.Second)); len(errs) != 1 || errs[0] != nil {
		t.Fatalf("deliver = %v", errs)
	}
	if e := api.notified; e.GetKind() != notificationv1.Kind_KIND_FOLLOW || e.GetActorId() != follower.String() ||
		e.GetRecipientId() != followee.String() || e.GetPostId() != "" {
		t.Fatalf("follow event = %v", e)
	}

	queue = outbox.NewMemory()
	if err := NewOutbox(queue).Unfollowed(context.Background(), follower, followee); err != nil {
		t.Fatal(err)
	}
	if errs := deliverAll(t, queue, NewDeliverer(api, time.Second)); len(errs) != 1 || errs[0] != nil {
		t.Fatalf("deliver = %v", errs)
	}
	if e := api.retracted; e.GetKind() != notificationv1.Kind_KIND_FOLLOW || e.GetActorId() != follower.String() {
		t.Fatalf("unfollow event = %v", e)
	}
}

func TestOutbox_Gifts(t *testing.T) {
	t.Parallel()

	gift, sender, recipient := uuid.New(), uuid.New(), uuid.New()
	api := &fakeAPI{}

	queue := outbox.NewMemory()
	if err := NewOutbox(queue).GiftSent(context.Background(), gift, sender, recipient); err != nil {
		t.Fatal(err)
	}
	if errs := deliverAll(t, queue, NewDeliverer(api, time.Second)); len(errs) != 1 || errs[0] != nil {
		t.Fatalf("deliver = %v", errs)
	}
	if e := api.notified; e.GetKind() != notificationv1.Kind_KIND_GIFT || e.GetActorId() != sender.String() ||
		e.GetRecipientId() != recipient.String() || e.GetPostId() != gift.String() || e.GetCommentId() != "" {
		t.Fatalf("gift event = %v", e)
	}

	queue = outbox.NewMemory()
	if err := NewOutbox(queue).GiftDeleted(context.Background(), gift, sender, recipient); err != nil {
		t.Fatal(err)
	}
	if errs := deliverAll(t, queue, NewDeliverer(api, time.Second)); len(errs) != 1 || errs[0] != nil {
		t.Fatalf("deliver = %v", errs)
	}
	if e := api.retracted; e.GetKind() != notificationv1.Kind_KIND_GIFT || e.GetPostId() != gift.String() {
		t.Fatalf("deleted gift event = %v", e)
	}
}

func TestDeliverer_Errors(t *testing.T) {
	t.Parallel()

	queue := outbox.NewMemory()
	if err := NewOutbox(queue).Followed(context.Background(), uuid.New(), uuid.New()); err != nil {
		t.Fatal(err)
	}
	messages, _ := queue.Head(context.Background(), 1)

	retried := NewDeliverer(&fakeAPI{err: status.Error(codes.Unavailable, "down")}, time.Second).Deliver(context.Background(), messages[0])
	if retried == nil || outbox.IsPermanent(retried) {
		t.Errorf("unavailable = %v, want an error to retry", retried)
	}

	dropped := NewDeliverer(&fakeAPI{err: status.Error(codes.InvalidArgument, "self")}, time.Second).Deliver(context.Background(), messages[0])
	if !outbox.IsPermanent(dropped) {
		t.Errorf("invalid argument = %v, want a permanent error", dropped)
	}

	unknown := NewDeliverer(&fakeAPI{}, time.Second).Deliver(context.Background(), outbox.Message{Kind: "forget_post"})
	if !outbox.IsPermanent(unknown) {
		t.Errorf("unknown kind = %v, want a permanent error", unknown)
	}
}
