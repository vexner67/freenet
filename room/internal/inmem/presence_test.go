package inmem_test

import (
	"context"
	"github.com/google/uuid"
	"github.com/vexner67/freenet/room/internal/inmem"
	"testing"
	"time"
)

type clock struct{ now time.Time }

func (c *clock) Now() time.Time { return c.now }
func TestEphemeralStateExpires(t *testing.T) {
	ctx := context.Background()
	c := &clock{time.Now().UTC()}
	s := inmem.NewState(c, time.Hour)
	id, user := uuid.New(), uuid.New()
	if _, err := s.Join(ctx, id, user, c.now, 5, 20); err != nil {
		t.Fatal(err)
	}
	c.now = c.now.Add(time.Hour)
	members, err := s.Members(ctx, id)
	if err != nil || len(members) != 0 {
		t.Fatal(members, err)
	}
	current, err := s.Current(ctx, []uuid.UUID{user})
	if err != nil || len(current) != 0 {
		t.Fatal(current, err)
	}
	live, err := s.Live(ctx)
	if err != nil || len(live) != 0 {
		t.Fatal(live, err)
	}
}
