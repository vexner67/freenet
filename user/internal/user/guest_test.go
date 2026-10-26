package user_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestUser_LeavesTraceOn(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	filled := user.Profile{Name: "Ann", Nickname: "ann"}
	owner, _ := user.New("ann@example.com")
	blank, _ := user.New("eve@example.com")
	guest := user.Restore(user.Stored{ID: uuid.Must(uuid.NewV7()), Profile: filled})

	invisible := user.Restore(user.Stored{ID: uuid.Must(uuid.NewV7()), Profile: filled, Prime: user.Prime{Until: now.Add(time.Hour), Invisible: true}})
	lapsed := user.Restore(user.Stored{ID: uuid.Must(uuid.NewV7()), Profile: filled, Prime: user.Prime{Until: now.Add(-time.Hour), Invisible: true}})
	deleted := user.Restore(user.Stored{ID: uuid.Must(uuid.NewV7()), Profile: filled, Deactivation: user.Deactivation{At: now, PurgeAt: now.Add(time.Hour)}})

	tests := []struct {
		name  string
		guest *user.User
		owner *user.User
		want  bool
	}{
		{name: "a guest", guest: guest, owner: owner, want: true},
		{name: "oneself", guest: guest, owner: guest},
		{name: "without a profile", guest: blank, owner: owner},
		{name: "invisible", guest: invisible, owner: owner},
		{name: "invisible no more", guest: lapsed, owner: owner, want: true},
		{name: "a deleted guest", guest: deleted, owner: owner},
		{name: "a deleted page", guest: guest, owner: deleted},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.guest.LeavesTraceOn(tt.owner, now); got != tt.want {
				t.Fatalf("LeavesTraceOn = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewTopBoard(t *testing.T) {
	t.Parallel()

	for _, b := range user.TopBoards {
		got, err := user.NewTopBoard(b.String())
		if err != nil || got != b {
			t.Fatalf("NewTopBoard(%q) = %q, %v", b, got, err)
		}
		if b.Weekly() != (len(b.Kinds()) > 0) {
			t.Fatalf("%q: weekly %v with kinds %v", b, b.Weekly(), b.Kinds())
		}
	}

	if _, err := user.NewTopBoard("poor"); err == nil {
		t.Fatal("NewTopBoard of an unknown board")
	}
}
