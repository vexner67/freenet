package mediaclient

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	mediav1 "github.com/vexner67/freenet/media/api/media/v1"
	"github.com/vexner67/freenet/pkg/outbox"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type fakeAPI struct {
	mediav1.MediaServiceClient
	published *mediav1.PublishPhotosRequest
	deleted   *mediav1.DeletePublishedPhotosRequest
	deadline  bool
	err       error
}

func (f *fakeAPI) PublishPhotos(ctx context.Context, req *mediav1.PublishPhotosRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	_, f.deadline = ctx.Deadline()
	f.published = req
	return &emptypb.Empty{}, f.err
}

func (f *fakeAPI) DeletePublishedPhotos(_ context.Context, req *mediav1.DeletePublishedPhotosRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	f.deleted = req
	return &emptypb.Empty{}, f.err
}

// roundTrip records with Outbox and delivers the recorded message with
// Deliverer, as the relay would.
func roundTrip(t *testing.T, api *fakeAPI, record func(*Outbox) error) error {
	t.Helper()

	queue := outbox.NewMemory()
	if err := record(NewOutbox(queue)); err != nil {
		t.Fatalf("record: %v", err)
	}

	messages, _ := queue.Head(context.Background(), 10)
	if len(messages) != 1 {
		t.Fatalf("recorded %d messages, want 1", len(messages))
	}

	return NewDeliverer(api, time.Second).Deliver(context.Background(), messages[0])
}

func TestOutboxAndDeliverer(t *testing.T) {
	t.Parallel()

	api := &fakeAPI{}
	ctx := context.Background()
	owner, photos := uuid.New(), []uuid.UUID{uuid.New(), uuid.New()}
	want := []string{photos[0].String(), photos[1].String()}

	if err := roundTrip(t, api, func(o *Outbox) error { return o.Publish(ctx, owner, photos) }); err != nil {
		t.Fatalf("deliver publish: %v", err)
	}
	if r := api.published; r.GetOwnerId() != owner.String() || !slices.Equal(r.GetPhotoIds(), want) {
		t.Fatalf("published %v", r)
	}
	if !api.deadline {
		t.Error("the call has no timeout")
	}

	if err := roundTrip(t, api, func(o *Outbox) error { return o.Delete(ctx, owner, photos) }); err != nil {
		t.Fatalf("deliver delete: %v", err)
	}
	if r := api.deleted; r.GetOwnerId() != owner.String() || !slices.Equal(r.GetPhotoIds(), want) {
		t.Fatalf("deleted %v", r)
	}
}

func TestDeliverer_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		apiErr    error
		message   outbox.Message
		wantErr   bool
		permanent bool
	}{
		{name: "unavailable is retried", apiErr: status.Error(codes.Unavailable, "down"), wantErr: true},
		{name: "denied is retried, mTLS may be set up wrong", apiErr: status.Error(codes.PermissionDenied, "who"), wantErr: true},
		{name: "plain error is retried", apiErr: errors.New("broken"), wantErr: true},
		{name: "invalid is dropped", apiErr: status.Error(codes.InvalidArgument, "bad id"), wantErr: true, permanent: true},
		{name: "unknown kind is dropped", message: outbox.Message{Kind: "teleport"}, wantErr: true, permanent: true},
		{name: "bad payload is dropped", message: outbox.Message{Kind: kindDelete, Payload: []byte{0xff}}, wantErr: true, permanent: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m := tt.message
			if m.Kind == "" {
				m.Kind = kindPublish
			}

			err := NewDeliverer(&fakeAPI{err: tt.apiErr}, time.Second).Deliver(context.Background(), m)
			if (err != nil) != tt.wantErr || outbox.IsPermanent(err) != tt.permanent {
				t.Errorf("Deliver = %v (permanent %v), want error %v, permanent %v", err, outbox.IsPermanent(err), tt.wantErr, tt.permanent)
			}
		})
	}
}
