package service_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/club/internal/club"
	"github.com/vexner67/freenet/club/internal/inmem"
	"github.com/vexner67/freenet/club/internal/service"
)

// notifier remembers who was told of an appointment, by club and user.
type notifier struct {
	appointed map[[2]uuid.UUID]bool
}

func (n *notifier) Appointed(_ context.Context, clubID, userID uuid.UUID) error {
	n.appointed[[2]uuid.UUID{clubID, userID}] = true
	return nil
}

func (n *notifier) Dismissed(_ context.Context, clubID, userID uuid.UUID) error {
	delete(n.appointed, [2]uuid.UUID{clubID, userID})
	return nil
}

func newNotified() (*service.ClubService, *notifier) {
	store := inmem.NewStore()
	n := &notifier{appointed: make(map[[2]uuid.UUID]bool)}

	return service.NewClubService(service.ClubDeps{Clubs: store.Clubs(), Members: store.Members(), Tx: inmem.TxManager{}, Notifier: n}), n
}

func newService() *service.ClubService {
	svc, _ := newNotified()
	return svc
}

func create(t *testing.T, svc *service.ClubService, ownerID uuid.UUID) *club.Club {
	t.Helper()

	v, err := svc.Create(context.Background(), ownerID, "Любители котов", "про котов")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	return v.Club
}

func TestClubService_Create(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := newService()
	ann := uuid.New()

	v, err := svc.Create(ctx, ann, "Любители котов", "про котов")
	if err != nil {
		t.Fatal(err)
	}
	if v.Role != club.RoleOwner || v.OwnerID != ann || v.Club.Members() != 1 || v.Club.Wall() != club.WallOpen {
		t.Fatalf("created = %+v; ann owns it and is its only member", v)
	}

	anon, err := svc.Find(ctx, uuid.Nil, service.Ref{ID: v.Club.ID()})
	if err != nil || anon.Role != club.RoleNone || anon.OwnerID != ann {
		t.Fatalf("anonymous view = %+v, %v", anon, err)
	}

	if _, err = svc.Find(ctx, ann, service.Ref{ID: uuid.New()}); !errors.Is(err, club.ErrNotFound) {
		t.Fatalf("unknown club: %v", err)
	}
	if _, err = svc.Find(ctx, ann, service.Ref{Address: "nobody"}); !errors.Is(err, club.ErrNotFound) {
		t.Fatalf("unknown address: %v", err)
	}
}

func TestClubService_Update(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := newService()
	ann, bob := uuid.New(), uuid.New()
	c := create(t, svc, ann)
	other := create(t, svc, bob)

	changes := club.Changes{Name: "Кошки", Description: "мяу", Address: "Cats", Wall: club.WallClosed}

	if _, err := svc.Update(ctx, bob, c.ID(), changes); !errors.Is(err, club.ErrCannotEdit) {
		t.Fatalf("a stranger edits: %v", err)
	}

	v, err := svc.Update(ctx, ann, c.ID(), changes)
	if err != nil || v.Club.Name() != "Кошки" || v.Club.Address() != "Cats" || v.Club.Wall() != club.WallClosed {
		t.Fatalf("Update = %+v, %v", v, err)
	}

	// The address is found whatever the case, and taken whatever the case.
	found, err := svc.Find(ctx, bob, service.Ref{Address: "cats"})
	if err != nil || found.Club.ID() != c.ID() {
		t.Fatalf("Find by address = %+v, %v", found, err)
	}
	if _, err = svc.Update(ctx, bob, other.ID(), club.Changes{Name: "Другие", Address: "CATS", Wall: club.WallOpen}); !errors.Is(err, club.ErrAddressTaken) {
		t.Fatalf("taken address: %v", err)
	}

	if _, err = svc.Update(ctx, ann, uuid.New(), changes); !errors.Is(err, club.ErrNotFound) {
		t.Fatalf("unknown club: %v", err)
	}
}

