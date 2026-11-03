// Package notifyclient tells the notification service of new messages, to
// push them to the browsers of their recipients, and of calls. It is the
// only place in message that knows the notification proto contract.
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

// job is a push or a notification to tell. ctx is of the request that
// sent the message, without its end: its ID goes with the call and into
// the logs.
type job struct {
	ctx    context.Context //nolint:containedctx // the request's values outlive it in the queue
	push   *notificationv1.PushMessageRequest
	notify *notificationv1.NotifyRequest
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

	p.enqueue(ctx, job{push: req})
}

// Called tells notification of a call: a missed one to the peer, or one
// they talked in to both of them, each naming the other.
func (p *Pusher) Called(ctx context.Context, c service.Call) {
	caller, peer := c.CallerID.String(), c.PeerID.String()
	if c.Seconds == 0 {
		p.enqueue(ctx, job{notify: &notificationv1.NotifyRequest{Event: &notificationv1.Event{
			Kind: notificationv1.Kind_KIND_MISSED_CALL, RecipientId: peer, ActorId: caller,
		}}})

		return
	}

	seconds := int32(c.Seconds) //nolint:gosec // at most message.MaxCallSeconds
	for _, pair := range [][2]string{{peer, caller}, {caller, peer}} {
		p.enqueue(ctx, job{notify: &notificationv1.NotifyRequest{Event: &notificationv1.Event{
			Kind: notificationv1.Kind_KIND_CALL, RecipientId: pair[0], ActorId: pair[1], Seconds: seconds,
		}}})
	}
}

func (p *Pusher) enqueue(ctx context.Context, j job) {
	j.ctx = context.WithoutCancel(ctx)
	select {
	case p.queue <- j:
	default:
		p.logger.WarnContext(ctx, "push queue is full, push dropped")
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

	if j.notify != nil {
		if _, err := p.api.Notify(ctx, j.notify); err != nil {
			p.logger.WarnContext(ctx, "notify of a call", "error", err)
		}

		return
	}

	if _, err := p.api.PushMessage(ctx, j.push); err != nil {
		p.logger.WarnContext(ctx, "push a message", "error", err, "message_id", j.push.GetMessageId())
	}
}
