package postgres_test

import (
	"context"
	"errors"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/club/internal/club"
	"github.com/vexner67/freenet/club/internal/postgres"
	"github.com/vexner67/freenet/pkg/pgpool"
)

// Integration test: needs a database with migrations applied, e.g.
// TEST_DATABASE_URL=postgres://... go test ./internal/postgres
func TestRepositories(t *testing.T) {
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

	clubs := postgres.NewClubRepository(pool)
	members := postgres.NewMemberRepository(pool)
	ann, bob := uuid.New(), uuid.New()
	// Addresses are unique across the database, which other runs share.
	address := club.Address("cats" + uuid.NewString()[:8])

	create := func(owner uuid.UUID) *club.Club {
		t.Helper()
		c, err := club.New("Любители котов", "про котов\nи кошек", time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err = clubs.Create(ctx, c, owner); err != nil {
			t.Fatalf("Create: %v", err)
		}
		return c
	}

	c := create(ann)

	t.Run("a club reads back with its owner", func(t *testing.T) {
		got, err := clubs.Get(ctx, c.ID())
		if err != nil || got.Members() != 1 || got.Name() != c.Name() || got.Description() != c.Description() || !got.CreatedAt().Equal(c.CreatedAt()) {
			t.Fatalf("Get = %+v, %v", got, err)
		}
		if owner, err := members.Owner(ctx, c.ID()); err != nil || owner != ann {
			t.Fatalf("Owner = %v, %v", owner, err)
		}
		if role, err := members.Role(ctx, c.ID(), ann); err != nil || role != club.RoleOwner {
			t.Fatalf("Role = %q, %v", role, err)
		}
		if _, err = clubs.Get(ctx, uuid.New()); !errors.Is(err, club.ErrNotFound) {
			t.Fatalf("Get missing: %v", err)
		}
	})

	t.Run("address", func(t *testing.T) {
		edited, err := c.Edited(club.RoleOwner, club.Changes{Name: "Кошки", Address: address, Wall: club.WallClosed})
		if err != nil {
			t.Fatal(err)
		}
		if err = clubs.Update(ctx, edited); err != nil {
			t.Fatalf("Update: %v", err)
		}

		got, err := clubs.GetByAddress(ctx, club.Address("CATS"+address.String()[4:]))
		if err != nil || got.ID() != c.ID() || got.Wall() != club.WallClosed || got.Description() != "" {
			t.Fatalf("GetByAddress = %+v, %v", got, err)
		}

		other := create(uuid.New())
		taken, err := other.Edited(club.RoleOwner, club.Changes{Name: "Другие", Address: club.Address("CATS" + address.String()[4:]), Wall: club.WallOpen})
		if err != nil {
			t.Fatal(err)
		}
		if err = clubs.Update(ctx, taken); !errors.Is(err, club.ErrAddressTaken) {
			t.Fatalf("taken address: %v", err)
		}

		// No address is no address: any number of clubs have none.
		none, err := other.Edited(club.RoleOwner, club.Changes{Name: "Другие", Wall: club.WallOpen})
		if err != nil {
			t.Fatal(err)
		}
		if err = clubs.Update(ctx, none); err != nil {
			t.Fatalf("no address: %v", err)
		}

		many, err := clubs.GetMany(ctx, []uuid.UUID{c.ID(), other.ID(), uuid.New()})
		if err != nil || len(many) != 2 {
			t.Fatalf("GetMany = %v, %v", many, err)
		}
	})

	t.Run("members", func(t *testing.T) {
		if err := members.Join(ctx, c.ID(), bob); err != nil {
			t.Fatal(err)
		}
		if err := members.Join(ctx, c.ID(), bob); err != nil {
			t.Fatalf("join twice: %v", err)
		}
		if err := members.Join(ctx, uuid.New(), bob); !errors.Is(err, club.ErrNotFound) {
			t.Fatalf("join a missing club: %v", err)
		}
		if got, _ := clubs.Get(ctx, c.ID()); got.Members() != 2 {
			t.Fatalf("members = %d, want 2", got.Members())
		}

		list, err := members.List(ctx, c.ID(), uuid.Nil, 10, false)
		if err != nil || len(list) != 2 || list[0].UserID != bob || list[1].UserID != ann || list[1].Role != club.RoleOwner {
			t.Fatalf("List = %+v, %v; the newest first", list, err)
		}
		if older, err := members.List(ctx, c.ID(), list[0].ID, 10, false); err != nil || len(older) != 1 || older[0].UserID != ann {
			t.Fatalf("List after cursor = %+v, %v", older, err)
		}
		if admins, err := members.List(ctx, c.ID(), uuid.Nil, 10, true); err != nil || len(admins) != 1 || admins[0].UserID != ann {
			t.Fatalf("admins = %+v, %v", admins, err)
		}

		if err = members.SetRole(ctx, c.ID(), bob, club.RoleAdmin); err != nil {
			t.Fatal(err)
		}
		if role, _ := members.Role(ctx, c.ID(), bob); role != club.RoleAdmin {
			t.Fatalf("bob = %q", role)
		}
		if err = members.SetRole(ctx, c.ID(), ann, club.RoleMember); !errors.Is(err, club.ErrNotMember) {
			t.Fatalf("demote the owner: %v", err)
		}

		mine, err := members.ListClubs(ctx, bob, uuid.Nil, 10)
		if err != nil || len(mine) != 1 || mine[0].Club.ID() != c.ID() || mine[0].Role != club.RoleAdmin || mine[0].Club.Members() != 2 {
			t.Fatalf("ListClubs = %+v, %v", mine, err)
		}
		if ids, err := members.ClubIDs(ctx, bob, 10); err != nil || !reflect.DeepEqual(ids, []uuid.UUID{c.ID()}) {
			t.Fatalf("ClubIDs = %v, %v", ids, err)
		}

		// The owner never leaves, whatever the caller checked.
		if err = members.Leave(ctx, c.ID(), ann); err != nil {
			t.Fatal(err)
		}
		if err = members.Leave(ctx, c.ID(), bob); err != nil {
			t.Fatal(err)
		}
		if got, _ := clubs.Get(ctx, c.ID()); got.Members() != 1 {
			t.Fatalf("members after leaving = %d, want the owner", got.Members())
		}
		if role, _ := members.Role(ctx, c.ID(), bob); role != club.RoleNone {
			t.Fatalf("bob after leaving = %q", role)
		}
	})

	// The count changes in the statement that adds or removes a member.
	t.Run("concurrent joins and leaves", func(t *testing.T) {
		d := create(ann)
		people := make([]uuid.UUID, 20)
		for i := range people {
			people[i] = uuid.New()
		}

		var wg sync.WaitGroup
		for _, p := range people {
			wg.Go(func() {
				if err := members.Join(ctx, d.ID(), p); err != nil {
					t.Error(err)
				}
				if err := members.Join(ctx, d.ID(), p); err != nil {
					t.Error(err)
				}
			})
		}
		wg.Wait()
		for _, p := range people[:5] {
			wg.Go(func() {
				if err := members.Leave(ctx, d.ID(), p); err != nil {
					t.Error(err)
				}
			})
		}
		wg.Wait()

		if got, _ := clubs.Get(ctx, d.ID()); got.Members() != 16 {
			t.Fatalf("members = %d, want 16", got.Members())
		}
	})

	t.Run("erasing a user", func(t *testing.T) {
		e := create(ann)
		if err := members.Join(ctx, e.ID(), bob); err != nil {
			t.Fatal(err)
		}

		if err := members.DeleteByUser(ctx, ann); err != nil {
			t.Fatal(err)
		}

		got, err := clubs.Get(ctx, e.ID())
		if err != nil || got.Members() != 1 {
			t.Fatalf("club after its owner is erased = %+v, %v", got, err)
		}
		if owner, err := members.Owner(ctx, e.ID()); err != nil || owner != uuid.Nil {
			t.Fatalf("Owner = %v, %v; want none", owner, err)
		}
	})
}
