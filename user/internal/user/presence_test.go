package user_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestUser_MarkSeen(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	u, _ := user.New("ann@example.com")

	if u.IsOnline(now) || !u.LastSeen().IsZero() {
		t.Fatal("a new user has never been online")
	}

	u.MarkSeen(now.Add(-time.Minute))
	u.MarkSeen(now.Add(-time.Hour)) // told late: the later visit stays
	if !u.LastSeen().Equal(now.Add(-time.Minute)) || !u.IsOnline(now) {
		t.Fatalf("last seen %v", u.LastSeen())
	}

	if u.IsOnline(now.Add(user.OnlineFor)) {
		t.Fatal("online long after the visit")
	}
}

func TestUser_LastSeenFor(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	stranger, friend := uuid.New(), uuid.New()
	followed := user.Ties{Followed: true}

	ann, _ := user.New("ann@example.com")
	_ = ann.UpdateProfile(user.Profile{Name: "Ann", Nickname: "ann"})
	if err := ann.UpdatePrivacy(user.Privacy{
		WallWrite: user.WallWriteEveryone, MessagesWrite: user.MessagesWriteEveryone, ClubsRead: user.ClubsReadEveryone,
		PageRead: user.AudienceEveryone, InfoRead: user.AudienceEveryone, PhotosRead: user.AudienceEveryone,
		FriendsRead: user.AudienceEveryone, OnlineRead: user.AudienceFollowing,
	}); err != nil {
		t.Fatal(err)
	}

	// Online now: anyone sees it, whatever the setting.
	ann.MarkSeen(now.Add(-2 * time.Minute))
	if p := ann.PublicProfileFor(uuid.Nil, user.Ties{}, now); !p.Online || !p.LastSeen.Equal(now.Add(-2*time.Minute)) {
		t.Fatalf("online: %+v", p)
	}

	// A short absence is told to anyone too, as on openvk.
	if p := ann.PublicProfileFor(stranger, user.Ties{}, now.Add(24*time.Hour)); p.Online || p.LastSeen.IsZero() {
		t.Fatalf("a day away: %+v", p)
	}

	// A long one only to the audience of "Кто видит мою неактивность".
	later := now.Add(60 * 24 * time.Hour)
	for _, tt := range []struct {
		name   string
		viewer uuid.UUID
		ties   user.Ties
		shown  bool
	}{
		{"anonymous", uuid.Nil, user.Ties{}, false},
		{"a stranger", stranger, user.Ties{}, false},
		{"whom ann follows", friend, followed, true},
		{"ann", ann.ID(), user.Ties{}, true},
	} {
		if p := ann.PublicProfileFor(tt.viewer, tt.ties, later); p.LastSeen.IsZero() == tt.shown || p.Online {
			t.Errorf("%s: %+v", tt.name, p)
		}
	}

	// A deleted page tells nothing.
	_ = ann.Deactivate("", now, time.Hour)
	if p := ann.PublicProfileFor(friend, followed, now); !p.LastSeen.IsZero() || p.Online {
		t.Fatalf("deleted: %+v", p)
	}
}
