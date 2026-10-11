package postgres_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vexner67/freenet/pkg/pgpool"
	"github.com/vexner67/freenet/user/internal/postgres"
	"github.com/vexner67/freenet/user/internal/user"
)

// Integration test: needs a database with migrations applied, e.g.
// TEST_DATABASE_URL=postgres://... go test ./internal/postgres
func newRepo(t *testing.T) (*postgres.UserRepository, *pgxpool.Pool) {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("postgres.New: %v", err)
	}
	t.Cleanup(pool.Close)

	return postgres.NewUserRepository(pool), pool
}

// unique keeps parallel runs against one database from colliding.
func unique(prefix string) string {
	return prefix + uuid.NewString()[:8]
}

func TestUserRepository(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()

	email := user.Email(unique("ann") + "@example.com")
	u, err := user.New(email)
	if err != nil {
		t.Fatal(err)
	}

	if err = repo.Save(ctx, u); err != nil {
		t.Fatalf("Save: %v", err)
	}

	t.Run("email is unique regardless of case", func(t *testing.T) {
		dup, err := user.New(user.Email(strings.ToUpper(email.String())))
		if err != nil {
			t.Fatal(err)
		}

		if err = repo.Save(ctx, dup); !errors.Is(err, user.ErrAlreadyExists) {
			t.Fatalf("got %v, want ErrAlreadyExists", err)
		}
	})

	t.Run("empty profile round-trips with a NULL birthday", func(t *testing.T) {
		got, err := repo.GetByID(ctx, u.ID())
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.Email() != email || got.IsProfileCompleted() || !got.Profile().Birthday.Time().IsZero() {
			t.Fatalf("got %+v %+v", got.Email(), got.Profile())
		}

		id, err := repo.GetIDByEmail(ctx, email)
		if err != nil || id != u.ID() {
			t.Fatalf("GetIDByEmail = %s, %v", id, err)
		}
	})

	nickname := user.Nickname(unique("ann_"))
	birthday, err := user.NewBirthday(time.Date(1990, 5, 17, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}

	t.Run("update stores the whole profile", func(t *testing.T) {
		if err := u.UpdateProfile(user.Profile{
			Name:           "Ann",
			Surname:        "Lee",
			Nickname:       nickname,
			Status:         "hello",
			Gender:         user.GenderFemale,
			Relationship:   user.RelationshipMarried,
			Worldview:      user.WorldviewBuddhism,
			PoliticalViews: user.PoliticalViewsLiberal,
			Birthday:       birthday,
		}); err != nil {
			t.Fatal(err)
		}

		if err := repo.Update(ctx, u); err != nil {
			t.Fatalf("Update: %v", err)
		}

		got, err := repo.GetByNickname(ctx, user.Nickname(strings.ToUpper(nickname.String())))
		if err != nil {
			t.Fatalf("GetByNickname: %v", err)
		}
		if got.Profile() != u.Profile() {
			t.Fatalf("got %+v\nwant %+v", got.Profile(), u.Profile())
		}

		// Which nicknames are someone's, whatever the case, as written.
		asked := []user.Nickname{user.Nickname(strings.ToUpper(nickname.String())), "nobody_here", nickname}
		known, err := repo.Nicknames(ctx, asked)
		if err != nil || len(known) != 1 || known[0] != nickname {
			t.Fatalf("Nicknames = %v, %v; want only %v", known, err, nickname)
		}
	})

	t.Run("privacy settings round-trip", func(t *testing.T) {
		privacy := user.Privacy{ShowBirthday: true, ShowPoliticalViews: true, WallWrite: user.WallWriteOnlyMe, MessagesWrite: user.MessagesWriteNobody, ClubsRead: user.ClubsReadOnlyMe,
			PageRead: user.AudienceUsers, InfoRead: user.AudienceFollowing, PhotosRead: user.AudienceOnlyMe, FriendsRead: user.AudienceUsers, OnlineRead: user.AudienceFollowing,
			NotesRead: user.AudienceOnlyMe}
		if err := u.UpdatePrivacy(privacy); err != nil {
			t.Fatal(err)
		}

		if err := repo.Update(ctx, u); err != nil {
			t.Fatalf("Update: %v", err)
		}

		got, err := repo.GetByID(ctx, u.ID())
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.Privacy() != privacy {
			t.Fatalf("got %+v, want %+v", got.Privacy(), privacy)
		}
	})

	t.Run("nickname is unique regardless of case", func(t *testing.T) {
		other, err := user.New(user.Email(unique("bob") + "@example.com"))
		if err != nil {
			t.Fatal(err)
		}
		if err = repo.Save(ctx, other); err != nil {
			t.Fatal(err)
		}

		if err = other.UpdateProfile(user.Profile{Name: "Bob", Nickname: user.Nickname(strings.ToUpper(nickname.String()))}); err != nil {
			t.Fatal(err)
		}

		if err = repo.Update(ctx, other); !errors.Is(err, user.ErrNicknameTaken) {
			t.Fatalf("got %v, want ErrNicknameTaken", err)
		}
	})

	t.Run("follows", func(t *testing.T) {
		follows := postgres.NewFollowRepository(pool)

		other, err := user.New(user.Email(unique("fan") + "@example.com"))
		if err != nil {
			t.Fatal(err)
		}
		if err = repo.Save(ctx, other); err != nil {
			t.Fatal(err)
		}

		for range 2 { // idempotent
			if err = follows.Follow(ctx, other.ID(), u.ID()); err != nil {
				t.Fatalf("Follow: %v", err)
			}
		}

		if ok, err := follows.IsFollowing(ctx, other.ID(), u.ID()); err != nil || !ok {
			t.Fatalf("IsFollowing = %v, %v", ok, err)
		}
		if followers, following, err := follows.Counts(ctx, u.ID()); err != nil || followers != 1 || following != 0 {
			t.Fatalf("Counts = %d, %d, %v", followers, following, err)
		}

		list, err := follows.ListFollowers(ctx, u.ID(), uuid.Nil, 10)
		if err != nil || len(list) != 1 || list[0].User.ID() != other.ID() || list[0].User.Email() != other.Email() {
			t.Fatalf("ListFollowers = %+v, %v", list, err)
		}
		if after, err := follows.ListFollowers(ctx, u.ID(), list[0].FollowID, 10); err != nil || len(after) != 0 {
			t.Fatalf("after the cursor: %+v, %v", after, err)
		}

		ids, err := follows.FollowingIDs(ctx, other.ID(), 10)
		if err != nil || len(ids) != 1 || ids[0] != u.ID() {
			t.Fatalf("FollowingIDs = %v, %v", ids, err)
		}

		if err = follows.Follow(ctx, other.ID(), uuid.New()); !errors.Is(err, user.ErrNotFound) {
			t.Fatalf("follow a missing user: %v", err)
		}

		if err = follows.Unfollow(ctx, other.ID(), u.ID()); err != nil {
			t.Fatal(err)
		}
		if ok, _ := follows.IsFollowing(ctx, other.ID(), u.ID()); ok {
			t.Fatal("still following after Unfollow")
		}

		got, err := repo.GetByIDs(ctx, []uuid.UUID{u.ID(), other.ID(), uuid.New()})
		if err != nil || len(got) != 2 {
			t.Fatalf("GetByIDs = %d users, %v", len(got), err)
		}
	})

	t.Run("avatar and search", func(t *testing.T) {
		avatar := uuid.New()
		u.SetAvatar(avatar)
		if err := repo.Update(ctx, u); err != nil {
			t.Fatal(err)
		}

		got, err := repo.GetByID(ctx, u.ID())
		if err != nil || got.AvatarID() != avatar {
			t.Fatalf("avatar round trip: %v, %v", got.AvatarID(), err)
		}

		// The nickname is unique per run, so it finds exactly this user;
		// the name words come in another order and case.
		q, err := user.NewSearchQuery("LEE "+nickname.String(), user.SearchFilters{})
		if err != nil {
			t.Fatal(err)
		}
		found, err := repo.Search(ctx, q, uuid.Nil, 10)
		if err != nil || len(found) != 1 || found[0].ID() != u.ID() || found[0].AvatarID() != avatar {
			t.Fatalf("Search = %v, %v", found, err)
		}

		percent, err := user.NewSearchQuery("%", user.SearchFilters{})
		if err != nil {
			t.Fatal(err)
		}
		if found, err = repo.Search(ctx, percent, uuid.Nil, 10); err != nil || len(found) != 0 {
			t.Fatalf("%% must match itself, not everything: %d users, %v", len(found), err)
		}

		u.SetAvatar(uuid.Nil)
		if err = repo.Update(ctx, u); err != nil {
			t.Fatal(err)
		}
		if got, err = repo.GetByID(ctx, u.ID()); err != nil || got.AvatarID() != uuid.Nil {
			t.Fatalf("removed avatar is NULL: %v, %v", got.AvatarID(), err)
		}
	})

	t.Run("not found", func(t *testing.T) {
		if _, err := repo.GetByID(ctx, uuid.New()); !errors.Is(err, user.ErrNotFound) {
			t.Fatalf("GetByID: got %v", err)
		}
		if _, err := repo.GetByNickname(ctx, "nobody_here"); !errors.Is(err, user.ErrNotFound) {
			t.Fatalf("GetByNickname: got %v", err)
		}
		if _, err := repo.GetIDByEmail(ctx, "nobody@example.com"); !errors.Is(err, user.ErrNotFound) {
			t.Fatalf("GetIDByEmail: got %v", err)
		}

		ghost, err := user.New("ghost@example.com")
		if err != nil {
			t.Fatal(err)
		}
		if err = repo.Update(ctx, ghost); !errors.Is(err, user.ErrNotFound) {
			t.Fatalf("Update: got %v", err)
		}
	})
}
