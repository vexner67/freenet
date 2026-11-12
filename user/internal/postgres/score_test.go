package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/postgres"
	"github.com/vexner67/freenet/user/internal/user"
)

// users.score, which orders the search, counts the page as Completeness
// does: field by field, with the rating bought, up to user.MaxRating.
func TestUserRepository_SearchByScore(t *testing.T) {
	repo, pool := newRepo(t)
	wallets := postgres.NewWalletRepository(pool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	word := unique("рейт")
	newUser := func(fill func(u *user.User), rating int) *user.User {
		t.Helper()

		u, err := user.New(user.Email(unique("sc") + "@example.com"))
		if err != nil {
			t.Fatal(err)
		}
		if err = repo.Save(ctx, u); err != nil {
			t.Fatal(err)
		}
		if err = u.UpdateProfile(user.Profile{Name: user.Name(word), Nickname: user.Nickname(unique("sc"))}); err != nil {
			t.Fatal(err)
		}
		fill(u)
		if err = repo.Update(ctx, u); err != nil {
			t.Fatal(err)
		}
		if rating > 0 {
			if _, err = wallets.Credit(ctx, u.ID(), 0, rating, user.VoteEntry{}); err != nil {
				t.Fatal(err)
			}
		}

		got, err := repo.GetByID(ctx, u.ID())
		if err != nil {
			t.Fatal(err)
		}

		return got
	}

	birthday, _ := user.NewBirthday(time.Date(1990, 5, 6, 0, 0, 0, 0, time.UTC))
	full := newUser(func(u *user.User) {
		p := u.Profile()
		p.Surname, p.Status, p.Gender, p.Birthday, p.Hometown = "Ли", "Привет", user.GenderFemale, birthday, "Тверь"
		p.Relationship, p.Worldview, p.PoliticalViews = user.RelationshipSingle, user.WorldviewOrthodox, user.PoliticalViewsCommunist
		if err := u.UpdateProfile(p); err != nil {
			t.Fatal(err)
		}
		u.UpdateContacts(user.Contacts{Email: "a@mail.example", Telegram: "ann_tg", Website: "https://a.example", City: "Москва", Address: "Арбат"})
		u.UpdateInterests(user.Interests{
			Interests: "a", Music: "b", Films: "c", Shows: "d", Books: "e", Quotes: "f", Games: "g", About: "h",
		})
		u.SetAvatar(uuid.New())
	}, 0)
	photo := newUser(func(u *user.User) { u.SetAvatar(uuid.New()) }, 0)
	empty := newUser(func(*user.User) {}, 0)
	rich := newUser(func(u *user.User) { u.UpdateStatus("Богат") }, 1000)
	richest := newUser(func(*user.User) {}, user.MaxRating)

	if p, _ := full.Completeness(); p != 100 {
		t.Fatalf("a full page: %d", p)
	}
	// What vouchers give stops at the rating that can be bought, which the
	// database checks too.
	if richest.Wallet().Rating != user.MaxBoughtRating {
		t.Fatalf("Credit past the limit: %+v", richest.Wallet())
	}

	q, err := user.NewSearchQuery(word, user.SearchFilters{})
	if err != nil {
		t.Fatal(err)
	}

	var found []*user.User
	cursor := user.SearchCursor{}
	for range 10 {
		page, err := repo.Search(ctx, q, cursor, 2)
		if err != nil {
			t.Fatal(err)
		}
		found = append(found, page...)
		if len(page) < 2 {
			break
		}
		cursor = user.CursorOf(page[len(page)-1])
	}

	want := []*user.User{richest, rich, full, photo, empty}
	if len(found) != len(want) {
		t.Fatalf("found %d pages, want %d", len(found), len(want))
	}
	for i, u := range want {
		if found[i].ID() != u.ID() {
			t.Fatalf("place %d: %v, want %v", i, user.CursorOf(found[i]), user.CursorOf(u))
		}
	}

	// The column itself agrees with the domain, page by page.
	for _, u := range want {
		var score int
		if err = pool.QueryRow(ctx, "SELECT score FROM users WHERE id = $1", u.ID()).Scan(&score); err != nil {
			t.Fatal(err)
		}
		if p, _ := u.Completeness(); score != p {
			t.Fatalf("score %d, Completeness %d", score, p)
		}
	}
}
