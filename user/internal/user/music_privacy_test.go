package user_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestUser_MusicPrivacy(t *testing.T) {
	t.Parallel()

	u, err := user.New("ann@example.com")
	if err != nil {
		t.Fatal(err)
	}

	if a := u.AccessFor(uuid.Nil, user.Ties{}, time.Now()); !a.Audios || !a.Listening {
		t.Fatalf("the songs and what plays are shown to everyone by default: %+v", a)
	}

	privacy := u.Privacy()
	privacy.AudiosRead = user.AudienceFollowing
	privacy.ListeningRead = user.AudienceOnlyMe
	if err = u.UpdatePrivacy(privacy); err != nil {
		t.Fatal(err)
	}

	viewer := uuid.New()
	tests := []struct {
		name           string
		viewer         uuid.UUID
		ties           user.Ties
		audios, listen bool
	}{
		{"a user", viewer, user.Ties{}, false, false},
		{"whom ann follows", viewer, user.Ties{Followed: true}, true, false},
		{"ann", u.ID(), user.Ties{}, true, true},
	}
	for _, tt := range tests {
		if a := u.AccessFor(tt.viewer, tt.ties, time.Now()); a.Audios != tt.audios || a.Listening != tt.listen {
			t.Errorf("%s: %+v, want songs %v, listening %v", tt.name, a, tt.audios, tt.listen)
		}
	}

	// A client that does not know the settings leaves them to everyone.
	privacy.AudiosRead, privacy.ListeningRead = "", ""
	if err = u.UpdatePrivacy(privacy); err != nil || u.Privacy().AudiosRead != user.AudienceEveryone || u.Privacy().ListeningRead != user.AudienceEveryone {
		t.Fatalf("empty settings: %v, %+v", err, u.Privacy())
	}

	privacy.ListeningRead = "friends"
	if err = u.UpdatePrivacy(privacy); !errors.Is(err, user.ErrInvalidAudience) {
		t.Fatalf("an unknown setting: %v", err)
	}
}