func TestClubService_Membership(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := newService()
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()
	c := create(t, svc, ann)

	v, err := svc.Join(ctx, bob, c.ID(), false)
	if err != nil || v.Role != club.RoleMember || v.Club.Members() != 2 {
		t.Fatalf("Join = %+v, %v", v, err)
	}
	if v, err = svc.Join(ctx, bob, c.ID(), false); err != nil || v.Club.Members() != 2 {
		t.Fatalf("join twice = %+v, %v", v, err)
	}
	if _, err = svc.Join(ctx, bob, uuid.New(), false); !errors.Is(err, club.ErrNotFound) {
		t.Fatalf("join an unknown club: %v", err)
	}

	if _, err = svc.Join(ctx, cat, c.ID(), false); err != nil {
		t.Fatal(err)
	}

	page, err := svc.Members(ctx, uuid.Nil, c.ID(), uuid.Nil, 2, service.PickAll)
	if err != nil || len(page.Members) != 2 || page.Members[0].UserID != cat || page.NextCursor == uuid.Nil {
		t.Fatalf("first page = %+v, %v; the newest first", page, err)
	}
	if page, err = svc.Members(ctx, uuid.Nil, c.ID(), page.NextCursor, 2, service.PickAll); err != nil || len(page.Members) != 1 || page.Members[0].UserID != ann || page.NextCursor != uuid.Nil {
		t.Fatalf("last page = %+v, %v", page, err)
	}

	if _, err = svc.Leave(ctx, ann, c.ID()); !errors.Is(err, club.ErrOwnerCannotLeave) {
		t.Fatalf("the owner leaves: %v", err)
	}
	if v, err = svc.Leave(ctx, cat, c.ID()); err != nil || v.Role != club.RoleNone || v.Club.Members() != 2 {
		t.Fatalf("Leave = %+v, %v", v, err)
	}
	if _, err = svc.Leave(ctx, cat, c.ID()); err != nil {
		t.Fatalf("leave twice: %v", err)
	}

	mine, err := svc.Clubs(ctx, bob, uuid.Nil, 10, service.ClubFilter{})
	if err != nil || len(mine.Clubs) != 1 || mine.Clubs[0].Club.ID() != c.ID() || mine.Clubs[0].Role != club.RoleMember {
		t.Fatalf("bob's clubs = %+v, %v", mine, err)
	}
	if ids, err := svc.ClubIDs(ctx, bob); err != nil || len(ids) != 1 || ids[0] != c.ID() {
		t.Fatalf("bob's club IDs = %v, %v", ids, err)
	}

	// What one is in the clubs of a page: those one is not in are left out.
	other := create(t, svc, cat)
	asked := []uuid.UUID{c.ID(), other.ID(), uuid.New()}
	if roles, err := svc.Roles(ctx, bob, asked); err != nil || len(roles) != 1 || roles[c.ID()] != club.RoleMember {
		t.Fatalf("bob's roles = %v, %v", roles, err)
	}
	if roles, err := svc.Roles(ctx, ann, asked); err != nil || len(roles) != 1 || roles[c.ID()] != club.RoleOwner {
		t.Fatalf("ann's roles = %v, %v", roles, err)
	}
	if roles, err := svc.Roles(ctx, bob, nil); err != nil || len(roles) != 0 {
		t.Fatalf("roles in no clubs = %v, %v", roles, err)
	}
	if _, err := svc.Roles(ctx, bob, make([]uuid.UUID, service.MaxClubs+1)); !errors.Is(err, service.ErrTooManyIDs) {
		t.Fatalf("roles in too many clubs: %v", err)
	}
}

