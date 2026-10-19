package grpc_test

import (
	"slices"
	"testing"

	"github.com/google/uuid"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestSetVerified(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	admin := s.signIn(t, adminEmail)
	ann, annID := s.member(t, "ann")

	if _, err := s.client.SetVerified(ann, &userv1.SetVerifiedRequest{UserId: annID, Verified: true}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("SetVerified by ann: %v", err)
	}
	if _, err := s.client.SetVerified(admin, &userv1.SetVerifiedRequest{UserId: uuid.NewString(), Verified: true}); status.Code(err) != codes.NotFound {
		t.Fatalf("an unknown page: %v", err)
	}
	if _, err := s.client.SetVerified(admin, &userv1.SetVerifiedRequest{UserId: "x", Verified: true}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("a wrong ID: %v", err)
	}
	if _, err := s.client.SetVerified(admin, &userv1.SetVerifiedRequest{UserId: annID, Verified: true}); err != nil {
		t.Fatal(err)
	}

	if me, err := s.client.GetMe(ann, &userv1.GetMeRequest{}); err != nil || !me.GetUser().GetVerified() {
		t.Fatalf("ann's GetMe = %v, %v", me, err)
	}
	if p, err := s.client.GetProfile(admin, &userv1.GetProfileRequest{Nickname: "ann"}); err != nil || !p.GetProfile().GetVerified() {
		t.Fatalf("GetProfile = %v, %v", p, err)
	}

	found, err := s.client.SearchUsers(admin, &userv1.SearchUsersRequest{Verified: true})
	if err != nil || len(found.GetUsers()) != 1 || found.GetUsers()[0].GetId() != annID || !found.GetUsers()[0].GetVerified() {
		t.Fatalf("SearchUsers = %v, %v", found, err)
	}

	// A mention of ann shows the checkmark too.
	known, err := s.client.FindNicknames(ann, &userv1.FindNicknamesRequest{Nicknames: []string{"ANN", "admin_page"}})
	if err != nil || !slices.Equal(known.GetVerified(), []string{"ann"}) {
		t.Fatalf("FindNicknames = %v, %v", known, err)
	}

	if stats, err := s.client.GetStatistics(admin, &userv1.GetStatisticsRequest{}); err != nil || stats.GetVerified() != 1 {
		t.Fatalf("GetStatistics = %v, %v", stats, err)
	}

	if _, err = s.client.SetVerified(admin, &userv1.SetVerifiedRequest{UserId: annID}); err != nil {
		t.Fatal(err)
	}
	if p, err := s.client.GetProfile(admin, &userv1.GetProfileRequest{Nickname: "ann"}); err != nil || p.GetProfile().GetVerified() {
		t.Fatalf("GetProfile after taking = %v, %v", p, err)
	}
}
