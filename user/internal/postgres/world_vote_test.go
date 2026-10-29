package postgres_test

import (
	"context"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/postgres"
	"github.com/vexner67/freenet/user/internal/user"
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

// The votes of the world are written down in the ledger as any other:
// its kind is one the table takes.
func TestWorldVotesInTheLedger(t *testing.T) {
	repo, pool := newRepo(t)
	wallets := postgres.NewWalletRepository(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ann := savedUser(t, repo, "world")
	if _, err := wallets.Credit(ctx, ann, 100, 0, user.Entry(user.VoteWorld, uuid.Nil, "refund")); err != nil {
		t.Fatalf("Credit: %v", err)
	}
	if left, err := wallets.Spend(ctx, ann, 60, user.Entry(user.VoteWorld, uuid.Nil, "found_state")); err != nil || left != 40 {
		t.Fatalf("Spend = %v, %v", left, err)
	}
}

// Every kind of votes the domain has is one the ledger takes: a kind
// left out of vote_operations_kind_check fails only in Postgres.
func TestEveryVoteKindInTheLedger(t *testing.T) {
	_, pool := newRepo(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var check string
	if err := pool.QueryRow(ctx, `SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conname = 'vote_operations_kind_check'`).Scan(&check); err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile("../user/vote_operation.go")
	if err != nil {
		t.Fatal(err)
	}
	kinds := regexp.MustCompile(`VoteKind = "(\w+)"`).FindAllSubmatch(src, -1)
	if len(kinds) == 0 {
		t.Fatal("no kinds of votes found")
	}
	for _, k := range kinds {
		if !regexp.MustCompile(`'` + string(k[1]) + `'`).MatchString(check) {
			t.Errorf("vote_operations does not take the kind %q: %s", k[1], check)
		}
	}
}
