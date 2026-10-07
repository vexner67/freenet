// Package notifyclient delivers follows to the notification service. It
// is the only place in user that knows the notification proto contract.
package notifyclient

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	notificationv1 "github.com/vexner67/freenet/notification/api/notification/v1"
)

// Client is a best-effort service.Notifier: a failed call is logged and
// dropped, so the follow that caused it still succeeds.
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

func (c *Client) Followed(ctx context.Context, followerID, followeeID uuid.UUID) {
	c.call(ctx, "notify", func(ctx context.Context) error {
		_, err := c.api.Notify(ctx, &notificationv1.NotifyRequest{Event: followEvent(followerID, followeeID)})
		return err
	})
}

func (c *Client) Unfollowed(ctx context.Context, followerID, followeeID uuid.UUID) {
	c.call(ctx, "retract", func(ctx context.Context) error {
		_, err := c.api.Retract(ctx, &notificationv1.RetractRequest{Event: followEvent(followerID, followeeID)})
		return err
	})
}

// call runs one RPC with its own timeout. It ignores the cancellation of
// ctx: the follow is already saved when the request may be gone.
func (c *Client) call(ctx context.Context, op string, rpc func(context.Context) error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), c.timeout)
	defer cancel()

	if err := rpc(ctx); err != nil {
		c.logger.WarnContext(ctx, "notification not delivered", "op", op, "error", err)
	}
}

func followEvent(followerID, followeeID uuid.UUID) *notificationv1.Event {
	return &notificationv1.Event{
		Kind:        notificationv1.Kind_KIND_FOLLOW,
		RecipientId: followeeID.String(),
		ActorId:     followerID.String(),
	}
}

// Nop is the Notifier when no notification service is configured.
type Nop struct{}

func (Nop) Followed(context.Context, uuid.UUID, uuid.UUID)   {}
func (Nop) Unfollowed(context.Context, uuid.UUID, uuid.UUID) {}
