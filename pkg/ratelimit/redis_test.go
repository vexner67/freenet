package ratelimit_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/vexner67/freenet/pkg/ratelimit"
)

// Integration test: TEST_REDIS_ADDR=localhost:6379 go test ./...
func TestRedis(t *testing.T) {
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("TEST_REDIS_ADDR is not set")
	}

	ctx := context.Background()

	limiter, err := ratelimit.Connect(ctx, addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = limiter.Close() })

	// A second client to look at the key from outside.
	client := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = client.Close() })
	key := "test:" + uuid.NewString()

	for i := 1; i <= 3; i++ {
		ok, err := limiter.Allow(ctx, key, 2, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if want := i <= 2; ok != want {
			t.Fatalf("event %d: allowed = %v, want %v", i, ok, want)
		}
	}

	ttl, err := client.PTTL(ctx, "rate_limit:"+key).Result()
	if err != nil || ttl <= 0 || ttl > time.Minute {
		t.Fatalf("window must not be extended past a minute, ttl = %v, %v", ttl, err)
	}

	ok, err := limiter.Allow(ctx, "test:"+uuid.NewString(), 2, time.Minute)
	if err != nil || !ok {
		t.Fatalf("another key must have its own window: %v, %v", ok, err)
	}
}
