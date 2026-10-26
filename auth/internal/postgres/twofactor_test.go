package postgres_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/auth/internal/postgres"
	"github.com/vexner67/freenet/auth/internal/seal"
	"github.com/vexner67/freenet/auth/internal/twofactor"
	"github.com/vexner67/freenet/pkg/pgpool"
)

func newTwoFactorRepo(t *testing.T) *postgres.TwoFactorRepository {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgpool.New: %v", err)
	}
	t.Cleanup(pool.Close)

	box, err := seal.New("test-secret", "two-factor")
	if err != nil {
		t.Fatal(err)
	}

	return postgres.NewTwoFactorRepository(pool, box)
}

func TestTwoFactorRepository(t *testing.T) {
	repo := newTwoFactorRepo(t)
	ctx := context.Background()
	user := uuid.New()

	if _, err := repo.Get(ctx, user); !errors.Is(err, twofactor.ErrNotSetUp) {
		t.Fatalf("Get of none: %v", err)
	}
	if err := repo.Enable(ctx, user, 1, nil); !errors.Is(err, twofactor.ErrNotSetUp) {
		t.Fatalf("Enable of none: %v", err)
	}

	first, _ := twofactor.NewSecret()
	if err := repo.SetUp(ctx, twofactor.New(user, first)); err != nil {
		t.Fatalf("SetUp: %v", err)
	}

	// Set up again before enabling: the new secret replaces the first.
	secret, _ := twofactor.NewSecret()
	if err := repo.SetUp(ctx, twofactor.New(user, secret)); err != nil {
		t.Fatalf("SetUp again: %v", err)
	}

	got, err := repo.Get(ctx, user)
	if err != nil || got.Enabled() || string(got.Secret()) != string(secret) {
		t.Fatalf("Get = %+v, %v", got, err)
	}

	if err = repo.UseStep(ctx, user, 5); !errors.Is(err, twofactor.ErrInvalidCode) {
		t.Fatalf("UseStep before Enable: %v", err)
	}

	if err = repo.Enable(ctx, user, 10, []string{"a", "b", "c"}); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	if err = repo.Enable(ctx, user, 11, nil); !errors.Is(err, twofactor.ErrInvalidCode) {
		t.Fatalf("Enable twice: %v", err)
	}
	if err = repo.SetUp(ctx, twofactor.New(user, first)); !errors.Is(err, twofactor.ErrEnabled) {
		t.Fatalf("SetUp of an enabled: %v", err)
	}

	// Each code works once.
	if err = repo.UseStep(ctx, user, 10); !errors.Is(err, twofactor.ErrInvalidCode) {
		t.Fatalf("UseStep of the step of Enable: %v", err)
	}
	if err = repo.UseStep(ctx, user, 11); err != nil {
		t.Fatalf("UseStep: %v", err)
	}
	if err = repo.UseRecovery(ctx, user, "b"); err != nil {
		t.Fatalf("UseRecovery: %v", err)
	}
	if err = repo.UseRecovery(ctx, user, "b"); !errors.Is(err, twofactor.ErrInvalidCode) {
		t.Fatalf("UseRecovery twice: %v", err)
	}

	if got, err = repo.Get(ctx, user); err != nil || !got.Enabled() || got.LastStep() != 11 || got.RecoveryLeft() != 2 {
		t.Fatalf("Get after use = %+v, %v", got, err)
	}

	if err = repo.ReplaceRecovery(ctx, user, 11, []string{"d"}); !errors.Is(err, twofactor.ErrInvalidCode) {
		t.Fatalf("ReplaceRecovery with a used step: %v", err)
	}
	if err = repo.ReplaceRecovery(ctx, user, 12, []string{"d"}); err != nil {
		t.Fatalf("ReplaceRecovery: %v", err)
	}
	if err = repo.UseRecovery(ctx, user, "a"); !errors.Is(err, twofactor.ErrInvalidCode) {
		t.Fatalf("UseRecovery of a replaced code: %v", err)
	}

	if err = repo.Delete(ctx, user); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err = repo.Delete(ctx, user); !errors.Is(err, twofactor.ErrNotSetUp) {
		t.Fatalf("Delete twice: %v", err)
	}
}
