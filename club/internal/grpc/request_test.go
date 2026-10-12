package grpc_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	clubv1 "github.com/vexner67/freenet/club/api/club/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestJoinRequests(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()

	created, err := s.client.CreateClub(s.as(t, ann), &clubv1.CreateClubRequest{Name: "Кошки"})
	if err != nil {
		t.Fatal(err)
	}
	id := created.GetClub().GetClub().GetId()

	closed, err := s.client.UpdateClub(s.as(t, ann), &clubv1.UpdateClubRequest{
		ClubId: id, Name: "Кошки", Wall: clubv1.Wall_WALL_OPEN, AdminsDisplay: clubv1.AdminsDisplay_ADMINS_DISPLAY_CREATOR, Closed: true,
	})
	if err != nil || !closed.GetClub().GetClub().GetClosed() {
		t.Fatalf("UpdateClub: %v, %v", closed, err)
	}

	for _, who := range []uuid.UUID{bob, cat} {
		joined, err := s.client.JoinClub(s.as(t, who), &clubv1.JoinClubRequest{ClubId: id})
		if err != nil || !joined.GetClub().GetRequested() || joined.GetClub().GetRole() != clubv1.Role_ROLE_NONE {
			t.Fatalf("JoinClub = %v, %v; want a request", joined, err)
		}
	}

	// A closed club tells who is in it to its members only, but itself to
	// anyone.
	got, err := s.client.GetClub(context.Background(), &clubv1.GetClubRequest{Ref: id})
	if err != nil || !got.GetClub().GetClub().GetClosed() || got.GetClub().GetRequestCount() != 0 {
		t.Fatalf("GetClub = %v, %v", got, err)
	}
	if _, err = s.client.ListMembers(s.as(t, bob), &clubv1.ListMembersRequest{ClubId: id}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("ListMembers for a stranger: %v", err)
	}

	if _, err = s.client.ListJoinRequests(s.as(t, bob), &clubv1.ListJoinRequestsRequest{ClubId: id}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("ListJoinRequests for a stranger: %v", err)
	}
	if _, err = s.client.ListJoinRequests(context.Background(), &clubv1.ListJoinRequestsRequest{ClubId: id}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("ListJoinRequests without a token: %v", err)
	}

	page, err := s.client.ListJoinRequests(s.as(t, ann), &clubv1.ListJoinRequestsRequest{ClubId: id})
	if err != nil || page.GetTotal() != 2 || len(page.GetUserIds()) != 2 || page.GetUserIds()[0] != cat.String() {
		t.Fatalf("ListJoinRequests = %v, %v", page, err)
	}
	if got, err = s.client.GetClub(s.as(t, ann), &clubv1.GetClubRequest{Ref: id}); err != nil || got.GetClub().GetRequestCount() != 2 {
		t.Fatalf("the owner's GetClub = %v, %v", got, err)
	}

	if _, err = s.client.AcceptJoinRequest(s.as(t, bob), &clubv1.AcceptJoinRequestRequest{ClubId: id, UserId: bob.String()}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("accepting oneself: %v", err)
	}
	if _, err = s.client.AcceptJoinRequest(s.as(t, ann), &clubv1.AcceptJoinRequestRequest{ClubId: id, UserId: bob.String()}); err != nil {
		t.Fatalf("AcceptJoinRequest: %v", err)
	}
	if _, err = s.client.RejectJoinRequest(s.as(t, ann), &clubv1.RejectJoinRequestRequest{ClubId: id, UserId: cat.String()}); err != nil {
		t.Fatalf("RejectJoinRequest: %v", err)
	}

	for name, call := range map[string]func() error{
		"accepting nobody": func() error {
			_, err := s.client.AcceptJoinRequest(s.as(t, ann), &clubv1.AcceptJoinRequestRequest{ClubId: id, UserId: cat.String()})
			return err
		},
		"rejecting nobody": func() error {
			_, err := s.client.RejectJoinRequest(s.as(t, ann), &clubv1.RejectJoinRequestRequest{ClubId: id, UserId: cat.String()})
			return err
		},
		"an unknown club": func() error {
			_, err := s.client.AcceptJoinRequest(s.as(t, ann), &clubv1.AcceptJoinRequestRequest{ClubId: uuid.NewString(), UserId: cat.String()})
			return err
		},
	} {
		if err := call(); status.Code(err) != codes.NotFound {
			t.Errorf("%s: %v, want NotFound", name, err)
		}
	}

	for name, call := range map[string]func() error{
		"a bad club": func() error {
			_, err := s.client.AcceptJoinRequest(s.as(t, ann), &clubv1.AcceptJoinRequestRequest{ClubId: "x", UserId: cat.String()})
			return err
		},
		"a bad user": func() error {
			_, err := s.client.RejectJoinRequest(s.as(t, ann), &clubv1.RejectJoinRequestRequest{ClubId: id, UserId: "x"})
			return err
		},
		"a bad cursor": func() error {
			_, err := s.client.ListJoinRequests(s.as(t, ann), &clubv1.ListJoinRequestsRequest{ClubId: id, Cursor: "x"})
			return err
		},
	} {
		if err := call(); status.Code(err) != codes.InvalidArgument {
			t.Errorf("%s: %v, want InvalidArgument", name, err)
		}
	}

	members, err := s.client.ListMembers(s.as(t, bob), &clubv1.ListMembersRequest{ClubId: id})
	if err != nil || members.GetTotal() != 2 {
		t.Fatalf("ListMembers for the accepted = %v, %v", members, err)
	}
}
