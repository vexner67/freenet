package user_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestNewClubsRead(t *testing.T) {
	t.Parallel()

	for _, s := range []string{"everyone", "following", "only_me"} {
		if got, err := user.NewClubsRead(s); err != nil || got.String() != s {
			t.Fatalf("NewClubsRead(%q) = %q, %v", s, got, err)
		}
	}

	for _, s := range []string{"", "friends", "EVERYONE"} {
		if _, err := user.NewClubsRead(s); !errors.Is(err, user.ErrInvalidClubsRead) {
			t.Fatalf("NewClubsRead(%q) = %v, want ErrInvalidClubsRead", s, err)
		}
	}
}

func TestUser_ClubsVisibleTo(t *testing.T) {
	t.Parallel()

	stranger := uuid.New()

	tests := []struct {
		setting user.ClubsRead
		viewer  uuid.UUID
		ties    user.Ties
		want    bool
	}{
		{user.ClubsReadEveryone, uuid.Nil, user.Ties{}, true},
		{user.ClubsReadEveryone, stranger, user.Ties{Blocked: true}, true},
		{user.ClubsReadFollowing, stranger, user.Ties{Followed: true}, true},
		{user.ClubsReadFollowing, stranger, user.Ties{}, false},
		{user.ClubsReadFollowing, uuid.Nil, user.Ties{}, false},
		{user.ClubsReadOnlyMe, stranger, user.Ties{Followed: true}, false},
	}

	for _, tt := range tests {
		t.Run(tt.setting.String(), func(t *testing.T) {
			t.Parallel()

			u, err := user.New("ann@example.com")
			if err != nil {
				t.Fatal(err)
			}
			if err = u.UpdatePrivacy(user.Privacy{WallWrite: user.WallWriteEveryone, MessagesWrite: user.MessagesWriteEveryone, ClubsRead: tt.setting, PageRead: user.AudienceEveryone, InfoRead: user.AudienceEveryone, PhotosRead: user.AudienceEveryone, FriendsRead: user.AudienceEveryone, OnlineRead: user.AudienceEveryone}); err != nil {
				t.Fatal(err)
			}

			if got := u.ClubsVisibleTo(tt.viewer, tt.ties); got != tt.want {
				t.Fatalf("ClubsVisibleTo(%v, %+v) = %v, want %v", tt.viewer, tt.ties, got, tt.want)
			}
			if !u.ClubsVisibleTo(u.ID(), user.Ties{}) {
				t.Fatal("the owner always sees their clubs")
			}
		})
	}
}

func TestUser_ClubsReadDefaultsAndDeletedPages(t *testing.T) {
	t.Parallel()

	u, err := user.New("ann@example.com")
	if err != nil {
		t.Fatal(err)
	}

	if got := u.Privacy().ClubsRead; got != user.ClubsReadEveryone {
		t.Fatalf("new clubs setting = %q, want everyone, as openvk", got)
	}

	err = u.UpdatePrivacy(user.Privacy{WallWrite: user.WallWriteEveryone, MessagesWrite: user.MessagesWriteEveryone})
	if !errors.Is(err, user.ErrInvalidClubsRead) {
		t.Fatalf("UpdatePrivacy without ClubsRead = %v, want ErrInvalidClubsRead", err)
	}

	if err = u.UpdateProfile(user.Profile{Name: "Анна", Nickname: "ann"}); err != nil {
		t.Fatal(err)
	}
	if err = u.Deactivate("", time.Now(), time.Hour); err != nil {
		t.Fatal(err)
	}
	if u.ClubsVisibleTo(uuid.New(), user.Ties{}) {
		t.Fatal("the clubs of a deleted page show")
	}
}
