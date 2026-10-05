// Package notifyclient delivers post events to the notification service.
// It is the only place in post that knows the notification proto contract.
package notifyclient

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	notificationv1 "github.com/vexner67/freenet/notification/api/notification/v1"
	"github.com/vexner67/freenet/post/internal/service"
)

var kinds = map[service.EventKind]notificationv1.Kind{
	service.EventLike:    notificationv1.Kind_KIND_LIKE,
	service.EventComment: notificationv1.Kind_KIND_COMMENT,
}

// Client is a best-effort service.Notifier: a failed call is logged and
// dropped, so the like or comment that caused it still succeeds.
type Client struct {
	api     notificationv1.NotificationServiceClient
	timeout time.Duration
	logger  *slog.Logger
}

func New(api notificationv1.NotificationServiceClient, timeout time.Duration, logger *slog.Logger) *Client {
	return &Client{
		api:     api,
		timeout: timeout,
		logger:  logger,
	}
}

func (c *Client) Notify(ctx context.Context, e service.Event) {
	c.call(ctx, "notify", func(ctx context.Context) error {
		_, err := c.api.Notify(ctx, &notificationv1.NotifyRequest{Event: toProtoEvent(e)})
		return err
	})
}

func (c *Client) Retract(ctx context.Context, e service.Event) {
	c.call(ctx, "retract", func(ctx context.Context) error {
		_, err := c.api.Retract(ctx, &notificationv1.RetractRequest{Event: toProtoEvent(e)})
		return err
	})
}

func (c *Client) ForgetPost(ctx context.Context, postID uuid.UUID) {
	c.call(ctx, "forget post", func(ctx context.Context) error {
		_, err := c.api.ForgetPost(ctx, &notificationv1.ForgetPostRequest{PostId: postID.String()})
		return err
	})
}

// call runs one RPC with its own timeout. It ignores the cancellation of
// ctx: the user may close the page right after the like is saved, and
// the author should still hear about it.
func (c *Client) call(ctx context.Context, op string, rpc func(context.Context) error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), c.timeout)
	defer cancel()

	if err := rpc(ctx); err != nil {
		c.logger.Warn("notification not delivered", "op", op, "error", err)
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

func (Nop) Notify(context.Context, service.Event)  {}
func (Nop) Retract(context.Context, service.Event) {}
func (Nop) ForgetPost(context.Context, uuid.UUID)  {}
