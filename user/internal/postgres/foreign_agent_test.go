package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestUserRepository_SetForeignAgent(t *testing.T) {
	repo, _ := newRepo(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	nickname := unique("fa")
	u, err := user.New(user.Email(nickname + "@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.Save(ctx, u); err != nil {
		t.Fatal(err)
	}

	if err = repo.SetForeignAgent(ctx, uuid.New(), true); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("an unknown page: %v", err)
	}
	if err = repo.SetForeignAgent(ctx, u.ID(), true); err != nil {
		t.Fatal(err)
	}

	// Saving the profile read before keeps the plaque.
	if err = u.UpdateProfile(user.Profile{Name: "Фёдор", Nickname: user.Nickname(nickname)}); err != nil {
		t.Fatal(err)
	}
	if err = repo.Update(ctx, u); err != nil {
		t.Fatal(err)
	}

	got, err := repo.GetByID(ctx, u.ID())
	if err != nil || !got.IsForeignAgent() {
		t.Fatalf("GetByID = %v, foreign agent %v", err, got != nil && got.IsForeignAgent())
	}

	q, _ := user.NewSearchQuery(nickname, user.SearchFilters{})
	found, err := repo.Search(ctx, q, uuid.Nil, 10)
	if err != nil || len(found) != 1 || !found[0].IsForeignAgent() {
		t.Fatalf("Search = %d, %v", len(found), err)
	}

	if err = repo.SetForeignAgent(ctx, u.ID(), false); err != nil {
		t.Fatal(err)
	}
	if got, err = repo.GetByID(ctx, u.ID()); err != nil || got.IsForeignAgent() {
		t.Fatalf("after taking: %v", err)
	}
}
