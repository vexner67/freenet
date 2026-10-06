package redis_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"github.com/vexner67/freenet/auth/internal/account"
	"github.com/vexner67/freenet/auth/internal/redis"
	"github.com/vexner67/freenet/auth/internal/service"
)

// Integration test: TEST_REDIS_ADDR=localhost:6379 go test ./internal/redis
func newClient(t *testing.T) *goredis.Client {
	t.Helper()

	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("TEST_REDIS_ADDR is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := redis.New(ctx, addr)
	if err != nil {
		t.Fatalf("redis.New: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	return client
}

func randomEmail() account.Email {
	return account.Email(uuid.NewString()[:8] + "@example.com")
}

func TestCodeRepository(t *testing.T) {
	repo := redis.NewCodeRepository(newClient(t))
	ctx := context.Background()
	email := randomEmail()

	if _, err := repo.IncrementAttempts(ctx, email); !errors.Is(err, service.ErrCodeNotFound) {
		t.Fatalf("attempts without a code: got %v, want ErrCodeNotFound", err)
	}

	if err := repo.Save(ctx, email, "hash-1", time.Minute); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if hash, err := repo.Get(ctx, email); err != nil || hash != "hash-1" {
		t.Fatalf("Get = %q, %v", hash, err)
	}

	for want := 1; want <= 3; want++ {
		got, err := repo.IncrementAttempts(ctx, email)
		if err != nil || got != want {
			t.Fatalf("IncrementAttempts = %d, %v; want %d", got, err, want)
		}
	}

	if err := repo.Save(ctx, email, "hash-2", time.Minute); err != nil {
		t.Fatal(err)
	}
	if got, err := repo.IncrementAttempts(ctx, email); err != nil || got != 1 {
		t.Fatalf("a new code must reset attempts: %d, %v", got, err)
	}

	if err := repo.Delete(ctx, email); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := repo.Delete(ctx, email); !errors.Is(err, service.ErrCodeNotFound) {
		t.Fatalf("second Delete: got %v, want ErrCodeNotFound", err)
	}
	if _, err := repo.Get(ctx, email); !errors.Is(err, service.ErrCodeNotFound) {
		t.Fatalf("Get after Delete: got %v, want ErrCodeNotFound", err)
	}
}

func TestCodeRepository_Expiry(t *testing.T) {
	client := newClient(t)
	repo := redis.NewCodeRepository(client)
	ctx := context.Background()
	email := randomEmail()

	if err := repo.Save(ctx, email, "hash", time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.IncrementAttempts(ctx, email); err != nil {
		t.Fatal(err)
	}

	ttl, err := client.PTTL(ctx, "verification_attempts:"+email.String()).Result()
	if err != nil || ttl <= 0 || ttl > time.Minute {
		t.Fatalf("attempts must expire with the code, ttl = %v, %v", ttl, err)
	}
}