func TestClubService_Appoint(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc, told := newNotified()
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()
	c := create(t, svc, ann)

	if _, err := svc.Join(ctx, bob, c.ID(), false); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name              string
		requester, target uuid.UUID
		role              club.Role
		err               error
	}{
		{"a member appoints", bob, bob, club.RoleAdmin, club.ErrCannotAppoint},
		{"the owner appoints a stranger", ann, cat, club.RoleAdmin, club.ErrNotMember},
		{"the owner demotes himself", ann, ann, club.RoleMember, club.ErrCannotAppoint},
	}
	for _, tt := range tests {
		if err := svc.Appoint(ctx, tt.requester, c.ID(), tt.target, tt.role); !errors.Is(err, tt.err) {
			t.Errorf("%s: %v, want %v", tt.name, err, tt.err)
		}
	}

	// Making a member a member tells nobody.
	if err := svc.Appoint(ctx, ann, c.ID(), bob, club.RoleMember); err != nil || len(told.appointed) != 0 {
		t.Fatalf("a member stays a member: %v, told %v", err, told.appointed)
	}

	// The new admin hears about it, once however often he is appointed.
	for range 2 {
		if err := svc.Appoint(ctx, ann, c.ID(), bob, club.RoleAdmin); err != nil {
			t.Fatal(err)
		}
	}
	if len(told.appointed) != 1 || !told.appointed[[2]uuid.UUID{c.ID(), bob}] {
		t.Fatalf("told = %v, want bob of the club", told.appointed)
	}

	admins, err := svc.Members(ctx, uuid.Nil, c.ID(), uuid.Nil, 10, service.PickAdmins)
	if err != nil || len(admins.Members) != 2 || admins.Total != 2 {
		t.Fatalf("admins = %+v, %v; the owner and bob", admins, err)
	}
	if all, err := svc.Members(ctx, uuid.Nil, c.ID(), uuid.Nil, 1, service.PickAll); err != nil || len(all.Members) != 1 || all.Total != 2 {
		t.Fatalf("members = %+v, %v; one of two shown", all, err)
	}

	// An admin edits, writes for the club and keeps its wall clean, but
	// does not appoint.
	if _, err = svc.Update(ctx, bob, c.ID(), club.Changes{Name: "Коты", Wall: club.WallOpen}); err != nil {
		t.Fatalf("the admin edits: %v", err)
	}
	if ok, err := svc.CanModerateWall(ctx, c.ID(), bob); err != nil || !ok {
		t.Fatalf("the admin moderates: %v, %v", ok, err)
	}
	if ids, err := svc.WallModerators(ctx, c.ID()); err != nil || !slices.Equal(ids, []uuid.UUID{bob, ann}) {
		t.Fatalf("WallModerators = %v, %v; bob, then the owner", ids, err)
	}
	if err = svc.Appoint(ctx, bob, c.ID(), bob, club.RoleMember); !errors.Is(err, club.ErrCannotAppoint) {
		t.Fatalf("the admin appoints: %v", err)
	}

	if err = svc.Appoint(ctx, ann, c.ID(), bob, club.RoleMember); err != nil {
		t.Fatal(err)
	}
	if ok, _ := svc.CanModerateWall(ctx, c.ID(), bob); ok {
		t.Fatal("bob is a member again")
	}
	if len(told.appointed) != 0 {
		t.Fatalf("the dismissal left %v", told.appointed)
	}
}

func TestClubService_Transfer(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := newService()
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()
	c := create(t, svc, ann)

	for _, id := range []uuid.UUID{bob, cat} {
		if _, err := svc.Join(ctx, id, c.ID(), false); err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.Appoint(ctx, ann, c.ID(), bob, club.RoleAdmin); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name              string
		requester, target uuid.UUID
		err               error
	}{
		{"an admin hands it over", bob, bob, club.ErrCannotTransfer},
		{"a member hands it over", cat, bob, club.ErrCannotTransfer},
		{"to a member", ann, cat, club.ErrNotAdmin},
		{"to a stranger", ann, uuid.New(), club.ErrNotAdmin},
		{"to the owner", ann, ann, club.ErrNotAdmin},
	}
	for _, tt := range tests {
		if err := svc.Transfer(ctx, tt.requester, c.ID(), tt.target); !errors.Is(err, tt.err) {
			t.Errorf("%s: %v, want %v", tt.name, err, tt.err)
		}
	}
	if err := svc.Transfer(ctx, ann, uuid.New(), bob); !errors.Is(err, club.ErrNotFound) {
		t.Fatalf("an unknown club: %v", err)
	}

	if err := svc.Transfer(ctx, ann, c.ID(), bob); err != nil {
		t.Fatal(err)
	}

	// Bob owns the club now; Ann stays an admin and may leave.
	v, err := svc.Find(ctx, ann, service.Ref{ID: c.ID()})
	if err != nil || v.OwnerID != bob || v.Role != club.RoleAdmin {
		t.Fatalf("after the transfer = %+v, %v", v, err)
	}
	if err = svc.Appoint(ctx, bob, c.ID(), ann, club.RoleMember); err != nil {
		t.Fatalf("the new owner appoints: %v", err)
	}
	if _, err = svc.Leave(ctx, ann, c.ID()); err != nil {
		t.Fatalf("the old owner leaves: %v", err)
	}
	if err = svc.Transfer(ctx, ann, c.ID(), bob); !errors.Is(err, club.ErrCannotTransfer) {
		t.Fatalf("the old owner hands it over again: %v", err)
	}
}

