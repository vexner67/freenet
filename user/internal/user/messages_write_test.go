package user_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestNewMessagesWrite(t *testing.T) {
	t.Parallel()

	for _, s := range []string{"everyone", "following", "nobody"} {
		if got, err := user.NewMessagesWrite(s); err != nil || got.String() != s {
			t.Fatalf("NewMessagesWrite(%q) = %q, %v", s, got, err)
		}
	}

	for _, s := range []string{"", "only_me", "friends"} {
		if _, err := user.NewMessagesWrite(s); !errors.Is(err, user.ErrInvalidMessagesWrite) {
			t.Fatalf("NewMessagesWrite(%q) = %v, want ErrInvalidMessagesWrite", s, err)
		}
	}
}

func TestUser_CanBeMessagedBy(t *testing.T) {
	t.Parallel()

	sender := uuid.New()

	tests := []struct {
		name    string
		setting user.MessagesWrite
		ties    user.Ties
		want    bool
	}{
		{"everyone", user.MessagesWriteEveryone, user.Ties{}, true},
		{"following, followed", user.MessagesWriteFollowing, user.Ties{Followed: true}, true},
		{"following, a stranger", user.MessagesWriteFollowing, user.Ties{}, false},
		{"nobody", user.MessagesWriteNobody, user.Ties{Followed: true}, false},
		{"nobody, but wrote first", user.MessagesWriteNobody, user.Ties{WroteFirst: true}, true},
		{"following, a stranger who was written to", user.MessagesWriteFollowing, user.Ties{WroteFirst: true}, true},
		{"blocked beats everyone", user.MessagesWriteEveryone, user.Ties{Blocked: true}, false},
		{"blocked beats a reply", user.MessagesWriteEveryone, user.Ties{Blocked: true, WroteFirst: true, Followed: true}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			u, err := user.New("ann@example.com")
			if err != nil {
				t.Fatal(err)
			}
			if err = u.UpdatePrivacy(user.Privacy{WallWrite: user.WallWriteEveryone, MessagesWrite: tt.setting, ClubsRead: user.ClubsReadEveryone, PageRead: user.AudienceEveryone, InfoRead: user.AudienceEveryone, PhotosRead: user.AudienceEveryone, FriendsRead: user.AudienceEveryone, OnlineRead: user.AudienceEveryone}); err != nil {
				t.Fatal(err)
			}

			if got := u.CanBeMessagedBy(sender, tt.ties); got != tt.want {
				t.Fatalf("CanBeMessagedBy = %v, want %v", got, tt.want)
			}
			if u.CanBeMessagedBy(u.ID(), user.Ties{}) {
				t.Fatal("nobody writes to themselves")
			}
		})
	}
}

func TestUser_MessagesWriteDefaults(t *testing.T) {
	t.Parallel()

	u, err := user.New("ann@example.com")
	if err != nil {
		t.Fatal(err)
	}

	if got := u.Privacy().MessagesWrite; got != user.MessagesWriteEveryone {
		t.Fatalf("new messages setting = %q, want everyone", got)
	}

	err = u.UpdatePrivacy(user.Privacy{WallWrite: user.WallWriteEveryone})
	if !errors.Is(err, user.ErrInvalidMessagesWrite) {
		t.Fatalf("UpdatePrivacy without MessagesWrite = %v, want ErrInvalidMessagesWrite", err)
	}
}
