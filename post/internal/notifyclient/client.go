// Package notifyclient tells the notification service about post events
// through the outbox: Outbox records them with the change that caused
// them, Deliverer hands them over afterwards. It is the only place in post
// that knows the notification proto contract.
package notifyclient

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	notificationv1 "github.com/vexner67/freenet/notification/api/notification/v1"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/outbox"
	"github.com/vexner67/freenet/post/internal/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Stream is the outbox stream of the notification service.
const Stream = "notification"

// Outbox kinds: the payload is the request of the RPC of the same name.
const (
	kindNotify        = "notify"
	kindRetract       = "retract"
	kindForgetPost    = "forget_post"
	kindForgetComment = "forget_comment"
)

var kinds = map[service.EventKind]notificationv1.Kind{
	service.EventLike:     notificationv1.Kind_KIND_LIKE,
	service.EventComment:  notificationv1.Kind_KIND_COMMENT,
	service.EventWallPost: notificationv1.Kind_KIND_WALL_POST,
	service.EventRepost:   notificationv1.Kind_KIND_REPOST,

	service.EventCommentReply: notificationv1.Kind_KIND_COMMENT_REPLY,
	service.EventCommentLike:  notificationv1.Kind_KIND_COMMENT_LIKE,
	service.EventPostAccepted: notificationv1.Kind_KIND_POST_ACCEPTED,
}

// Recorder adds a message to the outbox in the transaction of ctx.
type Recorder interface {
	Add(ctx context.Context, kind string, payload []byte) error
}

// Outbox is the service.Notifier: it records the requests to make.
type Outbox struct {
	recorder Recorder
}

func NewOutbox(recorder Recorder) *Outbox {
	return &Outbox{
		recorder: recorder,
	}
}

func (o *Outbox) Notify(ctx context.Context, e service.Event) error {
	return o.add(ctx, kindNotify, &notificationv1.NotifyRequest{Event: toProtoEvent(e)})
}

func (o *Outbox) Retract(ctx context.Context, e service.Event) error {
	return o.add(ctx, kindRetract, &notificationv1.RetractRequest{Event: toProtoEvent(e)})
}

func (o *Outbox) ForgetPost(ctx context.Context, postID uuid.UUID) error {
	return o.add(ctx, kindForgetPost, &notificationv1.ForgetPostRequest{PostId: postID.String()})
}

func (o *Outbox) ForgetComment(ctx context.Context, commentID uuid.UUID) error {
	return o.add(ctx, kindForgetComment, &notificationv1.ForgetCommentRequest{CommentId: commentID.String()})
}

func (o *Outbox) add(ctx context.Context, kind string, req proto.Message) error {
	payload, err := proto.Marshal(req)
	if err != nil {
		return errs.Wrap("proto.Marshal", err)
	}

	return errs.Wrap("o.recorder.Add", o.recorder.Add(ctx, kind, payload))
}

// Deliverer makes the recorded requests to the notification service.
type Deliverer struct {
	api     notificationv1.NotificationServiceClient
	timeout time.Duration
}

func NewDeliverer(api notificationv1.NotificationServiceClient, timeout time.Duration) *Deliverer {
	return &Deliverer{
		api:     api,
		timeout: timeout,
	}
}

// Deliver is an outbox.Deliver.
func (d *Deliverer) Deliver(ctx context.Context, m outbox.Message) error {
	ctx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()

	switch m.Kind {
	case kindNotify:
		return call(ctx, m.Payload, &notificationv1.NotifyRequest{}, d.api.Notify)
	case kindRetract:
		return call(ctx, m.Payload, &notificationv1.RetractRequest{}, d.api.Retract)
	case kindForgetPost:
		return call(ctx, m.Payload, &notificationv1.ForgetPostRequest{}, d.api.ForgetPost)
	case kindForgetComment:
		return call(ctx, m.Payload, &notificationv1.ForgetCommentRequest{}, d.api.ForgetComment)
	default:
		return outbox.Permanent(fmt.Errorf("unknown kind %q", m.Kind))
	}
}

type rpc[Req proto.Message] func(ctx context.Context, req Req, opts ...grpc.CallOption) (*emptypb.Empty, error)

// call decodes req from payload and makes the call. Errors that a retry
// cannot fix are permanent: the notification service rejects the request
// itself, not its own state.
func call[Req proto.Message](ctx context.Context, payload []byte, req Req, fn rpc[Req]) error {
	if err := proto.Unmarshal(payload, req); err != nil {
		return outbox.Permanent(errs.Wrap("proto.Unmarshal", err))
	}

	_, err := fn(ctx, req)

	switch status.Code(err) {
	case codes.OK:
		return nil
	case codes.InvalidArgument, codes.NotFound, codes.AlreadyExists, codes.FailedPrecondition, codes.OutOfRange, codes.Unimplemented:
		return outbox.Permanent(err)
	default:
		return err
	}
}

func toProtoEvent(e service.Event) *notificationv1.Event {
	pe := &notificationv1.Event{
		Kind:        kinds[e.Kind],
		RecipientId: e.RecipientID.String(),
		ActorId:     e.ActorID.String(),
		PostId:      e.PostID.String(),
	}

	if e.CommentID != uuid.Nil {
		pe.CommentId = e.CommentID.String()
	}

	return pe
}

// Nop is the Notifier when no notification service is configured.
type Nop struct{}

func (Nop) Notify(context.Context, service.Event) error  { return nil }
func (Nop) Retract(context.Context, service.Event) error { return nil }
func (Nop) ForgetPost(context.Context, uuid.UUID) error  { return nil }

func (Nop) ForgetComment(context.Context, uuid.UUID) error { return nil }
