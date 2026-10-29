package user_test

import (
	"slices"
	"testing"

	"github.com/vexner67/freenet/user/internal/user"
)

func TestAdminsEmails(t *testing.T) {
	t.Parallel()

	admins, err := user.NewAdmins([]string{"Boss@Example.com", " ", "admin@example.com"})
	if err != nil {
		t.Fatal(err)
	}

	if got, want := admins.Emails(), []string{"admin@example.com", "boss@example.com"}; !slices.Equal(got, want) {
		t.Fatalf("Emails = %v, want %v", got, want)
	}
}

func TestIsEndless(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		votes int64
		want  bool
	}{
		{name: "endless", votes: user.EndlessVotes, want: true},
		{name: "spent a lot", votes: user.EndlessFloor, want: true},
		{name: "rich", votes: user.EndlessFloor - 1, want: false},
		{name: "none", votes: 0, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := user.IsEndless(tt.votes); got != tt.want {
				t.Fatalf("IsEndless(%d) = %v, want %v", tt.votes, got, tt.want)
			}
		})
	}
}
