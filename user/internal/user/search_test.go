package user_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/vexner67/freenet/user/internal/user"
)

func TestSearchQuery(t *testing.T) {
	t.Parallel()

	ann := user.Profile{Name: "Анна", Surname: "Ли", Nickname: "ann_lee"}

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

			q, err := user.NewSearchQuery(tt.query)
			if err != nil {
				t.Fatal(err)
			}
			if q.Matches(ann) != tt.match {
				t.Fatalf("Matches = %v, want %v", !tt.match, tt.match)
			}
		})
	}

	if _, err := user.NewSearchQuery("   "); !errors.Is(err, user.ErrSearchQueryEmpty) {
		t.Fatalf("blank: %v", err)
	}
	if _, err := user.NewSearchQuery(strings.Repeat("я", 65)); !errors.Is(err, user.ErrSearchQueryTooLong) {
		t.Fatalf("long: %v", err)
	}

	q, err := user.NewSearchQuery("a b c d e f g")
	if err != nil || len(q.Words()) != 5 {
		t.Fatalf("words are capped at 5: %v, %v", q.Words(), err)
	}
}
