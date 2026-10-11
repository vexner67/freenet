package user_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestUser_CanBeInvitedBy(t *testing.T) {
	t.Parallel()

	inviter := uuid.New()

	tests := []struct {
		name    string
		setting user.MessagesWrite
		ties    user.Ties
		want    bool
	}{
		{"the default, followed", "", user.Ties{Followed: true}, true},
		{"the default, a stranger", "", user.Ties{}, false},
		{"everyone", user.MessagesWriteEveryone, user.Ties{}, true},
		{"following, followed", user.MessagesWriteFollowing, user.Ties{Followed: true}, true},
		{"following, a stranger", user.MessagesWriteFollowing, user.Ties{}, false},
		{"nobody", user.MessagesWriteNobody, user.Ties{Followed: true}, false},
		{"having written first opens nothing", user.MessagesWriteNobody, user.Ties{WroteFirst: true}, false},
		{"blocked beats everyone", user.MessagesWriteEveryone, user.Ties{Blocked: true, Followed: true}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			u := invitee(t, tt.setting)

			if got := u.CanBeInvitedBy(inviter, tt.ties); got != tt.want {
				t.Fatalf("CanBeInvitedBy = %v, want %v", got, tt.want)
			}
			if u.CanBeInvitedBy(u.ID(), user.Ties{Followed: true}) {
				t.Fatal("nobody invites themselves")
			}
		})
	}
}

func TestUser_ChatsInvite(t *testing.T) {
	t.Parallel()

	u := invitee(t, user.MessagesWriteEveryone)

	if err := u.Deactivate("", time.Now(), time.Hour); err != nil {
		t.Fatal(err)
	}
	if u.CanBeInvitedBy(uuid.New(), user.Ties{Followed: true}) {
		t.Fatal("a deleted page is added to no chat")
	}

	fresh, err := user.New("bob@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if got := fresh.Privacy().ChatsInvite; got != user.MessagesWriteFollowing {
		t.Fatalf("a new page lets %q add it to chats", got)
	}

	privacy := fresh.Privacy()
	privacy.ChatsInvite = "friends"
	if err = fresh.UpdatePrivacy(privacy); !errors.Is(err, user.ErrInvalidMessagesWrite) {
		t.Fatalf("an unknown setting: %v", err)
	}
}

func invitee(t *testing.T, setting user.MessagesWrite) *user.User {
	t.Helper()

	u, err := user.New("ann@example.com")
	if err != nil {
		t.Fatal(err)
	}

	privacy := u.Privacy()
	privacy.ChatsInvite = setting
	if err = u.UpdatePrivacy(privacy); err != nil {
		t.Fatal(err)
	}

	return u
}
