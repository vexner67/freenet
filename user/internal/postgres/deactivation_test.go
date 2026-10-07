package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestUserRepository_Deactivation(t *testing.T) {
	repo, _ := newRepo(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	nickname := unique("deact")
	u, err := user.New(user.Email(nickname + "@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.Save(ctx, u); err != nil {
		t.Fatal(err)
	}
	if err = u.UpdateProfile(user.Profile{Name: "Удалённая", Nickname: user.Nickname(nickname)}); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	if err = u.Deactivate("надоело", now, time.Hour); err != nil {
		t.Fatal(err)
	}
	if err = repo.Update(ctx, u); err != nil {
		t.Fatal(err)
	}

	got, err := repo.GetByID(ctx, u.ID())
	if err != nil {
		t.Fatal(err)
	}
	if d := got.Deactivation(); !got.IsDeactivated() || !d.At.Equal(now) || !d.PurgeAt.Equal(now.Add(time.Hour)) || d.Reason != "надоело" {
		t.Fatalf("stored deactivation = %+v", d)
	}

	q, err := user.NewSearchQuery(nickname)
	if err != nil {
		t.Fatal(err)
	}
	if found, err := repo.Search(ctx, q, uuid.Nil, 10); err != nil || len(found) != 0 {
		t.Fatalf("search found a deleted page: %d, %v", len(found), err)
	}

	if err = got.Reactivate(now); err != nil {
		t.Fatal(err)
	}
	if err = repo.Update(ctx, got); err != nil {
		t.Fatal(err)
	}
	if again, err := repo.GetByID(ctx, u.ID()); err != nil || again.IsDeactivated() {
		t.Fatalf("restored page = %+v, %v", again.Deactivation(), err)
	}
	if found, err := repo.Search(ctx, q, uuid.Nil, 10); err != nil || len(found) != 1 {
		t.Fatalf("search after restoring: %d, %v", len(found), err)
	}
}