func TestClubService_Walls(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := newService()
	ann, bob := uuid.New(), uuid.New()
	c := create(t, svc, ann)

	can := func(user uuid.UUID, fromClub bool) bool {
		t.Helper()
		access, err := svc.CanPostOnWall(ctx, user, c.ID(), fromClub)
		if err != nil {
			t.Fatal(err)
		}
		return access.Post
	}

	if !can(bob, false) || can(bob, true) || !can(ann, true) {
		t.Fatal("open wall: anyone posts, only the admins for the club")
	}

	if _, err := svc.Update(ctx, ann, c.ID(), club.Changes{Name: c.Name(), Wall: club.WallClosed}); err != nil {
		t.Fatal(err)
	}
	if can(bob, false) || !can(ann, false) {
		t.Fatal("closed wall: only the admins")
	}
	if access, _ := svc.CanPostOnWall(ctx, bob, c.ID(), false); access.Suggest {
		t.Fatal("closed wall: nobody suggests")
	}

	if _, err := svc.Update(ctx, ann, c.ID(), club.Changes{Name: c.Name(), Wall: club.WallSuggest}); err != nil {
		t.Fatal(err)
	}
	if access, err := svc.CanPostOnWall(ctx, bob, c.ID(), false); err != nil || access.Post || !access.Suggest {
		t.Fatalf("suggest wall: bob suggests = %+v, %v", access, err)
	}
	if access, err := svc.CanPostOnWall(ctx, ann, c.ID(), true); err != nil || !access.Post || access.Suggest {
		t.Fatalf("suggest wall: ann posts for the club = %+v, %v", access, err)
	}

	if _, err := svc.CanPostOnWall(ctx, bob, uuid.New(), false); !errors.Is(err, club.ErrNotFound) {
		t.Fatalf("unknown club: %v", err)
	}

	// A user's wall is no club's: nobody moderates it here.
	if ok, err := svc.CanModerateWall(ctx, ann, ann); err != nil || ok {
		t.Fatalf("a user's wall = %v, %v", ok, err)
	}
	if ids, err := svc.WallModerators(ctx, ann); err != nil || len(ids) != 0 {
		t.Fatalf("a user's wall has moderators %v, %v", ids, err)
	}
}

func TestClubService_GetMany(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := newService()
	a, b := create(t, svc, uuid.New()), create(t, svc, uuid.New())

	found, err := svc.GetMany(ctx, []uuid.UUID{a.ID(), uuid.New(), b.ID()})
	if err != nil || len(found) != 2 {
		t.Fatalf("GetMany = %v, %v", found, err)
	}

	if _, err = svc.GetMany(ctx, make([]uuid.UUID, service.MaxClubs+1)); !errors.Is(err, service.ErrTooManyIDs) {
		t.Fatalf("too many: %v", err)
	}
}

func TestClubService_PurgeUser(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := newService()
	ann, bob := uuid.New(), uuid.New()
	c := create(t, svc, ann)

	if _, err := svc.Join(ctx, bob, c.ID(), false); err != nil {
		t.Fatal(err)
	}

	if err := svc.PurgeUser(ctx, ann); err != nil {
		t.Fatal(err)
	}
	if err := svc.PurgeUser(ctx, ann); err != nil {
		t.Fatalf("purge twice: %v", err)
	}

	// The only one left inherits the club.
	v, err := svc.Find(ctx, bob, service.Ref{ID: c.ID()})
	if err != nil || v.OwnerID != bob || v.Club.Members() != 1 || v.Role != club.RoleOwner {
		t.Fatalf("after the owner is erased = %+v, %v; the club goes to bob", v, err)
	}
}

