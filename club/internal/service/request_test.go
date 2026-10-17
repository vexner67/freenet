package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/club/internal/club"
	"github.com/vexner67/freenet/club/internal/service"
)

// closeClub makes the club of ownerID a closed one.
func closeClub(t *testing.T, svc *service.ClubService, ownerID uuid.UUID, c *club.Club, closed bool) service.View {
	t.Helper()

	v, err := svc.Update(context.Background(), ownerID, c.ID(), club.Changes{Name: c.Name(), Wall: club.WallOpen, Closed: closed})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	return v
}

func TestClubService_ClosedClub(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc, n := newNotified()
	ann, bob, cat, dan := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	c := create(t, svc, ann)

	// Bob joined while the club was open and stays in it.
	if _, err := svc.Join(ctx, bob, c.ID()); err != nil {
		t.Fatal(err)
	}

	if v := closeClub(t, svc, ann, c, true); !v.Club.Closed() || v.Requests != 0 {
		t.Fatalf("closed = %+v", v)
	}
	// Saving the settings again tells the news nothing new.
	closeClub(t, svc, ann, c, true)
	if len(n.walls) != 1 || n.walls[0] != "closed "+c.ID().String() {
		t.Fatalf("the news heard %v, want the club closed once", n.walls)
	}

	// Joining is asking now, twice as once.
	for range 2 {
		v, err := svc.Join(ctx, cat, c.ID())
		if err != nil || v.Role != club.RoleNone || !v.Requested || v.Club.Members() != 2 {
			t.Fatalf("cat asks = %+v, %v", v, err)
		}
	}
	if _, err := svc.Join(ctx, dan, c.ID()); err != nil {
		t.Fatal(err)
	}
	if n.readers[[2]uuid.UUID{c.ID(), cat}] {
		t.Fatal("the news read the club to someone who only asked")
	}

	// A member who joins again asks for nothing.
	if v, err := svc.Join(ctx, bob, c.ID()); err != nil || v.Requested || v.Role != club.RoleMember {
		t.Fatalf("bob joins again = %+v, %v", v, err)
	}

	// Those outside see neither the members nor the wall to write on.
	for _, viewer := range []uuid.UUID{uuid.Nil, cat} {
		if _, err := svc.Members(ctx, viewer, c.ID(), uuid.Nil, 10, service.PickAll); !errors.Is(err, club.ErrClosed) {
			t.Fatalf("%v reads the members: %v", viewer, err)
		}
	}
	if page, err := svc.Members(ctx, bob, c.ID(), uuid.Nil, 10, service.PickAll); err != nil || len(page.Members) != 2 {
		t.Fatalf("bob reads the members: %v, %v", page.Members, err)
	}
	if access, err := svc.CanPostOnWall(ctx, cat, c.ID(), false); err != nil || access.Post || access.Suggest {
		t.Fatalf("cat writes on the wall: %+v, %v", access, err)
	}
	if access, err := svc.CanPostOnWall(ctx, bob, c.ID(), false); err != nil || !access.Post {
		t.Fatalf("bob writes on the wall: %+v, %v", access, err)
	}

	// Only those who run the club see and decide the requests.
	if _, err := svc.Requests(ctx, bob, c.ID(), uuid.Nil, 10); !errors.Is(err, club.ErrCannotDecide) {
		t.Fatalf("a member reads the requests: %v", err)
	}
	if err := svc.Accept(ctx, bob, c.ID(), cat); !errors.Is(err, club.ErrCannotDecide) {
		t.Fatalf("a member accepts: %v", err)
	}
	if err := svc.Reject(ctx, cat, c.ID(), cat); !errors.Is(err, club.ErrCannotDecide) {
		t.Fatalf("a stranger rejects: %v", err)
	}

	page, err := svc.Requests(ctx, ann, c.ID(), uuid.Nil, 1)
	if err != nil || page.Total != 2 || len(page.Requests) != 1 || page.Requests[0].UserID != dan || page.NextCursor == uuid.Nil {
		t.Fatalf("the first page of requests = %+v, %v", page, err)
	}
	if page, err = svc.Requests(ctx, ann, c.ID(), page.NextCursor, 1); err != nil || len(page.Requests) != 1 || page.Requests[0].UserID != cat || page.NextCursor != uuid.Nil {
		t.Fatalf("the second page of requests = %+v, %v", page, err)
	}
	if v, err := svc.Find(ctx, ann, service.Ref{ID: c.ID()}); err != nil || v.Requests != 2 || v.Requested {
		t.Fatalf("ann's view = %+v, %v", v, err)
	}
	if v, err := svc.Find(ctx, bob, service.Ref{ID: c.ID()}); err != nil || v.Requests != 0 {
		t.Fatalf("a member is told the requests: %+v, %v", v, err)
	}

	if err = svc.Accept(ctx, ann, c.ID(), cat); err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if err = svc.Accept(ctx, ann, c.ID(), cat); !errors.Is(err, club.ErrNoRequest) {
		t.Fatalf("Accept twice: %v", err)
	}
	if v, err := svc.Find(ctx, cat, service.Ref{ID: c.ID()}); err != nil || v.Role != club.RoleMember || v.Requested || v.Club.Members() != 3 {
		t.Fatalf("cat accepted = %+v, %v", v, err)
	}
	if !n.readers[[2]uuid.UUID{c.ID(), cat}] {
		t.Fatal("the news do not read the club to the accepted")
	}

	if err = svc.Reject(ctx, ann, c.ID(), dan); err != nil {
		t.Fatalf("Reject: %v", err)
	}
	if err = svc.Reject(ctx, ann, c.ID(), dan); !errors.Is(err, club.ErrNoRequest) {
		t.Fatalf("Reject twice: %v", err)
	}
	if v, err := svc.Find(ctx, dan, service.Ref{ID: c.ID()}); err != nil || v.Role != club.RoleNone || v.Requested {
		t.Fatalf("dan rejected = %+v, %v", v, err)
	}

	// Leaving takes a request back.
	if _, err = svc.Join(ctx, dan, c.ID()); err != nil {
		t.Fatal(err)
	}
	if v, err := svc.Leave(ctx, dan, c.ID()); err != nil || v.Requested {
		t.Fatalf("dan takes the request back = %+v, %v", v, err)
	}

	// An erased user asks for nothing.
	if _, err = svc.Join(ctx, dan, c.ID()); err != nil {
		t.Fatal(err)
	}
	if err = svc.PurgeUser(ctx, dan); err != nil {
		t.Fatal(err)
	}
	if page, err = svc.Requests(ctx, ann, c.ID(), uuid.Nil, 10); err != nil || page.Total != 0 {
		t.Fatalf("requests after the erasure = %+v, %v", page, err)
	}

	// A club that opens forgets who asked: they just join.
	if _, err = svc.Join(ctx, dan, c.ID()); err != nil {
		t.Fatal(err)
	}
	if v := closeClub(t, svc, ann, c, false); v.Club.Closed() {
		t.Fatal("the club is still closed")
	}
	if len(n.walls) != 2 || n.walls[1] != "opened "+c.ID().String() {
		t.Fatalf("the news heard %v", n.walls)
	}
	if v, err := svc.Join(ctx, dan, c.ID()); err != nil || v.Role != club.RoleMember || v.Requested {
		t.Fatalf("dan joins the opened club = %+v, %v", v, err)
	}
	closeClub(t, svc, ann, c, true)
	if page, err = svc.Requests(ctx, ann, c.ID(), uuid.Nil, 10); err != nil || page.Total != 0 {
		t.Fatalf("requests of the club closed again = %+v, %v", page, err)
	}

	if _, err = svc.Requests(ctx, ann, uuid.New(), uuid.Nil, 10); !errors.Is(err, club.ErrNotFound) {
		t.Fatalf("requests of an unknown club: %v", err)
	}
}
