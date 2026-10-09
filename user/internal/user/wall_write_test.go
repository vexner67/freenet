package user_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestNewWallWrite(t *testing.T) {
	t.Parallel()

	for _, s := range []string{"everyone", "following", "only_me"} {
		if got, err := user.NewWallWrite(s); err != nil || got.String() != s {
			t.Fatalf("NewWallWrite(%q) = %q, %v", s, got, err)
		}
	}

	for _, s := range []string{"", "friends", "EVERYONE"} {
		if _, err := user.NewWallWrite(s); !errors.Is(err, user.ErrInvalidWallWrite) {
			t.Fatalf("NewWallWrite(%q) = %v, want ErrInvalidWallWrite", s, err)
		}
	}
}

func TestUser_CanPostOnWall(t *testing.T) {
	t.Parallel()

	stranger := uuid.New()

	tests := []struct {
		wallWrite user.WallWrite
		follows   bool
		want      bool
	}{
		{user.WallWriteEveryone, false, true},
		{user.WallWriteFollowing, true, true},
		{user.WallWriteFollowing, false, false},
		{user.WallWriteOnlyMe, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.wallWrite.String(), func(t *testing.T) {
			t.Parallel()

			u, err := user.New("ann@example.com")
			if err != nil {
				t.Fatal(err)
			}
			if err = u.UpdatePrivacy(user.Privacy{WallWrite: tt.wallWrite, MessagesWrite: user.MessagesWriteEveryone, ClubsRead: user.ClubsReadEveryone, PageRead: user.AudienceEveryone, InfoRead: user.AudienceEveryone, PhotosRead: user.AudienceEveryone, FriendsRead: user.AudienceEveryone, OnlineRead: user.AudienceEveryone}); err != nil {
				t.Fatal(err)
			}

			if got := u.CanPostOnWall(stranger, user.Ties{Followed: tt.follows}); got != tt.want {
				t.Fatalf("CanPostOnWall(follows = %v) = %v, want %v", tt.follows, got, tt.want)
			}
			if u.CanPostOnWall(stranger, user.Ties{Followed: true, Blocked: true}) {
				t.Fatal("a block shuts the wall")
			}
			if !u.CanPostOnWall(u.ID(), user.Ties{}) {
				t.Fatal("the owner always may post on their wall")
			}
		})
	}
}

func TestUser_WallWriteDefaults(t *testing.T) {
	t.Parallel()

	u, err := user.New("ann@example.com")
	if err != nil {
		t.Fatal(err)
	}

	if got := u.Privacy().WallWrite; got != user.WallWriteFollowing {
		t.Fatalf("a new wall is open to %q, want following", got)
	}

	if err = u.UpdatePrivacy(user.Privacy{}); !errors.Is(err, user.ErrInvalidWallWrite) {
		t.Fatalf("UpdatePrivacy without WallWrite = %v, want ErrInvalidWallWrite", err)
	}
	if got := u.Privacy().WallWrite; got != user.WallWriteFollowing {
		t.Fatalf("a rejected update changed the setting to %q", got)
	}
}
