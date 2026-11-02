// Package fediclient tells the federation service about the posts of
// authors on their own walls, which may go to the Fediverse, through the
// outbox: Outbox records it with the post written, edited or deleted,
// Deliverer hands it over afterwards. It is the only place in post that
// knows the federation proto contract.
package fediclient

import (
	"context"
	"fmt"
	"time"

	federationv1 "github.com/vexner67/freenet/federation/api/federation/v1"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/outbox"
	"github.com/vexner67/freenet/post/internal/post"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// Stream is the outbox stream of the federation service.
const Stream = "federation"

// kindPostChanged: the payload is a PostChangedRequest.
const kindPostChanged = "post_changed"

// Recorder adds a message to the outbox in the transaction of ctx.
type Recorder interface {
	Add(ctx context.Context, kind string, payload []byte) error
}

// Outbox is the service.Federator: it records the changes to tell.
type Outbox struct {
	recorder Recorder
}

func NewOutbox(recorder Recorder) *Outbox {
	return &Outbox{
		recorder: recorder,
	}
}

func (o *Outbox) Published(ctx context.Context, p *post.Post) error {
	return o.add(ctx, federationv1.PostChange_POST_CHANGE_PUBLISHED, p)
}

func (o *Outbox) Edited(ctx context.Context, p *post.Post) error {
	return o.add(ctx, federationv1.PostChange_POST_CHANGE_EDITED, p)
}

func (o *Outbox) Deleted(ctx context.Context, p *post.Post) error {
	return o.add(ctx, federationv1.PostChange_POST_CHANGE_DELETED, p)
}

func (o *Outbox) add(ctx context.Context, change federationv1.PostChange, p *post.Post) error {
	payload, err := proto.Marshal(&federationv1.PostChangedRequest{
		Change:   change,
		PostId:   p.ID().String(),
		AuthorId: p.AuthorID().String(),
	})
	if err != nil {
		return errs.Wrap("proto.Marshal", err)
	}

	return errs.Wrap("o.recorder.Add", o.recorder.Add(ctx, kindPostChanged, payload))
}

// Key keeps the changes of one post in order, a deletion after the post,
// and lets those of different posts go at the same time.
func Key(m outbox.Message) string {
	var req federationv1.PostChangedRequest
	if proto.Unmarshal(m.Payload, &req) != nil {
		return ""
	}

	return req.GetPostId()
}

// Deliverer makes the recorded calls to the federation service.
type Deliverer struct {
	api     federationv1.FederationServiceClient
	timeout time.Duration
}

func NewDeliverer(api federationv1.FederationServiceClient, timeout time.Duration) *Deliverer {
	return &Deliverer{
		api:     api,
		timeout: timeout,
	}
}

// Deliver is an outbox.Deliver. Errors that a retry cannot fix are
// permanent: federation rejects the request itself.
func (d *Deliverer) Deliver(ctx context.Context, m outbox.Message) error {
	if m.Kind != kindPostChanged {
		return outbox.Permanent(fmt.Errorf("unknown kind %q", m.Kind))
	}

	var req federationv1.PostChangedRequest
	if err := proto.Unmarshal(m.Payload, &req); err != nil {
		return outbox.Permanent(errs.Wrap("proto.Unmarshal", err))
	}

	ctx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()

	_, err := d.api.PostChanged(ctx, &req)

	switch status.Code(err) {
	case codes.OK:
		return nil
	case codes.InvalidArgument, codes.Unimplemented:
		return outbox.Permanent(err)
	default:
		return err
	}
}

// Nop is the service.Federator when no federation service is configured.
type Nop struct{}

func (Nop) Published(context.Context, *post.Post) error { return nil }
func (Nop) Edited(context.Context, *post.Post) error    { return nil }
func (Nop) Deleted(context.Context, *post.Post) error   { return nil }
