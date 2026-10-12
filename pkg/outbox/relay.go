package outbox

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
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
	// lanes is how many deliveries a relay told the keys of its messages
	// makes at a time.
	lanes = 8
)

// Key names what a message is about. Messages of one key go in the order
// they were recorded; messages of different keys do not wait for each
// other. A message with no key ("") is about everything: it goes after
// all the messages before it and before all those after it.
type Key func(m Message) string

// Option changes how a relay delivers.
type Option func(r *Relay)

// Keyed lets the relay deliver the messages of different keys at the same
// time. Without it every message waits for the one before.
func Keyed(key Key) Option {
	return func(r *Relay) {
		r.key = key
	}
}

// Relay delivers the messages of a queue until its context ends.
type Relay struct {
	queue   Queue
	deliver Deliver
	key     Key
	logger  *slog.Logger
	now     func() time.Time

	messages *prometheus.CounterVec
	lag      prometheus.Gauge
}

// NewRelay registers the relay's metrics in reg.
func NewRelay(queue Queue, deliver Deliver, reg prometheus.Registerer, logger *slog.Logger, opts ...Option) (*Relay, error) {
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

	for _, opt := range opts {
		opt(r)
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

// deliverDue delivers the messages that are due until one that holds
// back all the others is not due or fails, or nothing more goes.
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

		if !r.deliverBatch(ctx, messages) || len(messages) < batchSize {
			return
		}
	}
}

// deliverBatch reports whether every message of the batch went. The
// messages between two that have no key go lane by lane, a lane being the
// messages of one key in their order.
func (r *Relay) deliverBatch(ctx context.Context, messages []Message) bool {
	var (
		order  []string
		byLane = map[string][]Message{}
	)

	for _, m := range messages {
		key := ""
		if r.key != nil {
			key = r.key(m)
		}

		if key != "" {
			if _, ok := byLane[key]; !ok {
				order = append(order, key)
			}

			byLane[key] = append(byLane[key], m)

			continue
		}

		if !r.deliverLanes(ctx, order, byLane) || !r.deliverLane(ctx, []Message{m}) {
			return false
		}

		order, byLane = nil, map[string][]Message{}
	}

	return r.deliverLanes(ctx, order, byLane)
}

// deliverLanes delivers the lanes at the same time, a few at once, and
// reports whether all of them went whole. A lane that stops holds back
// only itself: the others go on.
func (r *Relay) deliverLanes(ctx context.Context, order []string, byLane map[string][]Message) bool {
	var (
		wg      sync.WaitGroup
		stopped atomic.Bool
		free    = make(chan struct{}, lanes)
	)

	for _, key := range order {
		free <- struct{}{}

		wg.Add(1)

		go func() {
			defer wg.Done()
			defer func() { <-free }()

			if !r.deliverLane(ctx, byLane[key]) {
				stopped.Store(true)
			}
		}()
	}

	wg.Wait()

	return !stopped.Load()
}

// deliverLane delivers messages in order until one is not due or fails.
func (r *Relay) deliverLane(ctx context.Context, messages []Message) bool {
	for _, m := range messages {
		if r.now().Before(m.DueAt) || !r.deliverOne(ctx, m) {
			return false
		}
	}

	return true
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
