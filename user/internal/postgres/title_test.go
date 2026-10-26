package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/postgres"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestTitleRepository(t *testing.T) {
	repo, pool := newRepo(t)
	titles := postgres.NewTitleRepository(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ann, bob := savedUser(t, repo, "title"), savedUser(t, repo, "title")
	now := time.Now().UTC().Truncate(time.Microsecond)

	title := func(holder uuid.UUID, name user.TitleName, icon user.TitleIcon) *user.Title {
		t.Helper()

		got, err := user.NewTitle(holder, name, icon, now)
		if err != nil {
			t.Fatal(err)
		}

		return got
	}

	if err := titles.AddTitle(ctx, title(uuid.New(), "Король", user.TitleCrown)); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("AddTitle to nobody: %v", err)
	}

	first, second := title(ann, "Король шахты", user.TitleCrown), title(ann, "Король шахты", user.TitleCrown)
	for _, added := range []*user.Title{first, second, title(bob, "Игрок недели", user.TitleCup)} {
		if err := titles.AddTitle(ctx, added); err != nil {
			t.Fatal(err)
		}
	}

	got, err := titles.Titles(ctx, ann)
	if err != nil || len(got) != 2 || got[0].ID() != second.ID() || got[1].ID() != first.ID() ||
		got[1].UserID() != ann || got[1].Name() != "Король шахты" || got[1].Icon() != user.TitleCrown || !got[1].GrantedAt().Equal(now) {
		t.Fatalf("Titles = %+v, %v", got, err)
	}

	if err = titles.DeleteTitle(ctx, first.ID()); err != nil {
		t.Fatal(err)
	}
	if err = titles.DeleteTitle(ctx, first.ID()); !errors.Is(err, user.ErrTitleNotFound) {
		t.Fatalf("DeleteTitle twice: %v", err)
	}
	if got, err = titles.Titles(ctx, ann); err != nil || len(got) != 1 || got[0].ID() != second.ID() {
		t.Fatalf("Titles after DeleteTitle = %+v, %v", got, err)
	}
}
