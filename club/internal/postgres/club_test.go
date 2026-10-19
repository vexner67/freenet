package postgres_test

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/club/internal/club"
	"github.com/vexner67/freenet/club/internal/postgres"
	"github.com/vexner67/freenet/club/internal/service"
	"github.com/vexner67/freenet/pkg/pgpool"
)

// Integration test: needs a database with migrations applied, e.g.
// TEST_DATABASE_URL=postgres://... go test ./internal/postgres
func TestRepositories(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	clubs := postgres.NewClubRepository(pool)
	members := postgres.NewMemberRepository(pool)
	ann, bob := uuid.New(), uuid.New()
	// Addresses are unique across the database, which other runs share.
	address := club.Address("cats" + uuid.NewString()[:8])

	create := func(owner uuid.UUID) *club.Club {
		t.Helper()
		c, err := club.New("Любители котов", "про котов\nи кошек", time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err = clubs.Create(ctx, c, owner); err != nil {
			t.Fatalf("Create: %v", err)
		}
		return c
	}

	c := create(ann)

	t.Run("a club reads back with its owner", func(t *testing.T) {
		got, err := clubs.Get(ctx, c.ID())
		if err != nil || got.Members() != 1 || got.Name() != c.Name() || got.Description() != c.Description() || !got.CreatedAt().Equal(c.CreatedAt()) {
			t.Fatalf("Get = %+v, %v", got, err)
		}
		if owner, err := members.Owner(ctx, c.ID()); err != nil || owner != ann {
			t.Fatalf("Owner = %v, %v", owner, err)
		}
		if role, err := members.Role(ctx, c.ID(), ann); err != nil || role != club.RoleOwner {
			t.Fatalf("Role = %q, %v", role, err)
		}
		if _, err = clubs.Get(ctx, uuid.New()); !errors.Is(err, club.ErrNotFound) {
			t.Fatalf("Get missing: %v", err)
		}
	})

	t.Run("address", func(t *testing.T) {
		edited, err := c.Edited(club.RoleOwner, club.Changes{Name: "Кошки", Address: address, Wall: club.WallClosed, AdminsDisplay: club.AdminsDisplayCreator})
		if err != nil {
			t.Fatal(err)
		}
		if err = clubs.Update(ctx, edited); err != nil {
			t.Fatalf("Update: %v", err)
		}

		got, err := clubs.GetByAddress(ctx, club.Address("CATS"+address.String()[4:]))
		if err != nil || got.ID() != c.ID() || got.Wall() != club.WallClosed || got.Description() != "" {
			t.Fatalf("GetByAddress = %+v, %v", got, err)
		}

		other := create(uuid.New())
		taken, err := other.Edited(club.RoleOwner, club.Changes{Name: "Другие", Address: club.Address("CATS" + address.String()[4:]), Wall: club.WallOpen, AdminsDisplay: club.AdminsDisplayCreator})
		if err != nil {
			t.Fatal(err)
		}
		if err = clubs.Update(ctx, taken); !errors.Is(err, club.ErrAddressTaken) {
			t.Fatalf("taken address: %v", err)
		}

		// No address is no address: any number of clubs have none.
		none, err := other.Edited(club.RoleOwner, club.Changes{Name: "Другие", Wall: club.WallOpen, AdminsDisplay: club.AdminsDisplayCreator})
		if err != nil {
			t.Fatal(err)
		}
		if err = clubs.Update(ctx, none); err != nil {
			t.Fatalf("no address: %v", err)
		}

		many, err := clubs.GetMany(ctx, []uuid.UUID{c.ID(), other.ID(), uuid.New()})
		if err != nil || len(many) != 2 {
			t.Fatalf("GetMany = %v, %v", many, err)
		}
	})

	t.Run("members", func(t *testing.T) {
		if err := members.Join(ctx, c.ID(), bob); err != nil {
			t.Fatal(err)
		}
		if err := members.Join(ctx, c.ID(), bob); err != nil {
			t.Fatalf("join twice: %v", err)
		}
		if err := members.Join(ctx, uuid.New(), bob); !errors.Is(err, club.ErrNotFound) {
			t.Fatalf("join a missing club: %v", err)
		}
		if got, _ := clubs.Get(ctx, c.ID()); got.Members() != 2 {
			t.Fatalf("members = %d, want 2", got.Members())
		}

		list, err := members.List(ctx, c.ID(), uuid.Nil, 10, service.AllMembers)
		if err != nil || len(list) != 2 || list[0].UserID != bob || list[1].UserID != ann || list[1].Role != club.RoleOwner {
			t.Fatalf("List = %+v, %v; the newest first", list, err)
		}
		if older, err := members.List(ctx, c.ID(), list[0].ID, 10, service.AllMembers); err != nil || len(older) != 1 || older[0].UserID != ann {
			t.Fatalf("List after cursor = %+v, %v", older, err)
		}
		if admins, err := members.List(ctx, c.ID(), uuid.Nil, 10, service.Managers); err != nil || len(admins) != 1 || admins[0].UserID != ann {
			t.Fatalf("admins = %+v, %v", admins, err)
		}

		if err = members.SetRole(ctx, c.ID(), bob, club.RoleAdmin); err != nil {
			t.Fatal(err)
		}
		if role, _ := members.Role(ctx, c.ID(), bob); role != club.RoleAdmin {
			t.Fatalf("bob = %q", role)
		}
		if err = members.SetRole(ctx, c.ID(), ann, club.RoleMember); !errors.Is(err, club.ErrNotMember) {
			t.Fatalf("demote the owner: %v", err)
		}

		mine, err := members.ListClubs(ctx, bob, uuid.Nil, 10, service.ClubFilter{})
		if err != nil || len(mine) != 1 || mine[0].Club.ID() != c.ID() || mine[0].Role != club.RoleAdmin || mine[0].Club.Members() != 2 {
			t.Fatalf("ListClubs = %+v, %v", mine, err)
		}

		// The owner never leaves, whatever the caller checked.
		if err = members.Leave(ctx, c.ID(), ann); err != nil {
			t.Fatal(err)
		}
		if err = members.Leave(ctx, c.ID(), bob); err != nil {
			t.Fatal(err)
		}
		if got, _ := clubs.Get(ctx, c.ID()); got.Members() != 1 {
			t.Fatalf("members after leaving = %d, want the owner", got.Members())
		}
		if role, _ := members.Role(ctx, c.ID(), bob); role != club.RoleNone {
			t.Fatalf("bob after leaving = %q", role)
		}
	})

	// The count changes in the statement that adds or removes a member.
	t.Run("concurrent joins and leaves", func(t *testing.T) {
		d := create(ann)
		people := make([]uuid.UUID, 20)
		for i := range people {
			people[i] = uuid.New()
		}

		var wg sync.WaitGroup
		for _, p := range people {
			wg.Go(func() {
				if err := members.Join(ctx, d.ID(), p); err != nil {
					t.Error(err)
				}
				if err := members.Join(ctx, d.ID(), p); err != nil {
					t.Error(err)
				}
			})
		}
		wg.Wait()
		for _, p := range people[:5] {
			wg.Go(func() {
				if err := members.Leave(ctx, d.ID(), p); err != nil {
					t.Error(err)
				}
			})
		}
		wg.Wait()

		if got, _ := clubs.Get(ctx, d.ID()); got.Members() != 16 {
			t.Fatalf("members = %d, want 16", got.Members())
		}
	})

	t.Run("erasing a user", func(t *testing.T) {
		e := create(ann)
		if err := members.Join(ctx, e.ID(), bob); err != nil {
			t.Fatal(err)
		}

		if err := members.DeleteByUser(ctx, ann); err != nil {
			t.Fatal(err)
		}

		// The only one left inherits the club.
		got, err := clubs.Get(ctx, e.ID())
		if err != nil || got.Members() != 1 {
			t.Fatalf("club after its owner is erased = %+v, %v", got, err)
		}
		if owner, err := members.Owner(ctx, e.ID()); err != nil || owner != bob {
			t.Fatalf("Owner = %v, %v; want bob", owner, err)
		}

		// Erased too, he leaves it to nobody.
		if err = members.DeleteByUser(ctx, bob); err != nil {
			t.Fatal(err)
		}
		if owner, err := members.Owner(ctx, e.ID()); err != nil || owner != uuid.Nil {
			t.Fatalf("Owner of an empty club = %v, %v; want none", owner, err)
		}
	})

	t.Run("an erased owner's club goes to its oldest admin", func(t *testing.T) {
		cat, dan, eve := uuid.New(), uuid.New(), uuid.New()
		e, other := create(ann), create(ann)

		// Bob is in it longest, cat and dan run it, cat longer.
		for _, user := range []uuid.UUID{bob, cat, dan, eve} {
			if err := members.Join(ctx, e.ID(), user); err != nil {
				t.Fatal(err)
			}
		}
		for _, user := range []uuid.UUID{dan, cat} {
			if err := members.SetRole(ctx, e.ID(), user, club.RoleAdmin); err != nil {
				t.Fatal(err)
			}
		}

		if err := members.DeleteByUser(ctx, ann); err != nil {
			t.Fatal(err)
		}
		if owner, err := members.Owner(ctx, e.ID()); err != nil || owner != cat {
			t.Fatalf("Owner = %v, %v; want cat, the oldest admin", owner, err)
		}
		if n, err := members.CountAdmins(ctx, e.ID(), false); err != nil || n != 2 {
			t.Fatalf("CountAdmins = %d, %v; want cat and dan", n, err)
		}
		if got, err := clubs.Get(ctx, e.ID()); err != nil || got.Members() != 4 {
			t.Fatalf("members = %d, %v; want 4", got.Members(), err)
		}
		if owner, err := members.Owner(ctx, other.ID()); err != nil || owner != uuid.Nil {
			t.Fatalf("Owner of a club nobody else is in = %v, %v", owner, err)
		}

		// Erasing one who owns nothing gives nothing away.
		if err := members.DeleteByUser(ctx, eve); err != nil {
			t.Fatal(err)
		}
		if owner, _ := members.Owner(ctx, e.ID()); owner != cat {
			t.Fatalf("Owner after a member was erased = %v", owner)
		}
	})
}

func TestRepositories_AvatarSearchAndMenu(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	clubs := postgres.NewClubRepository(pool)
	members := postgres.NewMemberRepository(pool)
	ann, bob := uuid.New(), uuid.New()
	// Words unique to this run: the database is shared with other runs.
	word := "кот" + uuid.NewString()[:6]

	create := func(owner uuid.UUID, name string) *club.Club {
		t.Helper()
		c, err := club.New(club.Name(name), "", time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err = clubs.Create(ctx, c, owner); err != nil {
			t.Fatal(err)
		}
		return c
	}

	cats := create(ann, "Любители "+word)
	other := create(bob, "Собаки")
	if err = members.Join(ctx, cats.ID(), bob); err != nil {
		t.Fatal(err)
	}

	photo := uuid.New()
	withPhoto, err := cats.WithAvatar(club.RoleOwner, photo)
	if err != nil {
		t.Fatal(err)
	}
	if err = clubs.SetAvatar(ctx, withPhoto); err != nil {
		t.Fatal(err)
	}
	if got, err := clubs.Get(ctx, cats.ID()); err != nil || got.AvatarID() != photo {
		t.Fatalf("Get after SetAvatar = %+v, %v", got, err)
	}
	noPhoto, _ := cats.WithAvatar(club.RoleOwner, uuid.Nil)
	if err = clubs.SetAvatar(ctx, noPhoto); err != nil {
		t.Fatal(err)
	}
	if got, _ := clubs.Get(ctx, cats.ID()); got.AvatarID() != uuid.Nil {
		t.Fatal("the photo stays")
	}

	// Each side on its own: one does not undo the other.
	left, right := uuid.New(), uuid.New()
	withLeft, _ := cats.WithBackdrop(club.RoleOwner, club.BackdropLeft, left)
	withRight, _ := cats.WithBackdrop(club.RoleOwner, club.BackdropRight, right)
	if err = clubs.SetBackdrop(ctx, withLeft, club.BackdropLeft); err != nil {
		t.Fatal(err)
	}
	if err = clubs.SetBackdrop(ctx, withRight, club.BackdropRight); err != nil {
		t.Fatal(err)
	}
	if got, err := clubs.Get(ctx, cats.ID()); err != nil || got.Backdrop() != (club.Backdrop{Left: left, Right: right}) {
		t.Fatalf("Get after SetBackdrop = %+v, %v", got, err)
	}
	noLeft, _ := cats.WithBackdrop(club.RoleOwner, club.BackdropLeft, uuid.Nil)
	if err = clubs.SetBackdrop(ctx, noLeft, club.BackdropLeft); err != nil {
		t.Fatal(err)
	}
	if got, _ := clubs.Get(ctx, cats.ID()); got.Backdrop() != (club.Backdrop{Right: right}) {
		t.Fatalf("after taking the left away = %+v", got.Backdrop())
	}

	q, err := club.NewSearchQuery(strings.ToUpper(word) + " любит")
	if err != nil {
		t.Fatal(err)
	}
	if found, err := clubs.Search(ctx, q, uuid.Nil, 10); err != nil || len(found) != 1 || found[0].ID() != cats.ID() {
		t.Fatalf("Search = %v, %v", found, err)
	}
	pct, _ := club.NewSearchQuery("%")
	if found, err := clubs.Search(ctx, pct, uuid.Nil, 10); err != nil || len(found) != 0 {
		t.Fatalf("%% matches itself only: %v, %v", found, err)
	}

	// The checkmark: only SetVerified gives it, Update keeps it.
	verifiedQ, _ := club.NewVerifiedQuery(word)
	if found, err := clubs.Search(ctx, verifiedQ, uuid.Nil, 10); err != nil || len(found) != 0 {
		t.Fatalf("Search of the verified before = %v, %v", found, err)
	}
	if err = clubs.SetVerified(ctx, cats.ID(), true); err != nil {
		t.Fatal(err)
	}
	if err = clubs.Update(ctx, cats); err != nil {
		t.Fatal(err)
	}
	if found, err := clubs.Search(ctx, verifiedQ, uuid.Nil, 10); err != nil || len(found) != 1 || !found[0].IsVerified() {
		t.Fatalf("Search of the verified = %v, %v", found, err)
	}
	if err = clubs.SetVerified(ctx, uuid.New(), true); !errors.Is(err, club.ErrNotFound) {
		t.Fatalf("SetVerified of no club = %v", err)
	}

	if n, err := members.CountClubs(ctx, bob, service.ClubFilter{}); err != nil || n != 2 {
		t.Fatalf("CountClubs = %d, %v", n, err)
	}
	if managed, err := members.ListClubs(ctx, bob, uuid.Nil, 10, service.ClubFilter{Managed: true}); err != nil || len(managed) != 1 || managed[0].Club.ID() != other.ID() {
		t.Fatalf("managed = %+v, %v", managed, err)
	}

	if ok, err := members.Pin(ctx, cats.ID(), bob, true, 10); err != nil || !ok {
		t.Fatalf("Pin by a member = %v, %v; not refused for a full menu", ok, err)
	}
	if pinned, _ := members.Pinned(ctx, bob); len(pinned) != 0 {
		t.Fatalf("a member's pin counts: %v", pinned)
	}
	if ok, err := members.Pin(ctx, other.ID(), bob, true, 1); err != nil || !ok {
		t.Fatalf("Pin = %v, %v", ok, err)
	}
	if ok, err := members.Pin(ctx, other.ID(), bob, true, 1); err != nil || !ok {
		t.Fatalf("Pin twice at the limit = %v, %v", ok, err)
	}
	third := create(bob, "Хомяки")
	if ok, err := members.Pin(ctx, third.ID(), bob, true, 1); err != nil || ok {
		t.Fatalf("Pin over the limit = %v, %v", ok, err)
	}
	pinned, err := members.Pinned(ctx, bob)
	if err != nil || len(pinned) != 1 || pinned[0].ID() != other.ID() {
		t.Fatalf("Pinned = %v, %v", pinned, err)
	}
	if ok, err := members.Pin(ctx, other.ID(), bob, false, 1); err != nil || !ok {
		t.Fatalf("Unpin = %v, %v", ok, err)
	}
}

func TestClubRepository_Delete(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	clubs := postgres.NewClubRepository(pool)
	members := postgres.NewMemberRepository(pool)
	ann, bob := uuid.New(), uuid.New()

	c, err := club.New("Кошки", "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = clubs.Create(ctx, c, ann); err != nil {
		t.Fatal(err)
	}
	if err = members.Join(ctx, c.ID(), bob); err != nil {
		t.Fatal(err)
	}

	if err = clubs.Delete(ctx, c.ID(), bob); !errors.Is(err, club.ErrCannotDelete) {
		t.Fatalf("Delete by a member = %v", err)
	}
	if err = clubs.Delete(ctx, c.ID(), ann); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err = clubs.Get(ctx, c.ID()); !errors.Is(err, club.ErrNotFound) {
		t.Fatalf("Get after Delete = %v", err)
	}
	if role, err := members.Role(ctx, c.ID(), bob); err != nil || role != club.RoleNone {
		t.Fatalf("the member after Delete = %q, %v", role, err)
	}
}

func TestRepositories_SettingsAndTransfer(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	clubs := postgres.NewClubRepository(pool)
	members := postgres.NewMemberRepository(pool)
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()

	c, err := club.New("Любители котов", "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = clubs.Create(ctx, c, ann); err != nil {
		t.Fatal(err)
	}
	if got, err := clubs.Get(ctx, c.ID()); err != nil || got.AdminsDisplay() != club.AdminsDisplayCreator || got.Website() != "" {
		t.Fatalf("a new club = %+v, %v; showing its creator", got, err)
	}

	edited, err := c.Edited(club.RoleOwner, club.Changes{
		Name: c.Name(), Website: "https://cats.example", Wall: club.WallSuggest, AdminsDisplay: club.AdminsDisplayAll,
		OpenTopics: true, TopicsAboveWall: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = clubs.Update(ctx, edited); err != nil {
		t.Fatal(err)
	}
	if got, err := clubs.Get(ctx, c.ID()); err != nil || got.Website() != "https://cats.example" || got.AdminsDisplay() != club.AdminsDisplayAll ||
		got.Wall() != club.WallSuggest || !got.OpenTopics() || !got.TopicsAboveWall() {
		t.Fatalf("Get after Update = %+v, %v", got, err)
	}

	for _, id := range []uuid.UUID{bob, cat} {
		if err = members.Join(ctx, c.ID(), id); err != nil {
			t.Fatal(err)
		}
	}
	if err = members.SetRole(ctx, c.ID(), bob, club.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if n, err := members.CountAdmins(ctx, c.ID(), false); err != nil || n != 2 {
		t.Fatalf("CountAdmins = %d, %v", n, err)
	}

	// Titles and hiding: only of those who run the club.
	if err = members.Describe(ctx, c.ID(), cat, described("кот", true)); !errors.Is(err, club.ErrNotManager) {
		t.Fatalf("Describe a member: %v", err)
	}
	// The title and the hiding change apart: neither undoes the other.
	title, hidden := club.Title("кормит котов"), true
	if err = members.Describe(ctx, c.ID(), bob, club.SeatChange{Title: &title}); err != nil {
		t.Fatal(err)
	}
	if err = members.Describe(ctx, c.ID(), bob, club.SeatChange{Hidden: &hidden}); err != nil {
		t.Fatal(err)
	}
	if err = members.Describe(ctx, c.ID(), bob, club.SeatChange{}); err != nil {
		t.Fatal(err)
	}
	// A title is written only over the one the change started from.
	cook, none := club.Title("повар"), club.Title("")
	if err = members.Describe(ctx, c.ID(), bob, club.SeatChange{Title: &cook, Was: &none}); !errors.Is(err, club.ErrTitleChanged) {
		t.Fatalf("Describe over a title the change did not see: %v", err)
	}
	if err = members.Describe(ctx, c.ID(), cat, club.SeatChange{Title: &cook, Was: &none}); !errors.Is(err, club.ErrNotManager) {
		t.Fatalf("Describe a member from a title: %v", err)
	}
	// The hiding does not look at the title.
	if err = members.Describe(ctx, c.ID(), bob, club.SeatChange{Hidden: &hidden, Was: &none}); err != nil {
		t.Fatalf("Describe the hiding from another title: %v", err)
	}
	if err = members.Describe(ctx, c.ID(), bob, club.SeatChange{Title: &cook, Was: &title}); err != nil {
		t.Fatal(err)
	}

	// Of several changes that started from one title, one is written.
	const writers = 8

	results := make(chan error, writers)
	for i := range writers {
		go func() {
			next := club.Title("повар " + strconv.Itoa(i))
			results <- members.Describe(ctx, c.ID(), bob, club.SeatChange{Title: &next, Was: &cook})
		}()
	}

	written := 0
	for range writers {
		switch err := <-results; {
		case err == nil:
			written++
		case !errors.Is(err, club.ErrTitleChanged):
			t.Fatalf("a concurrent Describe: %v", err)
		}
	}
	if written != 1 {
		t.Fatalf("%d of %d changes from one title were written, want 1", written, writers)
	}
	if err = members.Describe(ctx, c.ID(), bob, club.SeatChange{Title: &title}); err != nil {
		t.Fatal(err)
	}
	// Bob is hidden, Ann is not, and Cat runs nothing.
	if hid, err := members.Hidden(ctx, []uuid.UUID{uuid.New(), c.ID()}); err != nil || len(hid) != 1 || len(hid[c.ID()]) != 1 || hid[c.ID()][0] != bob {
		t.Fatalf("Hidden = %v, %v; want Bob", hid, err)
	}
	if hid, err := members.Hidden(ctx, nil); err != nil || len(hid) != 0 {
		t.Fatalf("Hidden of no clubs = %v, %v", hid, err)
	}
	if err = members.Describe(ctx, c.ID(), ann, described("главная", false)); err != nil {
		t.Fatal(err)
	}
	if seat, err := members.Seat(ctx, c.ID(), bob); err != nil || seat != (club.Seat{Role: club.RoleAdmin, Title: "кормит котов", Hidden: true}) {
		t.Fatalf("Seat = %+v, %v", seat, err)
	}
	if seat, err := members.Seat(ctx, c.ID(), uuid.New()); err != nil || seat != (club.Seat{}) {
		t.Fatalf("Seat of a stranger = %+v, %v", seat, err)
	}
	if n, err := members.CountAdmins(ctx, c.ID(), true); err != nil || n != 1 {
		t.Fatalf("CountAdmins shown = %d, %v", n, err)
	}
	if shown, err := members.List(ctx, c.ID(), uuid.Nil, 10, service.ShownManagers); err != nil || len(shown) != 1 || shown[0].UserID != ann || shown[0].Title != "главная" {
		t.Fatalf("List shown = %+v, %v", shown, err)
	}
	if all, err := members.List(ctx, c.ID(), uuid.Nil, 10, service.Managers); err != nil || len(all) != 2 || !all[0].Hidden {
		t.Fatalf("List managers = %+v, %v", all, err)
	}
	// Dismissed and appointed again, an admin starts with neither.
	if err = members.SetRole(ctx, c.ID(), bob, club.RoleMember); err != nil {
		t.Fatal(err)
	}
	if err = members.SetRole(ctx, c.ID(), bob, club.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if seat, err := members.Seat(ctx, c.ID(), bob); err != nil || seat != (club.Seat{Role: club.RoleAdmin}) {
		t.Fatalf("Seat after a dismissal = %+v, %v", seat, err)
	}
	if mine, err := members.ListClubs(ctx, bob, uuid.Nil, 10, service.ClubFilter{}); err != nil || len(mine) != 1 || mine[0].Club.Website() != "https://cats.example" {
		t.Fatalf("ListClubs = %+v, %v; with the settings", mine, err)
	}

	if err = members.Transfer(ctx, c.ID(), ann, cat); !errors.Is(err, club.ErrNotAdmin) {
		t.Fatalf("to a member: %v", err)
	}
	if owner, _ := members.Owner(ctx, c.ID()); owner != ann {
		t.Fatalf("a failed transfer changed the owner to %v", owner)
	}
	if err = members.Transfer(ctx, c.ID(), bob, ann); !errors.Is(err, club.ErrCannotTransfer) {
		t.Fatalf("by an admin: %v", err)
	}

	// Two transfers at once: one goes through, the other finds no owner.
	errs := make(chan error, 2)
	for range 2 {
		go func() { errs <- members.Transfer(ctx, c.ID(), ann, bob) }()
	}
	first, second := <-errs, <-errs
	if (first == nil) == (second == nil) || !errors.Is(errors.Join(first, second), club.ErrCannotTransfer) {
		t.Fatalf("concurrent transfers = %v, %v; one of them", first, second)
	}

	if owner, _ := members.Owner(ctx, c.ID()); owner != bob {
		t.Fatalf("owner = %v, want bob", owner)
	}
	if role, _ := members.Role(ctx, c.ID(), ann); role != club.RoleAdmin {
		t.Fatalf("the old owner = %q, want an admin", role)
	}
}

// described is a change of both things of a seat.
func described(title club.Title, hidden bool) club.SeatChange {
	return club.SeatChange{Title: &title, Hidden: &hidden}
}
