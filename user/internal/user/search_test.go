package user_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/vexner67/freenet/user/internal/user"
)

func TestSearchQuery(t *testing.T) {
	t.Parallel()

	ann, _ := user.New("ann@example.com")
	_ = ann.UpdateProfile(user.Profile{Name: "Анна", Surname: "Ли", Nickname: "ann_lee", Hometown: "Тверь"})
	ann.UpdateContacts(user.Contacts{City: "Москва"})
	ann.UpdateInterests(user.Interests{Films: "Брат, Брат 2", Books: "Мастер и Маргарита"})

	tests := []struct {
		query string
		match bool
	}{
		{"анна", true},
		{"АННА", true},
		{"ли анна", true},
		{"  ан   ли ", true},
		{"ann_", true},
		{"lee анн", true},
		{"анна петрова", false},
		{"боб", false},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			t.Parallel()

			q, err := user.NewSearchQuery(tt.query, user.SearchFilters{})
			if err != nil {
				t.Fatal(err)
			}
			if q.Matches(ann) != tt.match {
				t.Fatalf("Matches = %v, want %v", !tt.match, tt.match)
			}
		})
	}

	if _, err := user.NewSearchQuery("   ", user.SearchFilters{}); !errors.Is(err, user.ErrSearchQueryEmpty) {
		t.Fatalf("blank: %v", err)
	}
	if _, err := user.NewSearchQuery(strings.Repeat("я", 65), user.SearchFilters{}); !errors.Is(err, user.ErrSearchQueryTooLong) {
		t.Fatalf("long: %v", err)
	}

	q, err := user.NewSearchQuery("a b c d e f g", user.SearchFilters{})
	if err != nil || len(q.Words()) != 5 {
		t.Fatalf("words are capped at 5: %v, %v", q.Words(), err)
	}
}

func TestSearchQuery_Filters(t *testing.T) {
	t.Parallel()

	ann, _ := user.New("ann@example.com")
	_ = ann.UpdateProfile(user.Profile{Name: "Анна", Nickname: "ann", Hometown: "Тверь"})
	ann.UpdateContacts(user.Contacts{City: "Нижний Новгород"})
	ann.UpdateInterests(user.Interests{Films: "Брат, Брат 2", Shows: "Улицы разбитых фонарей", Books: "Мастер и Маргарита"})

	for _, tt := range []struct {
		query   string
		filters user.SearchFilters
		match   bool
	}{
		{"", user.SearchFilters{Hometown: "  ТВЕРЬ "}, true},
		{"", user.SearchFilters{Hometown: "Твер"}, false},
		{"", user.SearchFilters{City: "нижний  новгород"}, true},
		{"анна", user.SearchFilters{Film: "брат 2"}, true},
		{"боб", user.SearchFilters{Film: "брат 2"}, false},
		{"", user.SearchFilters{Show: "фонарей", Book: "маргарита"}, true},
		{"", user.SearchFilters{Book: "война и мир"}, false},
	} {
		q, err := user.NewSearchQuery(tt.query, tt.filters)
		if err != nil {
			t.Fatal(err)
		}
		if q.Matches(ann) != tt.match {
			t.Errorf("%q %+v: Matches = %v", tt.query, tt.filters, !tt.match)
		}
	}

	if _, err := user.NewSearchQuery("", user.SearchFilters{City: strings.Repeat("я", 65)}); !errors.Is(err, user.ErrSearchQueryTooLong) {
		t.Fatalf("long filter: %v", err)
	}
}

func TestSearchQuery_Verified(t *testing.T) {
	t.Parallel()

	ann, _ := user.New("ann@example.com")
	_ = ann.UpdateProfile(user.Profile{Name: "Анна", Nickname: "ann"})
	privacy := ann.Privacy()
	privacy.Closed = true
	if err := ann.UpdatePrivacy(privacy); err != nil {
		t.Fatal(err)
	}

	q, err := user.NewSearchQuery("", user.SearchFilters{Verified: true})
	if err != nil {
		t.Fatalf("the checkmark alone: %v", err)
	}
	if q.Filters().LookIntoInfo() {
		t.Fatal("the checkmark looks into the information")
	}
	if q.Matches(ann) {
		t.Fatal("a page without the checkmark is found")
	}
	// A closed page tells the checkmark, as it does on the page itself.
	if !q.Matches(ann.WithVerified(true)) {
		t.Fatal("a closed page with the checkmark is not found")
	}

	if q, _ = user.NewSearchQuery("боб", user.SearchFilters{Verified: true}); q.Matches(ann.WithVerified(true)) {
		t.Fatal("the words are not looked at")
	}
}
