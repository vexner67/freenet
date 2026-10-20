package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestUserRepository_SetAlert(t *testing.T) {
	repo, _ := newRepo(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	nickname := unique("al")
	u, err := user.New(user.Email(nickname + "@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.Save(ctx, u); err != nil {
		t.Fatal(err)
	}

	const alert = user.Alert("Страница под наблюдением")
	if err = repo.SetAlert(ctx, uuid.New(), alert); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("an unknown page: %v", err)
	}
	if err = repo.SetAlert(ctx, u.ID(), alert); err != nil {
		t.Fatal(err)
	}

	// Saving the profile read before keeps the label.
	if err = u.UpdateProfile(user.Profile{Name: "Фёдор", Nickname: user.Nickname(nickname)}); err != nil {
		t.Fatal(err)
	}
	if err = repo.Update(ctx, u); err != nil {
		t.Fatal(err)
	}

	got, err := repo.GetByID(ctx, u.ID())
	if err != nil || got.Alert() != alert {
		t.Fatalf("GetByID = %v, %v", got, err)
	}

	q, _ := user.NewSearchQuery(nickname, user.SearchFilters{})
	found, err := repo.Search(ctx, q, uuid.Nil, 10)
	if err != nil || len(found) != 1 || found[0].Alert() != alert {
		t.Fatalf("Search = %d, %v", len(found), err)
	}

	if err = repo.SetAlert(ctx, u.ID(), ""); err != nil {
		t.Fatal(err)
	}
	if got, err = repo.GetByID(ctx, u.ID()); err != nil || got.Alert() != "" {
		t.Fatalf("after taking: %v", err)
	}
}
