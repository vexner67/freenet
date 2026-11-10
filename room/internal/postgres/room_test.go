package postgres_test

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/room/internal/postgres"
	"github.com/vexner67/freenet/room/internal/room"
	"github.com/vexner67/freenet/room/internal/service"
	"os"
	"testing"
	"time"
)

func TestRoomRepository(t *testing.T) {
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
	defer pool.Close()
	repo := postgres.NewRoomRepository(pool, 2*time.Second)
	bans := postgres.NewBanRepository(pool, 2*time.Second)
	tx := postgres.NewTxManager(pool)
	owner, club := uuid.New(), uuid.New()
	now := time.Now().UTC().Truncate(time.Microsecond)
	personal, err := room.New(room.Input{OwnerID: owner, Title: "Personal", Kind: room.KindTalk, Access: room.AccessEveryone, Control: room.ControlOwner, Permanent: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	group, err := room.New(room.Input{OwnerID: owner, ClubID: club, Title: "Club", Kind: room.KindWatch, Access: room.AccessClub, Control: room.ControlEveryone, Permanent: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := repo.Delete(ctx, personal.ID()); err != nil {
			t.Error(err)
		}
		if err := repo.Delete(ctx, group.ID()); err != nil {
			t.Error(err)
		}
	}()
	for _, r := range []*room.Room{personal, group} {
		if err := repo.Save(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	got, err := repo.Get(ctx, group.ID())
	if err != nil || got.ClubID() != club || got.Title() != group.Title() || !got.CreatedAt().Equal(now) {
		t.Fatalf("round trip: %v %v", got, err)
	}
	rows, err := repo.List(ctx, service.Filter{OwnerID: owner, PersonalOnly: true, PermanentOnly: true, Limit: 5})
	if err != nil || len(rows) != 1 || rows[0].ID() != personal.ID() {
		t.Fatalf("personal: %v %v", rows, err)
	}
	rows, err = repo.List(ctx, service.Filter{ClubID: club, Limit: 5})
	if err != nil || len(rows) != 1 {
		t.Fatalf("club: %v %v", rows, err)
	}
	sentinel := errors.New("rollback")
	err = tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := bans.SaveBan(ctx, personal.ID(), owner, now.Add(time.Hour)); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	until, err := bans.Until(ctx, personal.ID(), owner)
	if err != nil || !until.IsZero() {
		t.Fatalf("ban rolled back: %v %v", until, err)
	}
	if err = bans.SaveBan(ctx, personal.ID(), owner, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	until, err = bans.Until(ctx, personal.ID(), owner)
	if err != nil || !until.Equal(now.Add(time.Hour)) {
		t.Fatal(until, err)
	}
	if err = bans.DeleteUser(ctx, owner); err != nil {
		t.Fatal(err)
	}
	if err = personal.Close(owner, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err = repo.Save(ctx, personal); err != nil {
		t.Fatal(err)
	}
	rows, err = repo.List(ctx, service.Filter{OwnerID: owner, PersonalOnly: true, Limit: 5})
	if err != nil || len(rows) != 0 {
		t.Fatal(rows, err)
	}
	if _, err = repo.Get(ctx, uuid.New()); !errors.Is(err, room.ErrNotFound) {
		t.Fatal(err)
	}
}
