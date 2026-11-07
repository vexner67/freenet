package service_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/inmem"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

// The search shows people by their rating, highest first, then the newest,
// page by page.
func TestUserService_SearchByRating(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	users := inmem.NewUserRepository()
	userSvc := service.NewUserService(users)

	register := func(nickname string, rating int) uuid.UUID {
		t.Helper()

		id, err := userSvc.GetOrCreateByEmail(ctx, mustEmail(t, nickname+"@example.com"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = userSvc.UpdateProfile(ctx, id, user.Profile{Name: "Анна", Nickname: mustNickname(t, nickname)}, ""); err != nil {
			t.Fatal(err)
		}
		if _, err = users.Credit(ctx, id, 0, rating, user.VoteEntry{}); err != nil {
			t.Fatal(err)
		}

		return id
	}

	older := register("older", 0)
	newer := register("newer", 0)
	rich := register("rich", 1000)
	richer := register("richer", user.MaxRating)

	// A photo is 20 % of the page.
	if _, err := userSvc.SetAvatar(ctx, older, uuid.New()); err != nil {
		t.Fatal(err)
	}

	q, err := user.NewSearchQuery("анна", user.SearchFilters{})
	if err != nil {
		t.Fatal(err)
	}

	var (
		got    []uuid.UUID
		cursor user.SearchCursor
	)
	for range 5 {
		page, err := userSvc.Search(ctx, q, cursor, 1)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range page.Users {
			got = append(got, p.ID)
		}
		if page.NextCursor == (user.SearchCursor{}) {
			break
		}
		cursor = page.NextCursor
	}

	want := []uuid.UUID{richer, rich, older, newer}
	if len(got) != len(want) {
		t.Fatalf("found %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("found %v, want %v", got, want)
		}
	}
}
