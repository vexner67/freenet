package service_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/inmem"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestIgnoreService(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := service.NewIgnoreService(inmem.NewIgnoreRepository(inmem.NewUserRepository()))
	ann, bob, club := uuid.New(), uuid.New(), uuid.New()

	if err := svc.Ignore(ctx, ann, ann); !errors.Is(err, user.ErrIgnoreSelf) {
		t.Fatalf("ignoring oneself = %v", err)
	}

	for _, source := range []uuid.UUID{bob, club, bob} {
		if err := svc.Ignore(ctx, ann, source); err != nil {
			t.Fatalf("Ignore: %v", err)
		}
	}

	if got, err := svc.Ignored(ctx, ann); err != nil || !slices.Equal(got, []uuid.UUID{club, bob}) {
		t.Fatalf("Ignored = %v, %v; want the community, then bob", got, err)
	}
	if got, _ := svc.Ignored(ctx, bob); len(got) != 0 {
		t.Fatalf("bob ignores %v", got)
	}

	if err := svc.Unignore(ctx, ann, []uuid.UUID{bob, uuid.New()}); err != nil {
		t.Fatalf("Unignore: %v", err)
	}
	if err := svc.Unignore(ctx, ann, nil); err != nil {
		t.Fatalf("Unignore nothing: %v", err)
	}
	if got, _ := svc.Ignored(ctx, ann); !slices.Equal(got, []uuid.UUID{club}) {
		t.Fatalf("Ignored after Unignore = %v", got)
	}
}

func TestIgnoreService_Limit(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := service.NewIgnoreService(inmem.NewIgnoreRepository(inmem.NewUserRepository()))
	ann, first := uuid.New(), uuid.New()

	if err := svc.Ignore(ctx, ann, first); err != nil {
		t.Fatal(err)
	}
	for range user.MaxIgnoredSources - 1 {
		if err := svc.Ignore(ctx, ann, uuid.New()); err != nil {
			t.Fatal(err)
		}
	}

	if err := svc.Ignore(ctx, ann, uuid.New()); !errors.Is(err, user.ErrTooManyIgnored) {
		t.Fatalf("one over the limit = %v", err)
	}
	// What is ignored already takes no new place.
	if err := svc.Ignore(ctx, ann, first); err != nil {
		t.Fatalf("ignoring again at the limit = %v", err)
	}

	if err := svc.Unignore(ctx, ann, []uuid.UUID{first}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Ignore(ctx, ann, uuid.New()); err != nil {
		t.Fatalf("a place was freed: %v", err)
	}
}
