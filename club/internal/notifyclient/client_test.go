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
	// calls are the RPCs made, in order.
	calls []string
	err   error
}

func (f *fakeAPI) Notify(_ context.Context, req *notificationv1.NotifyRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	f.notified = req.GetEvent()
	f.calls = append(f.calls, "notify")
	return &emptypb.Empty{}, f.err
}

func (f *fakeAPI) Retract(_ context.Context, req *notificationv1.RetractRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	f.retracted = req.GetEvent()
	f.calls = append(f.calls, "retract")
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
	clubID, admin := uuid.New(), uuid.New()

	if err := o.Appointed(context.Background(), clubID, admin); err != nil {
		t.Fatal(err)
	}

	api := &fakeAPI{}
	if errs := deliverAll(t, queue, NewDeliverer(api, time.Second)); len(errs) != 1 || errs[0] != nil {
		t.Fatalf("deliver = %v", errs)
	}
	if e := api.notified; e.GetKind() != notificationv1.Kind_KIND_CLUB_ADMIN || e.GetActorId() != clubID.String() ||
		e.GetRecipientId() != admin.String() || e.GetPostId() != "" {
		t.Fatalf("appointment event = %v", e)
	}

	queue = outbox.NewMemory()
	if err := NewOutbox(queue).Dismissed(context.Background(), clubID, admin); err != nil {
		t.Fatal(err)
	}
	if errs := deliverAll(t, queue, NewDeliverer(api, time.Second)); len(errs) != 1 || errs[0] != nil {
		t.Fatalf("deliver = %v", errs)
	}
	if e := api.retracted; e.GetKind() != notificationv1.Kind_KIND_CLUB_ADMIN || e.GetActorId() != clubID.String() {
		t.Fatalf("dismissal event = %v", e)
	}
}

func TestDeliverer_Errors(t *testing.T) {
	t.Parallel()

	queue := outbox.NewMemory()
	if err := NewOutbox(queue).Appointed(context.Background(), uuid.New(), uuid.New()); err != nil {
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

func TestOutbox_Reminded(t *testing.T) {
	t.Parallel()

	queue := outbox.NewMemory()
	event, member := uuid.New(), uuid.New()

	if err := NewOutbox(queue).Reminded(context.Background(), event, member); err != nil {
		t.Fatal(err)
	}

	// An earlier reminder is taken back, then the new one is told.
	api := &fakeAPI{}
	if errs := deliverAll(t, queue, NewDeliverer(api, time.Second)); len(errs) != 2 || errs[0] != nil || errs[1] != nil {
		t.Fatalf("deliver = %v", errs)
	}
	if len(api.calls) != 2 || api.calls[0] != "retract" || api.calls[1] != "notify" {
		t.Fatalf("calls = %v, want retract, notify", api.calls)
	}

	for _, e := range []*notificationv1.Event{api.retracted, api.notified} {
		if e.GetKind() != notificationv1.Kind_KIND_EVENT_REMINDER || e.GetActorId() != event.String() ||
			e.GetRecipientId() != member.String() || e.GetPostId() != "" {
			t.Fatalf("reminder event = %v", e)
		}
	}

	if err := (Nop{}).Reminded(context.Background(), event, member); err != nil {
		t.Fatal(err)
	}
}
