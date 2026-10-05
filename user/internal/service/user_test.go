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

func mustEmail(t *testing.T, s string) user.Email {
	t.Helper()

	email, err := user.NewEmail(s)
	if err != nil {
		t.Fatalf("NewEmail(%q): %v", s, err)
	}

	return email
}

func mustNickname(t *testing.T, s string) user.Nickname {
	t.Helper()

	nickname, err := user.NewNickname(s)
	if err != nil {
		t.Fatalf("NewNickname(%q): %v", s, err)
	}

	return nickname
}

func profile(t *testing.T, nickname string) user.Profile {
	t.Helper()

	return user.Profile{
		Name:           "Ann",
		Nickname:       mustNickname(t, nickname),
		Worldview:      user.WorldviewBuddhism,
		PoliticalViews: user.PoliticalViewsLiberal,
	}
}

// registered returns a service with a freshly signed-in user, as after
// the first VerifyCode in auth.
func registered(t *testing.T, email string) (*service.UserService, uuid.UUID) {
	t.Helper()

	svc := service.NewUserService(inmem.NewUserRepository())

	id, err := svc.GetOrCreateByEmail(context.Background(), mustEmail(t, email))
	if err != nil {
		t.Fatalf("GetOrCreateByEmail: %v", err)
	}

	return svc, id
}

func TestUserService_GetOrCreateByEmail(t *testing.T) {
	t.Parallel()

	t.Run("creates a user once and then returns the same id", func(t *testing.T) {
		t.Parallel()

		svc, first := registered(t, "ann@example.com")

		second, err := svc.GetOrCreateByEmail(context.Background(), mustEmail(t, "Ann@Example.com"))
		if err != nil {
			t.Fatalf("second call: %v", err)
		}
		if second != first {
			t.Fatalf("id changed: %s != %s", second, first)
		}
	})

	t.Run("new user has an empty profile", func(t *testing.T) {
		t.Parallel()

		svc, id := registered(t, "ann@example.com")

		u, err := svc.GetByID(context.Background(), id)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if u.IsProfileCompleted() {
			t.Fatal("profile must not be completed right after sign-in")
		}
	})

	t.Run("lost race returns the id of the winner", func(t *testing.T) {
		t.Parallel()

		winner := uuid.New()
		svc := service.NewUserService(&racingRepo{winner: winner})

		got, err := svc.GetOrCreateByEmail(context.Background(), mustEmail(t, "ann@example.com"))
		if err != nil {
			t.Fatalf("GetOrCreateByEmail: %v", err)
		}
		if got != winner {
			t.Fatalf("got %s, want winner %s", got, winner)
		}
	})

	t.Run("repository failure is propagated", func(t *testing.T) {
		t.Parallel()

		boom := errors.New("boom")
		svc := service.NewUserService(&failingRepo{err: boom})

		_, err := svc.GetOrCreateByEmail(context.Background(), mustEmail(t, "ann@example.com"))
		if !errors.Is(err, boom) {
			t.Fatalf("got %v, want %v", err, boom)
		}
	})
}

func TestUserService_UpdateProfile(t *testing.T) {
	t.Parallel()

	t.Run("fills in the profile", func(t *testing.T) {
		t.Parallel()

		svc, id := registered(t, "ann@example.com")
		ctx := context.Background()

		if _, err := svc.UpdateProfile(ctx, id, profile(t, "ann")); err != nil {
			t.Fatalf("UpdateProfile: %v", err)
		}

		u, err := svc.GetByID(ctx, id)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if !u.IsProfileCompleted() || u.Profile().Nickname != "ann" {
			t.Fatalf("profile not stored: %+v", u.Profile())
		}
	})

	t.Run("nickname is unique regardless of case", func(t *testing.T) {
		t.Parallel()

		svc, ann := registered(t, "ann@example.com")
		ctx := context.Background()

		bob, err := svc.GetOrCreateByEmail(ctx, mustEmail(t, "bob@example.com"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = svc.UpdateProfile(ctx, ann, profile(t, "ann")); err != nil {
			t.Fatal(err)
		}

		_, err = svc.UpdateProfile(ctx, bob, profile(t, "ANN"))
		if !errors.Is(err, user.ErrNicknameTaken) {
			t.Fatalf("got %v, want ErrNicknameTaken", err)
		}

		u, err := svc.GetByID(ctx, bob)
		if err != nil {
			t.Fatal(err)
		}
		if u.IsProfileCompleted() {
			t.Fatal("failed update must not change the stored user")
		}
	})

	t.Run("old nickname is released after a change", func(t *testing.T) {
		t.Parallel()

		svc, ann := registered(t, "ann@example.com")
		ctx := context.Background()

		bob, err := svc.GetOrCreateByEmail(ctx, mustEmail(t, "bob@example.com"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = svc.UpdateProfile(ctx, ann, profile(t, "ann")); err != nil {
			t.Fatal(err)
		}
		if _, err = svc.UpdateProfile(ctx, ann, profile(t, "ann_new")); err != nil {
			t.Fatal(err)
		}
		if _, err = svc.UpdateProfile(ctx, bob, profile(t, "ann")); err != nil {
			t.Fatalf("released nickname must be free: %v", err)
		}
	})

	t.Run("name and nickname are required", func(t *testing.T) {
		t.Parallel()

		svc, id := registered(t, "ann@example.com")

		_, err := svc.UpdateProfile(context.Background(), id, user.Profile{Name: "Ann"})
		if !errors.Is(err, user.ErrNicknameEmpty) {
			t.Fatalf("got %v, want ErrNicknameEmpty", err)
		}
	})

	t.Run("unknown user", func(t *testing.T) {
		t.Parallel()

		svc := service.NewUserService(inmem.NewUserRepository())

		_, err := svc.UpdateProfile(context.Background(), uuid.New(), profile(t, "ann"))
		if !errors.Is(err, user.ErrNotFound) {
			t.Fatalf("got %v, want ErrNotFound", err)
		}
	})
}

func TestUserService_GetPublicProfile(t *testing.T) {
	t.Parallel()

	svc, id := registered(t, "ann@example.com")
	ctx := context.Background()

	if _, err := svc.UpdateProfile(ctx, id, profile(t, "ann")); err != nil {
		t.Fatal(err)
	}

	got, err := svc.GetPublicProfile(ctx, mustNickname(t, "Ann"))
	if err != nil {
		t.Fatalf("GetPublicProfile: %v", err)
	}
	if got.ID != id || got.Nickname != "ann" || got.Name != "Ann" {
		t.Fatalf("got %+v", got)
	}

	if _, err = svc.GetPublicProfile(ctx, mustNickname(t, "nobody")); !errors.Is(err, user.ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

// racingRepo simulates another request registering the email between
// our lookup and our insert. Unused methods panic via the nil interface.
type racingRepo struct {
	service.UserRepository

	winner uuid.UUID
	saved  bool
}

func (r *racingRepo) Save(context.Context, *user.User) error {
	r.saved = true
	return user.ErrAlreadyExists
}

func (r *racingRepo) GetIDByEmail(context.Context, user.Email) (uuid.UUID, error) {
	if !r.saved {
		return uuid.Nil, user.ErrNotFound
	}
	return r.winner, nil
}

type failingRepo struct {
	service.UserRepository

	err error
}

func (r *failingRepo) GetIDByEmail(context.Context, user.Email) (uuid.UUID, error) {
	return uuid.Nil, r.err
}
