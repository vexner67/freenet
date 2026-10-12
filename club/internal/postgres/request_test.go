package postgres_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/club/internal/club"
	"github.com/vexner67/freenet/club/internal/postgres"
	"github.com/vexner67/freenet/pkg/pgpool"
)

func TestRequestRepository(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	clubs, requests := postgres.NewClubRepository(pool), postgres.NewRequestRepository(pool)
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()

	c, err := club.New("Закрытые коты", "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = clubs.Create(ctx, c, ann); err != nil {
		t.Fatal(err)
	}

	// The settings keep the type of the club.
	closed, err := c.Edited(club.RoleOwner, club.Changes{Name: c.Name(), Wall: club.WallOpen, AdminsDisplay: club.AdminsDisplayCreator, Closed: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = clubs.Update(ctx, closed); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got, err := clubs.Get(ctx, c.ID()); err != nil || !got.Closed() {
		t.Fatalf("Get = %+v, %v; want a closed club", got, err)
	}

	if err = requests.Add(ctx, uuid.New(), bob); !errors.Is(err, club.ErrNotFound) {
		t.Fatalf("Add to an unknown club: %v", err)
	}

	// Asking twice is asking once.
	for _, who := range []uuid.UUID{bob, bob, cat} {
		if err = requests.Add(ctx, c.ID(), who); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}

	if n, err := requests.Count(ctx, c.ID()); err != nil || n != 2 {
		t.Fatalf("Count = %d, %v", n, err)
	}
	if has, err := requests.Has(ctx, c.ID(), bob); err != nil || !has {
		t.Fatalf("Has = %v, %v", has, err)
	}
	if has, err := requests.Has(ctx, c.ID(), ann); err != nil || has {
		t.Fatalf("Has for who did not ask = %v, %v", has, err)
	}

	// The newest first, page by page.
	page, err := requests.List(ctx, c.ID(), uuid.Nil, 1)
	if err != nil || len(page) != 1 || page[0].UserID != cat {
		t.Fatalf("List = %+v, %v", page, err)
	}
	if page, err = requests.List(ctx, c.ID(), page[0].ID, 10); err != nil || len(page) != 1 || page[0].UserID != bob {
		t.Fatalf("the next page = %+v, %v", page, err)
	}

	if gone, err := requests.Delete(ctx, c.ID(), bob); err != nil || !gone {
		t.Fatalf("Delete = %v, %v", gone, err)
	}
	if gone, err := requests.Delete(ctx, c.ID(), bob); err != nil || gone {
		t.Fatalf("Delete twice = %v, %v", gone, err)
	}

	if err = requests.DeleteByUser(ctx, cat); err != nil {
		t.Fatalf("DeleteByUser: %v", err)
	}
	if n, err := requests.Count(ctx, c.ID()); err != nil || n != 0 {
		t.Fatalf("Count after the erasure = %d, %v", n, err)
	}

	if err = requests.Add(ctx, c.ID(), bob); err != nil {
		t.Fatal(err)
	}
	if err = requests.DeleteByClub(ctx, c.ID()); err != nil {
		t.Fatalf("DeleteByClub: %v", err)
	}
	if n, err := requests.Count(ctx, c.ID()); err != nil || n != 0 {
		t.Fatalf("Count of the opened club = %d, %v", n, err)
	}
}
