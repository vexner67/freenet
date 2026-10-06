package ratelimit

import (
	"context"
	"testing"
	"time"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }
func allow(t *testing.T, l *Memory, key string) bool {
	t.Helper()

	ok, err := l.Allow(context.Background(), key, 2, time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	return ok
}

func TestMemory(t *testing.T) {
	t.Parallel()

	c := &clock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	l := NewMemory()
	l.now = c.now

	for i := 1; i <= 2; i++ {
		if !allow(t, l, "a") {
			t.Fatalf("event %d must pass", i)
		}
	}
	if allow(t, l, "a") {
		t.Fatal("third event in the window must be limited")
	}

	c.advance(time.Minute)
	if !allow(t, l, "a") {
		t.Fatal("a new window must start after the period")
	}

	allow(t, l, "b")
	c.advance(2 * time.Minute)
	allow(t, l, "c")

	if _, ok := l.windows["b"]; ok {
		t.Fatal("expired windows must be swept")
	}
	if len(l.windows) != 1 {
		t.Fatalf("windows = %d, want only the fresh one", len(l.windows))
	}
}
