package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/club/internal/club"
	"github.com/vexner67/freenet/club/internal/inmem"
	"github.com/vexner67/freenet/club/internal/service"
)

func newService() *service.ClubService {
	store := inmem.NewStore()
	return service.NewClubService(store.Clubs(), store.Members())
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

	v, err := svc.Join(ctx, bob, c.ID())
	if err != nil || v.Role != club.RoleMember || v.Club.Members() != 2 {
		t.Fatalf("Join = %+v, %v", v, err)
	}
	if v, err = svc.Join(ctx, bob, c.ID()); err != nil || v.Club.Members() != 2 {
		t.Fatalf("join twice = %+v, %v", v, err)
	}
	if _, err = svc.Join(ctx, bob, uuid.New()); !errors.Is(err, club.ErrNotFound) {
		t.Fatalf("join an unknown club: %v", err)
	}

	if _, err = svc.Join(ctx, cat, c.ID()); err != nil {
		t.Fatal(err)
	}

	page, err := svc.Members(ctx, c.ID(), uuid.Nil, 2, false)
	if err != nil || len(page.Members) != 2 || page.Members[0].UserID != cat || page.NextCursor == uuid.Nil {
		t.Fatalf("first page = %+v, %v; the newest first", page, err)
	}
	if page, err = svc.Members(ctx, c.ID(), page.NextCursor, 2, false); err != nil || len(page.Members) != 1 || page.Members[0].UserID != ann || page.NextCursor != uuid.Nil {
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

	mine, err := svc.Clubs(ctx, bob, uuid.Nil, 10, false)
	if err != nil || len(mine.Clubs) != 1 || mine.Clubs[0].Club.ID() != c.ID() || mine.Clubs[0].Role != club.RoleMember {
		t.Fatalf("bob's clubs = %+v, %v", mine, err)
	}
	if ids, err := svc.ClubIDs(ctx, bob); err != nil || len(ids) != 1 || ids[0] != c.ID() {
		t.Fatalf("bob's club IDs = %v, %v", ids, err)
	}
}

func TestClubService_Appoint(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := newService()
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()
	c := create(t, svc, ann)

	if _, err := svc.Join(ctx, bob, c.ID()); err != nil {
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

	if err := svc.Appoint(ctx, ann, c.ID(), bob, club.RoleAdmin); err != nil {
		t.Fatal(err)
	}

	admins, err := svc.Members(ctx, c.ID(), uuid.Nil, 10, true)
	if err != nil || len(admins.Members) != 2 || admins.Total != 2 {
		t.Fatalf("admins = %+v, %v; the owner and bob", admins, err)
	}
	if all, err := svc.Members(ctx, c.ID(), uuid.Nil, 1, false); err != nil || len(all.Members) != 1 || all.Total != 2 {
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
	if err = svc.Appoint(ctx, bob, c.ID(), bob, club.RoleMember); !errors.Is(err, club.ErrCannotAppoint) {
		t.Fatalf("the admin appoints: %v", err)
	}

	if err = svc.Appoint(ctx, ann, c.ID(), bob, club.RoleMember); err != nil {
		t.Fatal(err)
	}
	if ok, _ := svc.CanModerateWall(ctx, c.ID(), bob); ok {
		t.Fatal("bob is a member again")
	}
}

func TestClubService_Transfer(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := newService()
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()
	c := create(t, svc, ann)

	for _, id := range []uuid.UUID{bob, cat} {
		if _, err := svc.Join(ctx, id, c.ID()); err != nil {
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

	if _, err := svc.Join(ctx, bob, c.ID()); err != nil {
		t.Fatal(err)
	}

	if err := svc.PurgeUser(ctx, ann); err != nil {
		t.Fatal(err)
	}
	if err := svc.PurgeUser(ctx, ann); err != nil {
		t.Fatalf("purge twice: %v", err)
	}

	v, err := svc.Find(ctx, bob, service.Ref{ID: c.ID()})
	if err != nil || v.OwnerID != uuid.Nil || v.Club.Members() != 1 || v.Role != club.RoleMember {
		t.Fatalf("after the owner is erased = %+v, %v; the club stays with bob", v, err)
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
	if _, err = svc.Join(ctx, bob, cats.ID()); err != nil {
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
	all, err := svc.Clubs(ctx, bob, uuid.Nil, 10, false)
	if err != nil || len(all.Clubs) != 2 || all.Total != 2 {
		t.Fatalf("bob's clubs = %+v, %v", all, err)
	}
	managed, err := svc.Clubs(ctx, bob, uuid.Nil, 10, true)
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
	if managed, _ = svc.Clubs(ctx, bob, uuid.Nil, 10, true); !managed.Clubs[0].Pinned {
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
