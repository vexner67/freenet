// Package notifyclient tells the notification service about appointments
// through the outbox: Outbox records them with the change that caused
// them, Deliverer hands them over afterwards. It is the only place in club
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

// Outbox kinds: the payload is the request of the RPC of the same name,
// but for kindPurgeClub.
const (
	kindNotify  = "notify"
	kindRetract = "retract"
	// kindPurgeClub: a community is gone, and the notifications it caused
	// go with it. The payload is its ID, 16 bytes, as the migration that
	// deleted the events wrote it too.
	kindPurgeClub = "purge_club"
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

func (o *Outbox) Appointed(ctx context.Context, clubID, userID uuid.UUID) error {
	return o.add(ctx, kindNotify, &notificationv1.NotifyRequest{Event: adminEvent(clubID, userID)})
}

func (o *Outbox) Dismissed(ctx context.Context, clubID, userID uuid.UUID) error {
	return o.add(ctx, kindRetract, &notificationv1.RetractRequest{Event: adminEvent(clubID, userID)})
}

func (o *Outbox) Verified(ctx context.Context, clubID, ownerID uuid.UUID) error {
	return o.add(ctx, kindNotify, &notificationv1.NotifyRequest{Event: verifiedEvent(clubID, ownerID)})
}

func (o *Outbox) Unverified(ctx context.Context, clubID, ownerID uuid.UUID) error {
	return o.add(ctx, kindRetract, &notificationv1.RetractRequest{Event: verifiedEvent(clubID, ownerID)})
}

func (o *Outbox) Deleted(ctx context.Context, clubID uuid.UUID) error {
	return errs.Wrap("o.recorder.Add", o.recorder.Add(ctx, kindPurgeClub, clubID[:]))
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
	case kindPurgeClub:
		return d.purgeClub(ctx, m.Payload)
	default:
		return outbox.Permanent(fmt.Errorf("unknown kind %q", m.Kind))
	}
}

func (d *Deliverer) purgeClub(ctx context.Context, payload []byte) error {
	clubID, err := uuid.FromBytes(payload)
	if err != nil {
		return outbox.Permanent(errs.Wrap("uuid.FromBytes", err))
	}

	return result(d.api.PurgeUser(ctx, &notificationv1.PurgeUserRequest{UserId: clubID.String()}))
}

type rpc[Req proto.Message] func(ctx context.Context, req Req, opts ...grpc.CallOption) (*emptypb.Empty, error)

// call decodes req from payload and makes the call. Errors that a retry
// cannot fix are permanent: the notification service rejects the request
// itself, not its own state.
func call[Req proto.Message](ctx context.Context, payload []byte, req Req, fn rpc[Req]) error {
	if err := proto.Unmarshal(payload, req); err != nil {
		return outbox.Permanent(errs.Wrap("proto.Unmarshal", err))
	}

	return result(fn(ctx, req))
}

// result tells which errors of a call a retry cannot fix.
func result(_ *emptypb.Empty, err error) error {
	switch status.Code(err) {
	case codes.OK:
		return nil
	case codes.InvalidArgument, codes.NotFound, codes.AlreadyExists, codes.FailedPrecondition, codes.OutOfRange, codes.Unimplemented:
		return outbox.Permanent(err)
	default:
		return err
	}
}

// adminEvent: the club, not the owner who appointed, is the actor, as in
// the notifications about suggested posts.
func adminEvent(clubID, userID uuid.UUID) *notificationv1.Event {
	return &notificationv1.Event{
		Kind:        notificationv1.Kind_KIND_CLUB_ADMIN,
		RecipientId: userID.String(),
		ActorId:     clubID.String(),
	}
}

// verifiedEvent is from the club, as the appointment is: who gave the
// checkmark is not the owner's business.
func verifiedEvent(clubID, ownerID uuid.UUID) *notificationv1.Event {
	return &notificationv1.Event{
		Kind:        notificationv1.Kind_KIND_CLUB_VERIFIED,
		RecipientId: ownerID.String(),
		ActorId:     clubID.String(),
	}
}

// Nop is the Notifier when no notification service is configured.
type Nop struct{}

func (Nop) Appointed(context.Context, uuid.UUID, uuid.UUID) error  { return nil }
func (Nop) Dismissed(context.Context, uuid.UUID, uuid.UUID) error  { return nil }
func (Nop) Verified(context.Context, uuid.UUID, uuid.UUID) error   { return nil }
func (Nop) Unverified(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (Nop) Deleted(context.Context, uuid.UUID) error               { return nil }
