package user_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestUser_Verified(t *testing.T) {
	t.Parallel()

	u, _ := user.New("ann@example.com")
	if u.IsVerified() || u.PublicProfile().Verified {
		t.Fatal("a new page is verified")
	}

	v := u.WithVerified(true)
	if u.IsVerified() {
		t.Fatal("WithVerified changed the user it was called on")
	}
	if !v.IsVerified() || !v.PublicProfile().Verified || !v.ListedProfile(time.Now()).Verified {
		t.Fatal("the checkmark is not told")
	}
	if v.WithVerified(false).PublicProfile().Verified {
		t.Fatal("a taken checkmark is told")
	}
}

func TestUser_VerifiedHiddenAndDeleted(t *testing.T) {
	t.Parallel()

	hidden := user.Restore(user.Stored{
		ID:       uuid.New(),
		Privacy:  user.Privacy{PageRead: user.AudienceUsers, InfoRead: user.AudienceEveryone},
		Verified: true,
	})
	if p := hidden.PublicProfileFor(uuid.Nil, user.Ties{}, time.Now()); !p.PageHidden || !p.Verified {
		t.Fatalf("a hidden page: %+v", p)
	}

	deleted, _ := user.New("bob@example.com")
	deleted = deleted.WithVerified(true)
	if err := deleted.Deactivate("", time.Now(), time.Hour); err != nil {
		t.Fatal(err)
	}
	if deleted.PublicProfile().Verified {
		t.Fatal("a deleted page tells the checkmark")
	}

	hiddenDeleted := user.Restore(user.Stored{
		ID:           uuid.New(),
		Privacy:      user.Privacy{PageRead: user.AudienceUsers},
		Deactivation: user.Deactivation{At: time.Now(), PurgeAt: time.Now().Add(time.Hour)},
		Verified:     true,
	})
	if hiddenDeleted.PublicProfileFor(uuid.Nil, user.Ties{}, time.Now()).Verified {
		t.Fatal("a deleted hidden page tells the checkmark")
	}
}
