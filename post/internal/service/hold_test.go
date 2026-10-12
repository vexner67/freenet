package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/service"
)

// fakeHolder records what it was asked and fails with err.
type fakeHolder struct {
	held, released []uuid.UUID
	err            error
}

func (f *fakeHolder) Hold(_ context.Context, userID uuid.UUID) error {
	f.held = append(f.held, userID)
	return f.err
}

func (f *fakeHolder) Release(_ context.Context, userID uuid.UUID) error {
	f.released = append(f.released, userID)
	return f.err
}

func (f *fakeHolder) Forget(context.Context, uuid.UUID) error {
	return f.err
}

func TestHoldService(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ann := uuid.New()
	first, second := &fakeHolder{}, &fakeHolder{}
	holds := service.NewHoldService(fakeTx{}, first, second)

	if err := holds.HoldUser(ctx, ann); err != nil || len(first.held) != 1 || len(second.held) != 1 {
		t.Fatalf("HoldUser: %v, asked %v and %v", err, first.held, second.held)
	}
	if err := holds.ReleaseUser(ctx, ann); err != nil || len(first.released) != 1 || second.released[0] != ann {
		t.Fatalf("ReleaseUser: %v, asked %v and %v", err, first.released, second.released)
	}

	// One storage failing fails the whole, for the outbox to try again.
	first.err = errors.New("down")
	if err := holds.HoldUser(ctx, ann); !errors.Is(err, first.err) || len(second.held) != 1 {
		t.Fatalf("HoldUser with a storage down: %v, the second asked %v", err, second.held)
	}
	if err := holds.ReleaseUser(ctx, ann); !errors.Is(err, first.err) {
		t.Fatalf("ReleaseUser with a storage down: %v", err)
	}
}
