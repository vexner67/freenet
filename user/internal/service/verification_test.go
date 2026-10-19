package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/user/internal/inmem"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
)

func TestVerificationService_Set(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	w := newWalletWorld(t)
	admins, _ := user.NewAdmins([]string{"admin@example.com"})
	told := &fakeVerifiedNotifier{}
	verification := service.NewVerificationService(service.VerificationDeps{
		Users: w.users, Verifications: w.users, Tx: inmem.TxManager{}, Notifier: told, Admins: admins,
	})
	admin, ann := w.person(t, "admin"), w.person(t, "ann")

	verified := func() bool {
		u, err := w.users.GetByID(ctx, ann)
		if err != nil {
			t.Fatal(err)
		}
		return u.IsVerified()
	}

	// Only the administrators give the checkmark.
	if err := verification.Set(ctx, ann, ann, true); !errors.Is(err, user.ErrNotAdmin) {
		t.Fatalf("Set by ann: %v", err)
	}
	if err := verification.Set(ctx, admin, uuid.New(), true); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("an unknown page: %v", err)
	}

	if err := verification.Set(ctx, admin, ann, true); err != nil || !verified() {
		t.Fatalf("Set = %v, verified %v", err, verified())
	}
	if told.verified[ann] != admin {
		t.Fatalf("ann was not told by the admin: %v", told.verified)
	}

	// Saving the profile keeps the checkmark given meanwhile.
	if _, err := w.userSvc.UpdateProfile(ctx, ann, user.Profile{Name: "Анна", Nickname: "ann"}, ""); err != nil || !verified() {
		t.Fatalf("UpdateProfile = %v, verified %v", err, verified())
	}

	q, _ := user.NewSearchQuery("", user.SearchFilters{Verified: true})
	page, err := w.userSvc.Search(ctx, q, uuid.Nil, 10)
	if err != nil || len(page.Users) != 1 || page.Users[0].ID != ann || !page.Users[0].Verified {
		t.Fatalf("the verified pages: %+v, %v", page, err)
	}

	if err = verification.Set(ctx, admin, ann, false); err != nil || verified() {
		t.Fatalf("taking it: %v, verified %v", err, verified())
	}
	if _, ok := told.verified[ann]; ok {
		t.Fatal("the notification was not taken back")
	}

	// An administrator confirming their own page tells nobody.
	if err = verification.Set(ctx, admin, admin, true); err != nil || len(told.verified) != 0 {
		t.Fatalf("Set of their own page = %v, told %v", err, told.verified)
	}
}

// fakeVerifiedNotifier remembers who was told of a checkmark, and by whom.
type fakeVerifiedNotifier struct {
	verified map[uuid.UUID]uuid.UUID
}

func (n *fakeVerifiedNotifier) Verified(_ context.Context, adminID, userID uuid.UUID) error {
	if n.verified == nil {
		n.verified = make(map[uuid.UUID]uuid.UUID)
	}
	n.verified[userID] = adminID
	return nil
}

func (n *fakeVerifiedNotifier) Unverified(_ context.Context, _, userID uuid.UUID) error {
	delete(n.verified, userID)
	return nil
}
