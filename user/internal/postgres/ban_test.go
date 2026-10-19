package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestUserRepository_SetBan(t *testing.T) {
	repo, _ := newRepo(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	nickname := unique("b")
	u, err := user.New(user.Email(nickname + "@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.Save(ctx, u); err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	ban, _ := user.NewBan("spam", now, now.Add(time.Hour))
	if err = repo.SetBan(ctx, uuid.New(), ban); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("an unknown page: %v", err)
	}
	if err = repo.SetBan(ctx, u.ID(), ban); err != nil {
		t.Fatal(err)
	}

	// Saving the profile read before keeps the block.
	if err = u.UpdateProfile(user.Profile{Name: "Борис", Nickname: user.Nickname(nickname)}); err != nil {
		t.Fatal(err)
	}
	if err = repo.Update(ctx, u); err != nil {
		t.Fatal(err)
	}

	got, err := repo.GetByID(ctx, u.ID())
	if err != nil || got.Ban() != ban {
		t.Fatalf("GetByID = %v, ban %+v, want %+v", err, got.Ban(), ban)
	}

	q, _ := user.NewSearchQuery(nickname, user.SearchFilters{Banned: true})
	found, err := repo.Search(ctx, q, uuid.Nil, 10)
	if err != nil || len(found) != 1 || found[0].ID() != u.ID() {
		t.Fatalf("Search of the blocked = %v, %v", found, err)
	}

	// For good, then let be.
	forever, _ := user.NewBan("spam", now, time.Time{})
	if err = repo.SetBan(ctx, u.ID(), forever); err != nil {
		t.Fatal(err)
	}
	if got, err = repo.GetByID(ctx, u.ID()); err != nil || !got.Ban().Forever() || !got.IsBannedAt(now.AddDate(10, 0, 0)) {
		t.Fatalf("for good: %+v, %v", got.Ban(), err)
	}

	if err = repo.SetBan(ctx, u.ID(), user.Ban{}); err != nil {
		t.Fatal(err)
	}
	if got, err = repo.GetByID(ctx, u.ID()); err != nil || got.Ban() != (user.Ban{}) {
		t.Fatalf("let be: %+v, %v", got.Ban(), err)
	}
	if found, err = repo.Search(ctx, q, uuid.Nil, 10); err != nil || len(found) != 0 {
		t.Fatalf("Search after letting be = %v, %v", found, err)
	}
}
