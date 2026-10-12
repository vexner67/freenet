// Package notifyclient tells the notification service about follows and
// gifts through the outbox: Outbox records them with the change that caused
// them, Deliverer hands them over afterwards. It is the only place in user
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
	kindNotify  = "notify"
	kindRetract = "retract"
)

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

func (o *Outbox) Followed(ctx context.Context, followerID, followeeID uuid.UUID) error {
	return o.add(ctx, kindNotify, &notificationv1.NotifyRequest{Event: followEvent(followerID, followeeID)})
}

func (o *Outbox) Unfollowed(ctx context.Context, followerID, followeeID uuid.UUID) error {
	return o.add(ctx, kindRetract, &notificationv1.RetractRequest{Event: followEvent(followerID, followeeID)})
}

func (o *Outbox) GiftSent(ctx context.Context, giftID, senderID, recipientID uuid.UUID) error {
	return o.add(ctx, kindNotify, &notificationv1.NotifyRequest{Event: giftEvent(giftID, senderID, recipientID)})
}

func (o *Outbox) GiftDeleted(ctx context.Context, giftID, senderID, recipientID uuid.UUID) error {
	return o.add(ctx, kindRetract, &notificationv1.RetractRequest{Event: giftEvent(giftID, senderID, recipientID)})
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

func followEvent(followerID, followeeID uuid.UUID) *notificationv1.Event {
	return &notificationv1.Event{
		Kind:        notificationv1.Kind_KIND_FOLLOW,
		RecipientId: followeeID.String(),
		ActorId:     followerID.String(),
	}
}

// giftEvent names the sent gift where other events name a post: one
// notification per gift, however many one person sends.
func giftEvent(giftID, senderID, recipientID uuid.UUID) *notificationv1.Event {
	return &notificationv1.Event{
		Kind:        notificationv1.Kind_KIND_GIFT,
		RecipientId: recipientID.String(),
		ActorId:     senderID.String(),
		PostId:      giftID.String(),
	}
}

// Nop is the Notifier when no notification service is configured.
type Nop struct{}

func (Nop) Followed(context.Context, uuid.UUID, uuid.UUID) error   { return nil }
func (Nop) Unfollowed(context.Context, uuid.UUID, uuid.UUID) error { return nil }

func (Nop) GiftSent(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error    { return nil }
func (Nop) GiftDeleted(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error { return nil }