func TestClubService_PurgeUser_Heir(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := newService()
	ann, bob, cat, dan := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	c, empty := create(t, svc, ann), create(t, svc, ann)

	// Bob is in the club longest, cat and dan run it, cat longer.
	for _, user := range []uuid.UUID{bob, cat, dan} {
		if _, err := svc.Join(ctx, user, c.ID(), false); err != nil {
			t.Fatal(err)
		}
	}
	for _, user := range []uuid.UUID{dan, cat} {
		if err := svc.Appoint(ctx, ann, c.ID(), user, club.RoleAdmin); err != nil {
			t.Fatal(err)
		}
	}

	if err := svc.PurgeUser(ctx, ann); err != nil {
		t.Fatal(err)
	}

	v, err := svc.Find(ctx, cat, service.Ref{ID: c.ID()})
	if err != nil || v.OwnerID != cat || v.Role != club.RoleOwner || v.Club.Members() != 3 {
		t.Fatalf("the club of an erased owner = %+v, %v; want cat's, the oldest admin", v, err)
	}
	// The new owner runs it as the old one did.
	if err = svc.Appoint(ctx, cat, c.ID(), bob, club.RoleAdmin); err != nil {
		t.Fatalf("the heir appoints: %v", err)
	}
	if v, err = svc.Find(ctx, dan, service.Ref{ID: c.ID()}); err != nil || v.Role != club.RoleAdmin {
		t.Fatalf("dan = %+v, %v; still an admin", v.Role, err)
	}

	if v, err = svc.Find(ctx, uuid.Nil, service.Ref{ID: empty.ID()}); err != nil || v.OwnerID != uuid.Nil || v.Club.Members() != 0 {
		t.Fatalf("a club nobody else is in = %+v, %v; want no owner", v, err)
	}
}

