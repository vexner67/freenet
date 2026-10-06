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
	"github.com/vexner67/freenet/post/internal/service"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

type fakeAPI struct {
	notificationv1.NotificationServiceClient
	notified  *notificationv1.Event
	retracted *notificationv1.Event
	forgot    string
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

func (f *fakeAPI) ForgetPost(_ context.Context, req *notificationv1.ForgetPostRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	f.forgot = req.GetPostId()
	return &emptypb.Empty{}, f.err
}

func TestClient(t *testing.T) {
	t.Parallel()

	api := &fakeAPI{}
	c := New(api, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))

	like := service.Event{Kind: service.EventLike, RecipientID: uuid.New(), ActorID: uuid.New(), PostID: uuid.New()}

	// The request is over, the notification still goes.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	c.Notify(ctx, like)
	if api.ctxErr != nil {
		t.Fatalf("the call inherited the cancellation: %v", api.ctxErr)
	}
	if e := api.notified; e.GetKind() != notificationv1.Kind_KIND_LIKE || e.GetRecipientId() != like.RecipientID.String() ||
		e.GetActorId() != like.ActorID.String() || e.GetPostId() != like.PostID.String() || e.GetCommentId() != "" {
		t.Fatalf("like event = %v", e)
	}

	comment := service.Event{Kind: service.EventComment, RecipientID: uuid.New(), ActorID: uuid.New(), PostID: uuid.New(), CommentID: uuid.New()}
	c.Retract(context.Background(), comment)
	if e := api.retracted; e.GetKind() != notificationv1.Kind_KIND_COMMENT || e.GetCommentId() != comment.CommentID.String() {
		t.Fatalf("comment event = %v", e)
	}

	wallPost := service.Event{Kind: service.EventWallPost, RecipientID: uuid.New(), ActorID: uuid.New(), PostID: uuid.New()}
	c.Notify(context.Background(), wallPost)
	if e := api.notified; e.GetKind() != notificationv1.Kind_KIND_WALL_POST || e.GetRecipientId() != wallPost.RecipientID.String() ||
		e.GetPostId() != wallPost.PostID.String() || e.GetCommentId() != "" {
		t.Fatalf("wall post event = %v", e)
	}

	repost := service.Event{Kind: service.EventRepost, RecipientID: uuid.New(), ActorID: uuid.New(), PostID: uuid.New()}
	c.Notify(context.Background(), repost)
	if e := api.notified; e.GetKind() != notificationv1.Kind_KIND_REPOST || e.GetPostId() != repost.PostID.String() {
		t.Fatalf("repost event = %v", e)
	}

	postID := uuid.New()
	c.ForgetPost(context.Background(), postID)
	if api.forgot != postID.String() {
		t.Fatalf("forgot %q", api.forgot)
	}
}

func TestClient_FailureIsLogged(t *testing.T) {
	t.Parallel()

	var logs strings.Builder
	c := New(&fakeAPI{err: errors.New("unavailable")}, time.Second, slog.New(slog.NewTextHandler(&logs, nil)))

	c.Notify(context.Background(), service.Event{Kind: service.EventLike})

	if !strings.Contains(logs.String(), "notification not delivered") || !strings.Contains(logs.String(), "unavailable") {
		t.Fatalf("logs = %q", logs.String())
	}
}
