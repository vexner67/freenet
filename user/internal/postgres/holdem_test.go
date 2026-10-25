package postgres_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/user/internal/postgres"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestHoldemRepository(t *testing.T) {
	_, pool := newRepo(t)
	tables := postgres.NewHoldemRepository(pool)
	tx := pgpool.NewTxManager(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ann, bob, eve := uuid.New(), uuid.New(), uuid.New()
	now := time.Now().UTC().Truncate(time.Millisecond)
	const blind = 7654322

	open, err := user.NewHoldemTable(uuid.New(), ann, "Пятница", false, 6, blind, now)
	if err != nil {
		t.Fatal(err)
	}
	if err = open.Sit(ann, -1, blind*20, now); err != nil {
		t.Fatal(err)
	}
	private, err := user.NewHoldemTable(uuid.New(), ann, "Свои", true, 2, blind, now)
	if err != nil {
		t.Fatal(err)
	}
	if err = private.Invite(ann, []uuid.UUID{eve}); err != nil {
		t.Fatal(err)
	}
	for _, table := range []*user.HoldemTable{open, private} {
		if err = tables.AddTable(ctx, table); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = tables.DeleteTable(context.Background(), table.ID()) })
	}

	// Bob is matched to the open table and sits; the hand is due soon.
	err = tx.WithinTx(ctx, func(ctx context.Context) error {
		found, err := tables.LockFree(ctx, blind, bob)
		if err != nil || found.ID() != open.ID() {
			t.Fatalf("LockFree = %v, %v", found, err)
		}
		if err = found.Sit(bob, -1, blind*20, now); err != nil {
			return err
		}

		return tables.SaveTable(ctx, found)
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tables.LockFree(ctx, blind, ann); !errors.Is(err, user.ErrTableNotFound) {
		t.Fatalf("LockFree of the seated: %v", err)
	}

	got, err := tables.Table(ctx, open.ID())
	if err != nil || got.Players() != 2 || got.Name() != "Пятница" || got.Deadline().IsZero() {
		t.Fatalf("Table = %+v, %v", got, err)
	}
	if _, err = tables.Table(ctx, uuid.New()); !errors.Is(err, user.ErrTableNotFound) {
		t.Fatalf("Table of nothing: %v", err)
	}

	ids := func(list []*user.HoldemTable) []uuid.UUID {
		var out []uuid.UUID
		for _, table := range list {
			out = append(out, table.ID())
		}
		return out
	}
	for _, c := range []struct {
		section service.TableSection
		viewer  uuid.UUID
		want    *user.HoldemTable
		absent  *user.HoldemTable
	}{
		{service.TablesPublic, eve, open, private},
		{service.TablesMine, bob, open, private},
		{service.TablesInvited, eve, private, open},
	} {
		list, err := tables.Tables(ctx, c.section, c.viewer)
		if err != nil || !slices.Contains(ids(list), c.want.ID()) || slices.Contains(ids(list), c.absent.ID()) {
			t.Fatalf("Tables %s = %v, %v", c.section, ids(list), err)
		}
	}

	due, err := tables.Due(ctx, now.Add(time.Minute), 100)
	if err != nil || !slices.Contains(due, open.ID()) || slices.Contains(due, private.ID()) {
		t.Fatalf("Due = %v, %v", due, err)
	}

	if err = tables.DeleteTable(ctx, open.ID()); err != nil {
		t.Fatal(err)
	}
	if err = tables.SaveTable(ctx, open); !errors.Is(err, user.ErrTableNotFound) {
		t.Fatalf("SaveTable of the deleted: %v", err)
	}
}