func TestClubService_AvatarSearchAndMenu(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := newService()
	ann, bob := uuid.New(), uuid.New()
	cats := create(t, svc, ann)
	if _, err := svc.Update(ctx, ann, cats.ID(), club.Changes{Name: "Любители котов", Address: "cats", Wall: club.WallOpen}); err != nil {
		t.Fatal(err)
	}
	dogs, err := svc.Create(ctx, bob, "Собаки", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Join(ctx, bob, cats.ID(), false); err != nil {
		t.Fatal(err)
	}

	photo := uuid.New()
	v, err := svc.SetAvatar(ctx, ann, cats.ID(), photo)
	if err != nil || v.Club.AvatarID() != photo {
		t.Fatalf("SetAvatar = %+v, %v", v, err)
	}
	if _, err = svc.SetAvatar(ctx, bob, cats.ID(), uuid.New()); !errors.Is(err, club.ErrCannotEdit) {
		t.Fatalf("a member sets the photo: %v", err)
	}
	if v, err = svc.SetAvatar(ctx, ann, cats.ID(), uuid.Nil); err != nil || v.Club.AvatarID() != uuid.Nil {
		t.Fatalf("take the photo away = %+v, %v", v, err)
	}

	q, _ := club.NewSearchQuery("кот")
	found, err := svc.Search(ctx, q, uuid.Nil, 10)
	if err != nil || len(found.Clubs) != 1 || found.Clubs[0].ID() != cats.ID() {
		t.Fatalf("Search = %+v, %v", found, err)
	}

	// Bob is in two clubs and runs one: "Управление".
	all, err := svc.Clubs(ctx, bob, uuid.Nil, 10, service.ClubFilter{})
	if err != nil || len(all.Clubs) != 2 || all.Total != 2 {
		t.Fatalf("bob's clubs = %+v, %v", all, err)
	}
	managed, err := svc.Clubs(ctx, bob, uuid.Nil, 10, service.ClubFilter{Managed: true})
	if err != nil || len(managed.Clubs) != 1 || managed.Total != 1 || managed.Clubs[0].Club.ID() != dogs.Club.ID() {
		t.Fatalf("bob's managed clubs = %+v, %v", managed, err)
	}

	if err = svc.Pin(ctx, bob, cats.ID(), true); !errors.Is(err, club.ErrCannotPin) {
		t.Fatalf("pin a club one does not run: %v", err)
	}
	if err = svc.Pin(ctx, bob, dogs.Club.ID(), true); err != nil {
		t.Fatal(err)
	}
	if err = svc.Pin(ctx, bob, dogs.Club.ID(), true); err != nil {
		t.Fatalf("pin twice: %v", err)
	}
	pinned, err := svc.Pinned(ctx, bob)
	if err != nil || len(pinned) != 1 || pinned[0].ID() != dogs.Club.ID() {
		t.Fatalf("Pinned = %v, %v", pinned, err)
	}
	if managed, _ = svc.Clubs(ctx, bob, uuid.Nil, 10, service.ClubFilter{Managed: true}); !managed.Clubs[0].Pinned {
		t.Fatal("the list says the club is pinned")
	}
	if err = svc.Pin(ctx, bob, dogs.Club.ID(), false); err != nil {
		t.Fatal(err)
	}
	if pinned, _ = svc.Pinned(ctx, bob); len(pinned) != 0 {
		t.Fatalf("unpinned = %v", pinned)
	}

	// At most club.MaxPinned.
	for range club.MaxPinned {
		v, err := svc.Create(ctx, ann, "ещё", "")
		if err != nil {
			t.Fatal(err)
		}
		if err = svc.Pin(ctx, ann, v.Club.ID(), true); err != nil {
			t.Fatal(err)
		}
	}
	if err = svc.Pin(ctx, ann, cats.ID(), true); !errors.Is(err, club.ErrTooManyPinned) {
		t.Fatalf("the eleventh: %v", err)
	}
}

func TestClubService_Describe(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := newService()
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()
	c := create(t, svc, ann)

	for _, id := range []uuid.UUID{bob, cat} {
		if _, err := svc.Join(ctx, id, c.ID(), false); err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.Appoint(ctx, ann, c.ID(), bob, club.RoleAdmin); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name              string
		requester, target uuid.UUID
		err               error
	}{
		{"by a member", cat, bob, club.ErrCannotEdit},
		{"by a stranger", uuid.New(), ann, club.ErrCannotEdit},
		{"a member", ann, cat, club.ErrNotManager},
		{"a stranger", bob, uuid.New(), club.ErrNotManager},
	}
	for _, tt := range tests {
		if err := svc.Describe(ctx, tt.requester, c.ID(), tt.target, "кот", true); !errors.Is(err, tt.err) {
			t.Errorf("%s: %v, want %v", tt.name, err, tt.err)
		}
	}
	if err := svc.Describe(ctx, ann, uuid.New(), bob, "", false); !errors.Is(err, club.ErrNotFound) {
		t.Fatalf("an unknown club: %v", err)
	}

	// The admins describe each other: Bob the owner, Ann Bob, whom she hides.
	if err := svc.Describe(ctx, bob, c.ID(), ann, "главная", false); err != nil {
		t.Fatal(err)
	}
	if err := svc.Describe(ctx, ann, c.ID(), bob, "кормит котов", true); err != nil {
		t.Fatal(err)
	}

	// Those who run the club see Bob as he is; the others, a plain member.
	for _, viewer := range []uuid.UUID{ann, bob} {
		admins, err := svc.Members(ctx, viewer, c.ID(), uuid.Nil, 10, service.PickAdmins)
		if err != nil || admins.Total != 2 || len(admins.Members) != 2 ||
			admins.Members[0].Seat != (club.Seat{Role: club.RoleAdmin, Title: "кормит котов", Hidden: true}) ||
			admins.Members[1].Seat != (club.Seat{Role: club.RoleOwner, Title: "главная"}) {
			t.Fatalf("admins to one of them = %+v, %v", admins, err)
		}
	}
	for _, viewer := range []uuid.UUID{cat, uuid.Nil} {
		admins, err := svc.Members(ctx, viewer, c.ID(), uuid.Nil, 10, service.PickAdmins)
		if err != nil || admins.Total != 1 || len(admins.Members) != 1 || admins.Members[0].UserID != ann {
			t.Fatalf("admins to others = %+v, %v; want only the owner", admins, err)
		}

		all, err := svc.Members(ctx, viewer, c.ID(), uuid.Nil, 10, service.PickAll)
		if err != nil || len(all.Members) != 3 || all.Members[1].UserID != bob || all.Members[1].Seat != (club.Seat{Role: club.RoleMember}) {
			t.Fatalf("members to others = %+v, %v; want Bob a plain member", all, err)
		}
	}

	if v, err := svc.Find(ctx, cat, service.Ref{ID: c.ID()}); err != nil || v.OwnerID != ann || v.OwnerTitle != "главная" {
		t.Fatalf("the owner = %+v, %v", v, err)
	}

	// A hidden owner is nobody to those who do not run the club.
	if err := svc.Describe(ctx, ann, c.ID(), ann, "главная", true); err != nil {
		t.Fatal(err)
	}
	if v, err := svc.Find(ctx, cat, service.Ref{ID: c.ID()}); err != nil || v.OwnerID != uuid.Nil || v.OwnerTitle != "" {
		t.Fatalf("a hidden owner to a member = %+v, %v", v, err)
	}
	if v, err := svc.Find(ctx, bob, service.Ref{ID: c.ID()}); err != nil || v.OwnerID != ann || v.OwnerTitle != "главная" {
		t.Fatalf("a hidden owner to an admin = %+v, %v", v, err)
	}
	if mods, err := svc.WallModerators(ctx, c.ID()); err != nil || len(mods) != 2 {
		t.Fatalf("WallModerators = %v, %v; the hidden ones too", mods, err)
	}

	// Dismissed, Bob keeps neither the title nor the hiding.
	if err := svc.Appoint(ctx, ann, c.ID(), bob, club.RoleMember); err != nil {
		t.Fatal(err)
	}
	if err := svc.Appoint(ctx, ann, c.ID(), bob, club.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	admins, err := svc.Members(ctx, ann, c.ID(), uuid.Nil, 10, service.PickAdmins)
	if err != nil || admins.Members[0].UserID != bob || admins.Members[0].Seat != (club.Seat{Role: club.RoleAdmin}) {
		t.Fatalf("appointed again = %+v, %v; want no title", admins, err)
	}
}

func TestClubService_Events(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := newService()
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()
	group := create(t, svc, ann)

	soon, _ := club.NewSchedule(time.Now().Add(time.Hour), time.Time{})
	v, err := svc.CreateEvent(ctx, ann, "Новый год", "ёлка", soon)
	if err != nil || !v.Club.IsEvent() || v.Role != club.RoleOwner || v.Maybe || v.Club.Members() != 1 || v.Club.Maybes() != 0 {
		t.Fatalf("CreateEvent = %+v, %v", v, err)
	}
	event := v.Club

	// Bob might come, Cat is sure to; asking again changes nothing.
	if v, err = svc.Join(ctx, bob, event.ID(), true); err != nil || !v.Maybe || v.Club.Members() != 2 || v.Club.Maybes() != 1 {
		t.Fatalf("a maybe = %+v, %v", v, err)
	}
	if v, err = svc.Join(ctx, bob, event.ID(), true); err != nil || !v.Maybe || v.Club.Members() != 2 || v.Club.Maybes() != 1 {
		t.Fatalf("a maybe asking again = %+v, %v; want no change", v, err)
	}
	// A member changes the answer by joining again; the owner is sure.
	if v, err = svc.Join(ctx, bob, event.ID(), false); err != nil || v.Maybe || v.Club.Members() != 2 || v.Club.Maybes() != 0 {
		t.Fatalf("a maybe now sure = %+v, %v", v, err)
	}
	if v, err = svc.Join(ctx, bob, event.ID(), true); err != nil || !v.Maybe || v.Club.Members() != 2 || v.Club.Maybes() != 1 {
		t.Fatalf("a sure one now a maybe = %+v, %v", v, err)
	}
	if v, err = svc.Join(ctx, ann, event.ID(), true); err != nil || v.Maybe || v.Role != club.RoleOwner || v.Club.Maybes() != 1 {
		t.Fatalf("the owner asking to be a maybe = %+v, %v", v, err)
	}
	if v, err = svc.Join(ctx, cat, event.ID(), false); err != nil || v.Maybe || v.Club.Members() != 3 {
		t.Fatalf("a sure one = %+v, %v", v, err)
	}

	sure, err := svc.Members(ctx, uuid.Nil, event.ID(), uuid.Nil, 10, service.PickSure)
	if err != nil || sure.Total != 2 || len(sure.Members) != 2 || sure.Members[0].UserID != cat || sure.Members[1].UserID != ann {
		t.Fatalf("sure = %+v, %v", sure, err)
	}
	maybe, err := svc.Members(ctx, uuid.Nil, event.ID(), uuid.Nil, 10, service.PickMaybe)
	if err != nil || maybe.Total != 1 || len(maybe.Members) != 1 || maybe.Members[0].UserID != bob || !maybe.Members[0].Maybe {
		t.Fatalf("maybe = %+v, %v", maybe, err)
	}

	// Leaving takes the maybe out of the count.
	if v, err = svc.Leave(ctx, bob, event.ID()); err != nil || v.Maybe || v.Club.Members() != 2 || v.Club.Maybes() != 0 {
		t.Fatalf("a maybe leaves = %+v, %v", v, err)
	}

	// A group takes no maybe, nor does an event that has started.
	if v, err = svc.Join(ctx, bob, group.ID(), true); err != nil || v.Maybe || v.Club.Maybes() != 0 {
		t.Fatalf("a maybe in a group = %+v, %v", v, err)
	}
	past, _ := club.NewSchedule(time.Now().Add(-time.Hour), time.Time{})
	started, err := svc.CreateEvent(ctx, ann, "Вчера", "", past)
	if err != nil {
		t.Fatal(err)
	}
	if v, err = svc.Join(ctx, bob, started.Club.ID(), true); err != nil || v.Maybe {
		t.Fatalf("a maybe in a started event = %+v, %v", v, err)
	}

	// The site counts its groups; events are not groups.
	if n, err := svc.Count(ctx); err != nil || n != 1 {
		t.Fatalf("Count = %d, %v", n, err)
	}

	// The lists tell groups from events.
	events, err := svc.Clubs(ctx, ann, uuid.Nil, 10, service.ClubFilter{Kind: club.KindEvent})
	if err != nil || events.Total != 2 || len(events.Clubs) != 2 || events.Clubs[1].Club.ID() != event.ID() {
		t.Fatalf("events = %+v, %v", events, err)
	}
	groups, err := svc.Clubs(ctx, ann, uuid.Nil, 10, service.ClubFilter{Kind: club.KindGroup})
	if err != nil || groups.Total != 1 || groups.Clubs[0].Club.ID() != group.ID() {
		t.Fatalf("groups = %+v, %v", groups, err)
	}
	if all, err := svc.Clubs(ctx, ann, uuid.Nil, 10, service.ClubFilter{}); err != nil || all.Total != 3 {
		t.Fatalf("all = %+v, %v", all, err)
	}
	if managed, err := svc.Clubs(ctx, bob, uuid.Nil, 10, service.ClubFilter{Managed: true, Kind: club.KindEvent}); err != nil || managed.Total != 0 {
		t.Fatalf("bob runs no events: %+v, %v", managed, err)
	}

	// And the events ahead from those over; a group is neither.
	ahead, err := svc.Clubs(ctx, ann, uuid.Nil, 10, service.ClubFilter{Period: club.PeriodUpcoming})
	if err != nil || ahead.Total != 1 || len(ahead.Clubs) != 1 || ahead.Clubs[0].Club.ID() != event.ID() {
		t.Fatalf("upcoming = %+v, %v", ahead, err)
	}
	over, err := svc.Clubs(ctx, ann, uuid.Nil, 10, service.ClubFilter{Period: club.PeriodPast})
	if err != nil || over.Total != 1 || len(over.Clubs) != 1 || over.Clubs[0].Club.ID() != started.Club.ID() {
		t.Fatalf("past = %+v, %v", over, err)
	}

	// Its admins move the event; it never loses its start.
	later, _ := club.NewSchedule(soon.Start.Add(time.Hour), soon.Start.Add(2*time.Hour))
	changes := club.Changes{Name: "Новый год", Wall: club.WallOpen, AdminsDisplay: club.AdminsDisplayCreator, Schedule: later}
	if v, err = svc.Update(ctx, ann, event.ID(), changes); err != nil || v.Club.Schedule() != later {
		t.Fatalf("Update = %+v, %v", v, err)
	}
	changes.Schedule = club.Schedule{}
	if _, err = svc.Update(ctx, ann, event.ID(), changes); !errors.Is(err, club.ErrNoStart) {
		t.Fatalf("Update to no start: %v", err)
	}
}
