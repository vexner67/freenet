package postgres_test

import (
	"context"
	"os"
	"regexp"
	"testing"
	"time"
)

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
