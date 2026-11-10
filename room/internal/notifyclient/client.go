package notifyclient

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	notificationv1 "github.com/vexner67/freenet/notification/api/notification/v1"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/outbox"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"time"
)

const Stream = "notification"

type Recorder interface {
	Add(context.Context, string, []byte) error
}
type Outbox struct{ recorder Recorder }

func NewOutbox(recorder Recorder) *Outbox { return &Outbox{recorder: recorder} }
func (o *Outbox) Invite(ctx context.Context, actor, recipient, id uuid.UUID, _ string) error {
	return o.add(ctx, "invite", &notificationv1.NotifyRequest{Event: &notificationv1.Event{Kind: notificationv1.Kind_KIND_ROOM_INVITE, ActorId: actor.String(), RecipientId: recipient.String(), PostId: id.String()}})
}
func (o *Outbox) Revoke(ctx context.Context, id uuid.UUID) error {
	return o.add(ctx, "close", &notificationv1.ForgetPostRequest{PostId: id.String()})
}
func (o *Outbox) add(ctx context.Context, kind string, message proto.Message) error {
	payload, err := proto.Marshal(message)
	if err != nil {
		return errs.Wrap("proto.Marshal", err)
	}
	return errs.Wrap("o.recorder.Add", o.recorder.Add(ctx, kind, payload))
}

type Deliverer struct {
	client  notificationv1.NotificationServiceClient
	timeout time.Duration
}

func NewDeliverer(client notificationv1.NotificationServiceClient, timeout time.Duration) *Deliverer {
	return &Deliverer{client: client, timeout: timeout}
}
func (d *Deliverer) Deliver(ctx context.Context, message outbox.Message) error {
	ctx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()
	var err error
	switch message.Kind {
	case "invite":
		req := &notificationv1.NotifyRequest{}
		if err = proto.Unmarshal(message.Payload, req); err != nil {
			return outbox.Permanent(errs.Wrap("proto.Unmarshal", err))
		}
		_, err = d.client.Notify(ctx, req)
	case "close":
		req := &notificationv1.ForgetPostRequest{}
		if err = proto.Unmarshal(message.Payload, req); err != nil {
			return outbox.Permanent(errs.Wrap("proto.Unmarshal", err))
		}
		_, err = d.client.ForgetPost(ctx, req)
	default:
		return outbox.Permanent(fmt.Errorf("unknown room notification %q", message.Kind))
	}
	switch status.Code(err) {
	case codes.InvalidArgument, codes.NotFound, codes.PermissionDenied:
		return outbox.Permanent(err)
	}
	return errs.Wrap("d.client notification RPC", err)
}
