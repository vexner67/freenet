package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/auth/internal/inmem"
	"github.com/vexner67/freenet/auth/internal/service"
	"github.com/vexner67/freenet/auth/internal/session"
)

// savedSession puts a session of the user, last used that long ago, into
// the repository.
func savedSession(t *testing.T, repo *inmem.SessionRepository, userID uuid.UUID, usedAgo time.Duration) uuid.UUID {
	t.Helper()

	s, err := session.New(uuid.New(), userID, "hash", session.NewDevice("203.0.113.7", "browser"), time.Now().Add(-usedAgo))
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.Save(context.Background(), s); err != nil {
		t.Fatal(err)
	}

	return s.ID()
}

func TestSessionService(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := inmem.NewSessionRepository()
	sessions := service.NewSessionService(repo, 24*time.Hour)
	ann, bob := uuid.New(), uuid.New()

	laptop := savedSession(t, repo, ann, time.Minute)
	phone := savedSession(t, repo, ann, time.Hour)
	// Its refresh token has expired: the session cannot be continued.
	forgotten := savedSession(t, repo, ann, 25*time.Hour)
	bobs := savedSession(t, repo, bob, time.Minute)

	list, err := sessions.List(ctx, ann)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 || list[0].ID() != laptop || list[1].ID() != phone {
		t.Fatalf("List = %d sessions, want the laptop and the phone, the latest used first", len(list))
	}
	if list[0].Device().IP() != "203.0.113.7" || list[0].Device().Agent() != "browser" {
		t.Fatalf("the device of a session = %+v", list[0].Device())
	}

	if err = sessions.End(ctx, ann, bobs); !errors.Is(err, service.ErrNoSuchSession) {
		t.Fatalf("ann ended bob's session: %v", err)
	}
	if err = sessions.End(ctx, ann, uuid.New()); !errors.Is(err, service.ErrNoSuchSession) {
		t.Fatalf("End of a session that is not: %v", err)
	}
	if err = sessions.End(ctx, ann, phone); err != nil {
		t.Fatalf("End: %v", err)
	}
	if _, err = repo.Get(ctx, phone); !errors.Is(err, service.ErrSessionNotFound) {
		t.Fatalf("the ended session is still there: %v", err)
	}

	tablet := savedSession(t, repo, ann, 2*time.Hour)

	// The expired one goes too: it is the user's as well.
	if n, err := sessions.EndOthers(ctx, ann, laptop); err != nil || n != 2 {
		t.Fatalf("EndOthers = %d, %v; want the tablet and the forgotten one", n, err)
	}
	for _, id := range []uuid.UUID{tablet, forgotten} {
		if _, err = repo.Get(ctx, id); !errors.Is(err, service.ErrSessionNotFound) {
			t.Fatalf("a session survived EndOthers: %v", err)
		}
	}
	for _, id := range []uuid.UUID{laptop, bobs} {
		if _, err = repo.Get(ctx, id); err != nil {
			t.Fatalf("EndOthers ended the current session or another user's: %v", err)
		}
	}
}

// TestAuthService_SessionDevice: a session remembers where from it was
// signed in, then where from and when it was last refreshed; ending it
// stops its refresh token.
func TestAuthService_SessionDevice(t *testing.T) {
	t.Parallel()

	f := newFixture(t, nil)
	ctx := context.Background()
	sessions := service.NewSessionService(f.sessions.SessionRepository, time.Hour)

	pair := f.signIn(t)

	list, err := sessions.List(ctx, f.users.id)
	if err != nil || len(list) != 1 {
		t.Fatalf("List after the sign-in = %v, %v", list, err)
	}
	signedIn := list[0]
	if signedIn.Device() != testDevice || !signedIn.UsedAt().Equal(signedIn.CreatedAt()) {
		t.Fatalf("a new session = %+v, used %v, created %v", signedIn.Device(), signedIn.UsedAt(), signedIn.CreatedAt())
	}

	elsewhere := session.NewDevice("198.51.100.2", "Mozilla/5.0 (elsewhere)")
	pair, err = f.svc.RefreshToken(ctx, pair.Refresh, elsewhere)
	if err != nil {
		t.Fatalf("RefreshToken: %v", err)
	}

	list, err = sessions.List(ctx, f.users.id)
	if err != nil || len(list) != 1 {
		t.Fatalf("List after the refresh = %v, %v", list, err)
	}
	if got := list[0]; got.ID() != signedIn.ID() || got.Device() != elsewhere || got.UsedAt().Before(signedIn.UsedAt()) || !got.CreatedAt().Equal(signedIn.CreatedAt()) {
		t.Fatalf("a refreshed session = %+v, used %v, created %v", got.Device(), got.UsedAt(), got.CreatedAt())
	}

	if err = sessions.End(ctx, f.users.id, signedIn.ID()); err != nil {
		t.Fatalf("End: %v", err)
	}
	if _, err = f.svc.RefreshToken(ctx, pair.Refresh, elsewhere); !errors.Is(err, service.ErrSessionNotFound) {
		t.Fatalf("RefreshToken of an ended session: %v", err)
	}
}
