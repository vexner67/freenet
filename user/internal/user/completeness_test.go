package user_test

import (
	"slices"
	"testing"

	"github.com/vexner67/freenet/user/internal/user"
)

func TestUser_Completeness(t *testing.T) {
	t.Parallel()

	u, _ := user.New("ann@example.com")
	_ = u.UpdateProfile(user.Profile{Name: "Ann", Nickname: "ann"})

	// openvk's weights, without its rating.
	if p, unfilled := u.Completeness(); p != 10 || !slices.Equal(unfilled, []string{"interests", "email", "telegram", "status", "city"}) {
		t.Fatalf("empty page: %d %v", p, unfilled)
	}

	u.UpdateStatus("Привет")
	u.UpdateContacts(user.Contacts{City: "Москва", Telegram: "ann"})
	if p, unfilled := u.Completeness(); p != 60 || !slices.Equal(unfilled, []string{"interests", "email"}) {
		t.Fatalf("half full: %d %v", p, unfilled)
	}

	u.UpdateContacts(user.Contacts{City: "Москва", Telegram: "ann", Email: "ann@mail.example"})
	u.UpdateInterests(user.Interests{Interests: "кошки"})
	if p, unfilled := u.Completeness(); p != 100 || len(unfilled) != 0 {
		t.Fatalf("full: %d %v", p, unfilled)
	}

	if u.Profile().Status != "Привет" || u.PublicProfile().Completeness != 0 {
		t.Fatal("UpdateStatus keeps the status; only a viewer's page tells the completeness")
	}
}
