package pubsub_test

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/vexner67/freenet/pkg/pubsub"
)

// codec sends "key=event".
type codec struct{}

func (codec) Encode(key, event string) ([]byte, error) {
	return []byte(key + "=" + event), nil
}

func (codec) Decode(data []byte) (string, string, error) {
	key, event, _ := strings.Cut(string(data), "=")
	return key, event, nil
}

func newClient(t *testing.T) *redis.Client {
	t.Helper()

	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("TEST_REDIS_ADDR is not set")
	}

	client := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = client.Close() })

	return client
}

// replica runs a Redis hub over channel until the test ends.
func replica(t *testing.T, client *redis.Client, channel string) (*pubsub.Redis[string, string], context.CancelFunc) {
	t.Helper()

	r := pubsub.NewRedis[string, string](client, channel, codec{}, 8, slog.New(slog.DiscardHandler))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = r.Run(ctx)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	return r, cancel
}

// subscribe retries until the replica listens.
func subscribe(t *testing.T, r *pubsub.Redis[string, string], key string) (<-chan string, func()) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for {
		ch, cancel := r.Subscribe(key)
		select {
		case _, ok := <-ch:
			if ok {
				t.Fatal("an event before any was published")
			}
		default:
			return ch, cancel
		}

		if time.Now().After(deadline) {
			t.Fatal("the replica does not listen to Redis")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func receive(t *testing.T, ch <-chan string) (string, bool) {
	t.Helper()

	select {
	case e, ok := <-ch:
		return e, ok
	case <-time.After(5 * time.Second):
		t.Fatal("no event")
		return "", false
	}
}

func TestRedis_AcrossReplicas(t *testing.T) {
	client := newClient(t)
	channel := "test:" + uuid.NewString()

	a, _ := replica(t, client, channel)
	b, _ := replica(t, client, channel)

	ann, cancel := subscribe(t, b, "ann")
	defer cancel()
	bob, cancelBob := subscribe(t, b, "bob")
	defer cancelBob()

	a.Publish(context.Background(), "ann", "hello")

	if e, ok := receive(t, ann); !ok || e != "hello" {
		t.Fatalf("ann got %q, %v; want hello from the other replica", e, ok)
	}

	select {
	case e := <-bob:
		t.Fatalf("bob got %q, meant for ann", e)
	default:
	}
}

// A replica cut off from Redis drops its subscribers, who may have missed
// events, and takes them again once back.
func TestRedis_CutOffDropsSubscribers(t *testing.T) {
	client := newClient(t)
	channel := "test:" + uuid.NewString()

	r, _ := replica(t, client, channel)
	ann, cancel := subscribe(t, r, "ann")
	defer cancel()

	// Close the replica's subscription connection from the Redis side.
	if err := client.Do(context.Background(), "CLIENT", "KILL", "TYPE", "pubsub").Err(); err != nil {
		t.Fatalf("CLIENT KILL: %v", err)
	}

	if _, ok := receive(t, ann); ok {
		t.Fatal("the subscriber was kept across the cut")
	}

	again, cancelAgain := subscribe(t, r, "ann")
	defer cancelAgain()

	r.Publish(context.Background(), "ann", "back")
	if e, ok := receive(t, again); !ok || e != "back" {
		t.Fatalf("after the reconnect got %q, %v; want back", e, ok)
	}
}

func TestRedis_StoppedTurnsSubscribersAway(t *testing.T) {
	client := newClient(t)

	r, stop := replica(t, client, "test:"+uuid.NewString())
	ann, cancel := subscribe(t, r, "ann")
	defer cancel()

	stop()

	if _, ok := receive(t, ann); ok {
		t.Fatal("the subscriber was kept after Run stopped")
	}

	ch, _ := r.Subscribe("ann")
	if _, ok := <-ch; ok {
		t.Fatal("a new subscriber was taken while not listening")
	}
}
