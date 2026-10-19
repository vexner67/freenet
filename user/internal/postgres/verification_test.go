package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestUserRepository_SetVerified(t *testing.T) {
	repo, _ := newRepo(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	nickname := unique("v")
	u, err := user.New(user.Email(nickname + "@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.Save(ctx, u); err != nil {
		t.Fatal(err)
	}

	before, err := repo.Count(ctx, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	if err = repo.SetVerified(ctx, uuid.New(), true); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("an unknown page: %v", err)
	}
	if err = repo.SetVerified(ctx, u.ID(), true); err != nil {
		t.Fatal(err)
	}

	// Saving the profile read before keeps the checkmark.
	if err = u.UpdateProfile(user.Profile{Name: "Вера", Nickname: user.Nickname(nickname)}); err != nil {
		t.Fatal(err)
	}
	if err = repo.Update(ctx, u); err != nil {
		t.Fatal(err)
	}

	got, err := repo.GetByID(ctx, u.ID())
	if err != nil || !got.IsVerified() {
		t.Fatalf("GetByID = %v, verified %v", err, got != nil && got.IsVerified())
	}

	after, err := repo.Count(ctx, time.Now())
	if err != nil || after.Verified != before.Verified+1 {
		t.Fatalf("verified counted %d, then %d (%v)", before.Verified, after.Verified, err)
	}

	q, _ := user.NewSearchQuery(nickname, user.SearchFilters{Verified: true})
	found, err := repo.Search(ctx, q, uuid.Nil, 10)
	if err != nil || len(found) != 1 || found[0].ID() != u.ID() {
		t.Fatalf("Search = %d, %v", len(found), err)
	}

	if err = repo.SetVerified(ctx, u.ID(), false); err != nil {
		t.Fatal(err)
	}
	if found, err = repo.Search(ctx, q, uuid.Nil, 10); err != nil || len(found) != 0 {
		t.Fatalf("Search after taking = %d, %v", len(found), err)
	}
}
