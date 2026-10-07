package outbox

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/vexner67/freenet/pkg/reqid"
)

const (
	batchSize = 100
	// pollEvery is how often the relay looks without being woken: for
	// postponed messages and for the leadership of another relay.
	pollEvery  = 5 * time.Second
	minBackoff = time.Second
	maxBackoff = 5 * time.Minute
)

// Relay delivers the messages of a queue until its context ends.
type Relay struct {
	queue   Queue
	deliver Deliver
	logger  *slog.Logger
	now     func() time.Time

	messages *prometheus.CounterVec
	lag      prometheus.Gauge
}

// NewRelay registers the relay's metrics in reg.
func NewRelay(queue Queue, deliver Deliver, reg prometheus.Registerer, logger *slog.Logger) (*Relay, error) {
	r := &Relay{
		queue:   queue,
		deliver: deliver,
		logger:  logger,
		now:     time.Now,
		messages: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "outbox_messages_total",
			Help: "Outbox delivery attempts: delivered, retried later or dropped.",
		}, []string{"result"}),
		lag: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "outbox_lag_seconds",
			Help: "Age of the oldest undelivered outbox message; 0 when all are delivered.",
		}),
	}

	for _, c := range []prometheus.Collector{r.messages, r.lag} {
		if err := reg.Register(c); err != nil {
			return nil, fmt.Errorf("reg.Register: %w", err)
		}
	}

	return r, nil
}

// Run delivers until ctx is cancelled.
func (r *Relay) Run(ctx context.Context) error {
	for ctx.Err() == nil {
		r.deliverDue(ctx)

		if err := r.queue.Wait(ctx, pollEvery); err != nil && ctx.Err() == nil {
			r.logger.WarnContext(ctx, "outbox unavailable", "error", err)
			sleep(ctx, pollEvery)
		}
	}

	return nil
}

// deliverDue delivers messages in order until one is not due or fails.
func (r *Relay) deliverDue(ctx context.Context) {
	for {
		messages, err := r.queue.Head(ctx, batchSize)
		if err != nil {
			if ctx.Err() == nil {
				r.logger.WarnContext(ctx, "outbox head", "error", err)
			}
			return
		}

		r.lag.Set(0)
		if len(messages) > 0 {
			r.lag.Set(r.now().Sub(messages[0].CreatedAt).Seconds())
		}

		for _, m := range messages {
			if r.now().Before(m.DueAt) || !r.deliverOne(ctx, m) {
				return
			}
		}

		if len(messages) < batchSize {
			return
		}
	}
}

// deliverOne reports whether the messages after m may go.
func (r *Relay) deliverOne(ctx context.Context, m Message) bool {
	mctx := ctx
	if m.RequestID != "" {
		mctx = reqid.ContextWithID(ctx, m.RequestID)
	}

	err := r.deliver(mctx, m)

	switch {
	case err == nil:
		r.messages.WithLabelValues("delivered").Inc()
	case IsPermanent(err):
		r.messages.WithLabelValues("dropped").Inc()
		r.logger.ErrorContext(mctx, "outbox message dropped", "kind", m.Kind, "outbox_id", m.ID, "error", err)
	default:
		r.messages.WithLabelValues("retried").Inc()
		attempts := m.Attempts + 1
		r.logger.WarnContext(mctx, "outbox message not delivered", "kind", m.Kind, "outbox_id", m.ID, "attempts", attempts, "error", err)

		if err = r.queue.Postpone(ctx, m.ID, attempts, r.now().Add(backoff(attempts))); err != nil && ctx.Err() == nil {
			r.logger.WarnContext(mctx, "outbox postpone", "error", err)
		}

		return false
	}

	// Delivered or dropped. If the delete fails, the message goes again:
	// delivery is at least once.
	if err = r.queue.Delete(ctx, m.ID); err != nil {
		if ctx.Err() == nil {
			r.logger.WarnContext(mctx, "outbox delete", "error", err)
		}
		return false
	}

	return true
}

// backoff doubles from minBackoff up to maxBackoff.
func backoff(attempts int) time.Duration {
	d := minBackoff
	for i := 1; i < attempts && d < maxBackoff; i++ {
		d *= 2
	}

	return min(d, maxBackoff)
}

func sleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()

	select {
	case <-ctx.Done():
	case <-t.C:
	}
}
