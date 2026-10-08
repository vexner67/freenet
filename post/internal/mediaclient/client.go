// Package mediaclient tells the media service about the photos of posts
// through the outbox: Outbox records it with the post published or
// deleted, Deliverer hands it over afterwards. It is the only place in
// post that knows the media proto contract.
package mediaclient

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	mediav1 "github.com/vexner67/freenet/media/api/media/v1"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/outbox"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Stream is the outbox stream of the media service: in one stream a
// post's photos are published before they are deleted.
const Stream = "media"

// Outbox kinds: the payload is the request of the RPC of the same name.
const (
	kindPublish = "publish_photos"
	kindDelete  = "delete_photos"
)

// Recorder adds a message to the outbox in the transaction of ctx.
type Recorder interface {
	Add(ctx context.Context, kind string, payload []byte) error
}

// Outbox is the service.PhotoKeeper: it records the requests to make.
type Outbox struct {
	recorder Recorder
}

func NewOutbox(recorder Recorder) *Outbox {
	return &Outbox{
		recorder: recorder,
	}
}

func (o *Outbox) Publish(ctx context.Context, ownerID uuid.UUID, photoIDs []uuid.UUID) error {
	return o.add(ctx, kindPublish, &mediav1.PublishPhotosRequest{OwnerId: ownerID.String(), PhotoIds: toStrings(photoIDs)})
}

func (o *Outbox) Delete(ctx context.Context, ownerID uuid.UUID, photoIDs []uuid.UUID) error {
	return o.add(ctx, kindDelete, &mediav1.DeletePublishedPhotosRequest{OwnerId: ownerID.String(), PhotoIds: toStrings(photoIDs)})
}

func (o *Outbox) add(ctx context.Context, kind string, req proto.Message) error {
	payload, err := proto.Marshal(req)
	if err != nil {
		return errs.Wrap("proto.Marshal", err)
	}

	return errs.Wrap("o.recorder.Add", o.recorder.Add(ctx, kind, payload))
}

func toStrings(ids []uuid.UUID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id.String()
	}

	return out
}

// Deliverer makes the recorded requests to the media service.
type Deliverer struct {
	api     mediav1.MediaServiceClient
	timeout time.Duration
}

func NewDeliverer(api mediav1.MediaServiceClient, timeout time.Duration) *Deliverer {
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
	case kindPublish:
		return call(ctx, m.Payload, &mediav1.PublishPhotosRequest{}, d.api.PublishPhotos)
	case kindDelete:
		return call(ctx, m.Payload, &mediav1.DeletePublishedPhotosRequest{}, d.api.DeletePublishedPhotos)
	default:
		return outbox.Permanent(fmt.Errorf("unknown kind %q", m.Kind))
	}
}

type rpc[Req proto.Message] func(ctx context.Context, req Req, opts ...grpc.CallOption) (*emptypb.Empty, error)

// call decodes req from payload and makes the call. Errors that a retry
// cannot fix are permanent: media rejects the request itself.
func call[Req proto.Message](ctx context.Context, payload []byte, req Req, fn rpc[Req]) error {
	if err := proto.Unmarshal(payload, req); err != nil {
		return outbox.Permanent(errs.Wrap("proto.Unmarshal", err))
	}

	_, err := fn(ctx, req)

	switch status.Code(err) {
	case codes.OK:
		return nil
	case codes.InvalidArgument, codes.NotFound, codes.Unimplemented:
		return outbox.Permanent(err)
	default:
		return err
	}
}

// Nop is the PhotoKeeper when no media service is configured.
type Nop struct{}

func (Nop) Publish(context.Context, uuid.UUID, []uuid.UUID) error { return nil }
func (Nop) Delete(context.Context, uuid.UUID, []uuid.UUID) error  { return nil }
