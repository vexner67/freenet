// Package notifyclient tells the notification service of new messages, to
// push them to the browsers of their recipients. It is the only place in
// message that knows the notification proto contract.
package notifyclient

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/message/internal/service"
	notificationv1 "github.com/vexner67/freenet/notification/api/notification/v1"
)

// Pusher is the service.Pusher: a push waits in a queue, so the send does
// not wait for it, and Run hands them over one by one, each within
// timeout. Best effort: a full queue and a failed call drop the push.
type Pusher struct {
	api     notificationv1.NotificationServiceClient
	timeout time.Duration
	queue   chan job
	logger  *slog.Logger
}

// job is a push to tell. ctx is of the request that sent the message,
// without its end: its ID goes with the call and into the logs.
type job struct {
	ctx context.Context //nolint:containedctx // the request's values outlive it in the queue
	req *notificationv1.PushMessageRequest
}

func New(api notificationv1.NotificationServiceClient, timeout time.Duration, size int, logger *slog.Logger) *Pusher {
	return &Pusher{
		api:     api,
		timeout: timeout,
		queue:   make(chan job, size),
		logger:  logger,
	}
}

func (p *Pusher) Push(ctx context.Context, m service.Pushed) {
	req := &notificationv1.PushMessageRequest{
		MessageId:    m.ID.String(),
		SenderId:     m.SenderID.String(),
		RecipientIds: make([]string, len(m.RecipientIDs)),
		ChatTitle:    m.ChatTitle,
	}

	for i, id := range m.RecipientIDs {
		req.RecipientIds[i] = id.String()
	}

	if m.ChatID != uuid.Nil {
		req.ChatId = m.ChatID.String()
	}

	select {
	case p.queue <- job{ctx: context.WithoutCancel(ctx), req: req}:
	default:
		p.logger.WarnContext(ctx, "push queue is full, push dropped", "message_id", m.ID)
	}
}

// Run tells the notification service of the queued pushes until ctx ends.
func (p *Pusher) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case j := <-p.queue:
			p.tell(j)
		}
	}
}

func (p *Pusher) tell(j job) {
	ctx, cancel := context.WithTimeout(j.ctx, p.timeout)
	defer cancel()

	if _, err := p.api.PushMessage(ctx, j.req); err != nil {
		p.logger.WarnContext(ctx, "push a message", "error", err, "message_id", j.req.GetMessageId())
	}
}
