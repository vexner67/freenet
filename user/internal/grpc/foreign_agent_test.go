package grpc_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestSetForeignAgent(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	admin := s.signIn(t, adminEmail)
	ann, annID := s.member(t, "ann")

	put := func(ctx context.Context, id string, agent bool) error {
		_, err := s.client.SetForeignAgent(ctx, &userv1.SetForeignAgentRequest{UserId: id, ForeignAgent: agent})
		return err
	}

	if err := put(ann, annID, true); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("SetForeignAgent by ann: %v", err)
	}
	if err := put(admin, uuid.NewString(), true); status.Code(err) != codes.NotFound {
		t.Fatalf("an unknown page: %v", err)
	}
	if err := put(admin, "x", true); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("a wrong ID: %v", err)
	}
	if err := put(admin, annID, true); err != nil {
		t.Fatal(err)
	}

	if me, err := s.client.GetMe(ann, &userv1.GetMeRequest{}); err != nil || !me.GetUser().GetForeignAgent() {
		t.Fatalf("ann's GetMe = %v, %v", me, err)
	}
	if p, err := s.client.GetProfile(admin, &userv1.GetProfileRequest{Nickname: "ann"}); err != nil || !p.GetProfile().GetForeignAgent() {
		t.Fatalf("GetProfile = %v, %v", p, err)
	}
	found, err := s.client.SearchUsers(admin, &userv1.SearchUsersRequest{Query: "ann"})
	if err != nil || len(found.GetUsers()) != 1 || !found.GetUsers()[0].GetForeignAgent() {
		t.Fatalf("SearchUsers = %v, %v", found, err)
	}

	if err = put(admin, annID, false); err != nil {
		t.Fatal(err)
	}
	if p, err := s.client.GetProfile(admin, &userv1.GetProfileRequest{Nickname: "ann"}); err != nil || p.GetProfile().GetForeignAgent() {
		t.Fatalf("GetProfile after taking = %v, %v", p, err)
	}
}
