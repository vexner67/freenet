package notifyclient

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	notificationv1 "github.com/vexner67/freenet/notification/api/notification/v1"
	"github.com/vexner67/freenet/pkg/outbox"
	"github.com/vexner67/freenet/post/internal/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type fakeAPI struct {
	notificationv1.NotificationServiceClient
	notified  *notificationv1.Event
	retracted *notificationv1.Event
	forgot    string
	forgotC   string
	deadline  bool
	err       error
}

func (f *fakeAPI) Notify(ctx context.Context, req *notificationv1.NotifyRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	_, f.deadline = ctx.Deadline()
	f.notified = req.GetEvent()
	return &emptypb.Empty{}, f.err
}

func (f *fakeAPI) Retract(_ context.Context, req *notificationv1.RetractRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	f.retracted = req.GetEvent()
	return &emptypb.Empty{}, f.err
}

func (f *fakeAPI) ForgetPost(_ context.Context, req *notificationv1.ForgetPostRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	f.forgot = req.GetPostId()
	return &emptypb.Empty{}, f.err
}

func (f *fakeAPI) ForgetComment(_ context.Context, req *notificationv1.ForgetCommentRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	f.forgotC = req.GetCommentId()
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

	like := service.Event{Kind: service.EventLike, RecipientID: uuid.New(), ActorID: uuid.New(), PostID: uuid.New()}
	if err := roundTrip(t, api, func(o *Outbox) error { return o.Notify(ctx, like) }); err != nil {
		t.Fatalf("deliver like: %v", err)
	}
	if e := api.notified; e.GetKind() != notificationv1.Kind_KIND_LIKE || e.GetRecipientId() != like.RecipientID.String() ||
		e.GetActorId() != like.ActorID.String() || e.GetPostId() != like.PostID.String() || e.GetCommentId() != "" {
		t.Fatalf("like event = %v", e)
	}
	if !api.deadline {
		t.Error("the call has no timeout")
	}

	comment := service.Event{Kind: service.EventComment, RecipientID: uuid.New(), ActorID: uuid.New(), PostID: uuid.New(), CommentID: uuid.New()}
	if err := roundTrip(t, api, func(o *Outbox) error { return o.Retract(ctx, comment) }); err != nil {
		t.Fatalf("deliver comment: %v", err)
	}
	if e := api.retracted; e.GetKind() != notificationv1.Kind_KIND_COMMENT || e.GetCommentId() != comment.CommentID.String() {
		t.Fatalf("comment event = %v", e)
	}

	wallPost := service.Event{Kind: service.EventWallPost, RecipientID: uuid.New(), ActorID: uuid.New(), PostID: uuid.New()}
	if err := roundTrip(t, api, func(o *Outbox) error { return o.Notify(ctx, wallPost) }); err != nil {
		t.Fatalf("deliver wall post: %v", err)
	}
	if e := api.notified; e.GetKind() != notificationv1.Kind_KIND_WALL_POST || e.GetPostId() != wallPost.PostID.String() {
		t.Fatalf("wall post event = %v", e)
	}

	repost := service.Event{Kind: service.EventRepost, RecipientID: uuid.New(), ActorID: uuid.New(), PostID: uuid.New()}
	if err := roundTrip(t, api, func(o *Outbox) error { return o.Notify(ctx, repost) }); err != nil {
		t.Fatalf("deliver repost: %v", err)
	}
	if e := api.notified; e.GetKind() != notificationv1.Kind_KIND_REPOST || e.GetPostId() != repost.PostID.String() {
		t.Fatalf("repost event = %v", e)
	}

	reply := service.Event{Kind: service.EventCommentReply, RecipientID: uuid.New(), ActorID: uuid.New(), PostID: uuid.New(), CommentID: uuid.New()}
	if err := roundTrip(t, api, func(o *Outbox) error { return o.Notify(ctx, reply) }); err != nil {
		t.Fatalf("deliver reply: %v", err)
	}
	if e := api.notified; e.GetKind() != notificationv1.Kind_KIND_COMMENT_REPLY || e.GetCommentId() != reply.CommentID.String() {
		t.Fatalf("reply event = %v", e)
	}

	commentLike := service.Event{Kind: service.EventCommentLike, RecipientID: uuid.New(), ActorID: uuid.New(), PostID: uuid.New(), CommentID: uuid.New()}
	if err := roundTrip(t, api, func(o *Outbox) error { return o.Retract(ctx, commentLike) }); err != nil {
		t.Fatalf("deliver comment like: %v", err)
	}
	if e := api.retracted; e.GetKind() != notificationv1.Kind_KIND_COMMENT_LIKE {
		t.Fatalf("comment like event = %v", e)
	}

	commentID := uuid.New()
	if err := roundTrip(t, api, func(o *Outbox) error { return o.ForgetComment(ctx, commentID) }); err != nil {
		t.Fatalf("deliver forget comment: %v", err)
	}
	if api.forgotC != commentID.String() {
		t.Fatalf("forgot comment %q", api.forgotC)
	}

	postID := uuid.New()
	if err := roundTrip(t, api, func(o *Outbox) error { return o.ForgetPost(ctx, postID) }); err != nil {
		t.Fatalf("deliver forget post: %v", err)
	}
	if api.forgot != postID.String() {
		t.Fatalf("forgot %q", api.forgot)
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
		{name: "deadline is retried", apiErr: status.Error(codes.DeadlineExceeded, "slow"), wantErr: true},
		{name: "plain error is retried", apiErr: errors.New("broken"), wantErr: true},
		{name: "invalid is dropped", apiErr: status.Error(codes.InvalidArgument, "self"), wantErr: true, permanent: true},
		{name: "unknown kind is dropped", message: outbox.Message{Kind: "teleport"}, wantErr: true, permanent: true},
		{name: "bad payload is dropped", message: outbox.Message{Kind: kindNotify, Payload: []byte{0xff}}, wantErr: true, permanent: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m := tt.message
			if m.Kind == "" {
				m.Kind = kindNotify
			}

			err := NewDeliverer(&fakeAPI{err: tt.apiErr}, time.Second).Deliver(context.Background(), m)
			if (err != nil) != tt.wantErr || outbox.IsPermanent(err) != tt.permanent {
				t.Errorf("Deliver = %v (permanent %v), want error %v, permanent %v", err, outbox.IsPermanent(err), tt.wantErr, tt.permanent)
			}
		})
	}
}
