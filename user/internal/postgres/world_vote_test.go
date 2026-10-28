package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/postgres"
)

func TestWorldVoteRepository(t *testing.T) {
	_, pool := newRepo(t)
	ops := postgres.NewWorldVoteRepository(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	op, userID := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	if added, err := ops.AddWorldVote(ctx, op, userID, -50); err != nil || !added {
		t.Fatalf("AddWorldVote = %v, %v", added, err)
	}
	if added, err := ops.AddWorldVote(ctx, op, userID, -50); err != nil || added {
		t.Fatalf("AddWorldVote again = %v, %v", added, err)
	}
}
