package outbox

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/vexner67/freenet/pkg/reqid"
)

// receiver records deliveries and fails the kinds told to.
type receiver struct {
	mu        sync.Mutex
	got       []string
	requestID []string
	fail      map[string]error
}

func (r *receiver) deliver(ctx context.Context, m Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.fail[m.Kind]; err != nil {
		return err
	}

	r.got = append(r.got, m.Kind)
	r.requestID = append(r.requestID, reqid.FromContext(ctx))

	return nil
}

func (r *receiver) delivered() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.got)
}

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newRelay(t *testing.T, q Queue, r *receiver, c *clock) *Relay {
	t.Helper()

	relay, err := NewRelay(q, r.deliver, prometheus.NewRegistry(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("NewRelay: %v", err)
	}
	if c != nil {
		relay.now = c.now
	}

	return relay
}

func add(t *testing.T, q *Memory, ctx context.Context, kinds ...string) {
	t.Helper()

	for _, k := range kinds {
		if err := q.Add(ctx, k, []byte(k)); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}
}

func TestRelay_InOrderWithRequestID(t *testing.T) {
	t.Parallel()

	c := &clock{t: time.Now()}
	q := NewMemory()
	q.now = c.now
	r := &receiver{}
	relay := newRelay(t, q, r, c)

	add(t, q, reqid.ContextWithID(context.Background(), "req-1"), "like", "unlike")
	add(t, q, context.Background(), "follow")

	relay.deliverDue(context.Background())

	if got := r.delivered(); !slices.Equal(got, []string{"like", "unlike", "follow"}) {
		t.Errorf("delivered %v, want like, unlike, follow", got)
	}
	if !slices.Equal(r.requestID, []string{"req-1", "req-1", ""}) {
		t.Errorf("request IDs %q, want the recorded ones", r.requestID)
	}
	if left, _ := q.Head(context.Background(), batchSize); len(left) != 0 {
		t.Errorf("%d messages left, want none", len(left))
	}
	if got := testutil.ToFloat64(relay.messages.WithLabelValues("delivered")); got != 3 {
		t.Errorf("delivered metric = %v, want 3", got)
	}
}

// A failed message is retried later and holds back those after it, so an
// undo never overtakes what it undoes.
func TestRelay_RetryHoldsBackLaterMessages(t *testing.T) {
	t.Parallel()

	c := &clock{t: time.Now()}
	q := NewMemory()
	q.now = c.now
	r := &receiver{fail: map[string]error{"like": errors.New("unavailable")}}
	relay := newRelay(t, q, r, c)

	add(t, q, context.Background(), "like", "unlike")

	relay.deliverDue(context.Background())

	if got := r.delivered(); len(got) != 0 {
		t.Fatalf("delivered %v while the first fails, want nothing", got)
	}

	head, _ := q.Head(context.Background(), batchSize)
	if head[0].Attempts != 1 || !head[0].DueAt.Equal(c.t.Add(minBackoff)) {
		t.Fatalf("failed message: attempts %d due %v, want 1 and in %v", head[0].Attempts, head[0].DueAt.Sub(c.t), minBackoff)
	}

	// Not due yet: nothing happens even when the receiver is back.
	r.mu.Lock()
	r.fail = nil
	r.mu.Unlock()
	relay.deliverDue(context.Background())
	if got := r.delivered(); len(got) != 0 {
		t.Fatalf("delivered %v before the retry is due", got)
	}

	c.t = c.t.Add(minBackoff)
	relay.deliverDue(context.Background())
	if got := r.delivered(); !slices.Equal(got, []string{"like", "unlike"}) {
		t.Errorf("delivered %v, want like then unlike", got)
	}
}

func TestRelay_DropsPermanentFailures(t *testing.T) {
	t.Parallel()

	q := NewMemory()
	r := &receiver{fail: map[string]error{"bad": Permanent(errors.New("invalid"))}}
	relay := newRelay(t, q, r, nil)

	add(t, q, context.Background(), "bad", "good")

	relay.deliverDue(context.Background())

	if got := r.delivered(); !slices.Equal(got, []string{"good"}) {
		t.Errorf("delivered %v, want only good", got)
	}
	if left, _ := q.Head(context.Background(), batchSize); len(left) != 0 {
		t.Errorf("%d messages left, want the bad one dropped", len(left))
	}
	if got := testutil.ToFloat64(relay.messages.WithLabelValues("dropped")); got != 1 {
		t.Errorf("dropped metric = %v, want 1", got)
	}
}

func TestRelay_MoreThanABatch(t *testing.T) {
	t.Parallel()

	q := NewMemory()
	r := &receiver{}
	relay := newRelay(t, q, r, nil)

	for range batchSize + 5 {
		add(t, q, context.Background(), "m")
	}

	relay.deliverDue(context.Background())

	if got := len(r.delivered()); got != batchSize+5 {
		t.Errorf("delivered %d, want %d", got, batchSize+5)
	}
}

func TestRelay_RunWakesUpAndStops(t *testing.T) {
	t.Parallel()

	q := NewMemory()
	r := &receiver{}
	relay := newRelay(t, q, r, nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- relay.Run(ctx) }()

	add(t, q, context.Background(), "like")

	// Woken by Add, long before the poll.
	deadline := time.Now().Add(pollEvery / 2)
	for len(r.delivered()) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("not delivered: the relay was not woken")
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run = %v, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not stop")
	}
}

func TestBackoff(t *testing.T) {
	t.Parallel()

	for attempts, want := range map[int]time.Duration{
		1:  time.Second,
		2:  2 * time.Second,
		5:  16 * time.Second,
		9:  256 * time.Second,
		10: maxBackoff,
		50: maxBackoff,
	} {
		if got := backoff(attempts); got != want {
			t.Errorf("backoff(%d) = %v, want %v", attempts, got, want)
		}
	}
}
