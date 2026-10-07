package pubsub

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	publishTimeout = 2 * time.Second
	retryEvery     = time.Second
)

// Codec turns an event for a key into a Redis message and back.
type Codec[K comparable, T any] interface {
	Encode(key K, event T) ([]byte, error)
	Decode(data []byte) (K, T, error)
}

// Redis is a Hub shared by the replicas of a service: Publish goes
// through a Redis channel, and Run hands what comes from it to this
// replica's subscribers, so a stream gets the events published on any
// replica. Redis pub/sub keeps nothing, so whenever this replica is cut
// off from the channel, events may be lost: then it drops its
// subscribers and turns new ones away, and they catch up from storage
// and subscribe again once it is back, as with a subscriber that falls
// behind.
type Redis[K comparable, T any] struct {
	hub     *Hub[K, T]
	client  *redis.Client
	channel string
	codec   Codec[K, T]
	logger  *slog.Logger
	// listening is whether Run receives from the channel right now.
	listening atomic.Bool
}

// NewRedis makes a hub over channel whose subscribers may lag behind by
// buffer events. Events reach subscribers only while Run runs.
func NewRedis[K comparable, T any](client *redis.Client, channel string, codec Codec[K, T], buffer int, logger *slog.Logger) *Redis[K, T] {
	return &Redis[K, T]{
		hub:     New[K, T](buffer),
		client:  client,
		channel: channel,
		codec:   codec,
		logger:  logger,
	}
}

// Subscribe is Hub.Subscribe, but while cut off from Redis the channel
// comes closed: the subscriber is told at once to try again later.
func (r *Redis[K, T]) Subscribe(key K) (<-chan T, func()) {
	if !r.listening.Load() {
		closed := make(chan T)
		close(closed)
		return closed, func() {}
	}

	return r.hub.Subscribe(key)
}

// Publish sends event to the subscribers of key on every replica. It
// outlives the cancellation of ctx: the change it tells about is done.
func (r *Redis[K, T]) Publish(ctx context.Context, key K, event T) {
	data, err := r.codec.Encode(key, event)
	if err != nil {
		r.logger.ErrorContext(ctx, "live event not encoded", "channel", r.channel, "error", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), publishTimeout)
	defer cancel()

	// Subscribers who miss it are on replicas cut off from Redis too,
	// which drop them.
	if err = r.client.Publish(ctx, r.channel, data).Err(); err != nil {
		r.logger.WarnContext(ctx, "live event not published", "channel", r.channel, "error", err)
	}
}

// Run receives from the channel until ctx is cancelled, reconnecting
// when cut off.
func (r *Redis[K, T]) Run(ctx context.Context) error {
	sub := r.client.Subscribe(ctx, r.channel)

	// Receive blocks in a read that ignores ctx; closing ends it.
	stop := context.AfterFunc(ctx, func() { _ = sub.Close() })
	defer func() {
		if stop() {
			_ = sub.Close()
		}
	}()

	defer r.cutOff()

	for {
		msg, err := sub.Receive(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}

			if r.listening.Load() {
				r.logger.WarnContext(ctx, "live events cut off from Redis", "channel", r.channel, "error", err)
			}
			r.cutOff()
			sleep(ctx, retryEvery) // the next Receive reconnects
			continue
		}

		switch m := msg.(type) {
		case *redis.Subscription:
			// Also after a reconnect that Receive did not report: what
			// was published meanwhile is lost.
			r.cutOff()
			r.listening.Store(true)
			r.logger.InfoContext(ctx, "live events from Redis", "channel", r.channel)
		case *redis.Message:
			key, event, err := r.codec.Decode([]byte(m.Payload))
			if err != nil {
				r.logger.ErrorContext(ctx, "live event not decoded", "channel", r.channel, "error", err)
				continue
			}

			r.hub.Publish(ctx, key, event)
		}
	}
}

// cutOff turns new subscribers away and drops the current ones.
func (r *Redis[K, T]) cutOff() {
	r.listening.Store(false)
	r.hub.Close()
}

func sleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()

	select {
	case <-ctx.Done():
	case <-t.C:
	}
}
