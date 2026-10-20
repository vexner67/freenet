package user_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestUser_ForeignAgent(t *testing.T) {
	t.Parallel()

	u, _ := user.New("ann@example.com")
	if u.IsForeignAgent() || u.PublicProfile().ForeignAgent {
		t.Fatal("a new page is a foreign agent's")
	}

	agent := u.WithForeignAgent(true)
	if u.IsForeignAgent() {
		t.Fatal("WithForeignAgent changed the user it was called on")
	}
	if !agent.IsForeignAgent() || !agent.PublicProfile().ForeignAgent || !agent.ListedProfile(time.Now()).ForeignAgent {
		t.Fatal("the plaque is not told")
	}
	if agent.WithForeignAgent(false).PublicProfile().ForeignAgent {
		t.Fatal("a taken plaque is told")
	}
}

func TestUser_ForeignAgentHiddenClosedAndDeleted(t *testing.T) {
	t.Parallel()

	hidden := user.Restore(user.Stored{
		ID:           uuid.New(),
		Privacy:      user.Privacy{PageRead: user.AudienceUsers, InfoRead: user.AudienceOnlyMe},
		ForeignAgent: true,
	})
	if p := hidden.PublicProfileFor(uuid.Nil, user.Ties{}, time.Now()); !p.PageHidden || !p.ForeignAgent {
		t.Fatalf("a hidden page: %+v", p)
	}
	if p := hidden.PublicProfile(); !p.InfoHidden || !p.ForeignAgent {
		t.Fatalf("a page with hidden information: %+v", p)
	}

	deleted := user.Restore(user.Stored{
		ID:           uuid.New(),
		Deactivation: user.Deactivation{At: time.Now(), PurgeAt: time.Now().Add(time.Hour)},
		ForeignAgent: true,
	})
	if deleted.PublicProfile().ForeignAgent || deleted.PublicProfileFor(uuid.Nil, user.Ties{}, time.Now()).ForeignAgent {
		t.Fatal("a deleted page tells the plaque")
	}
}
