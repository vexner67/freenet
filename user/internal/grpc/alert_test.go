package grpc_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestSetAlert(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	admin := s.signIn(t, adminEmail)
	ann, annID := s.member(t, "ann")

	const label = "Страница под наблюдением"
	put := func(ctx context.Context, id, alert string) error {
		_, err := s.client.SetAlert(ctx, &userv1.SetAlertRequest{UserId: id, Alert: alert})
		return err
	}

	if err := put(ann, annID, label); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("SetAlert by ann: %v", err)
	}
	if err := put(admin, uuid.NewString(), label); status.Code(err) != codes.NotFound {
		t.Fatalf("an unknown page: %v", err)
	}
	if err := put(admin, "x", label); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("a wrong ID: %v", err)
	}
	if err := put(admin, annID, strings.Repeat("я", 1001)); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("a long label: %v", err)
	}
	if err := put(admin, annID, label); err != nil {
		t.Fatal(err)
	}

	if me, err := s.client.GetMe(ann, &userv1.GetMeRequest{}); err != nil || me.GetUser().GetAlert() != label {
		t.Fatalf("ann's GetMe = %v, %v", me, err)
	}
	if p, err := s.client.GetProfile(admin, &userv1.GetProfileRequest{Nickname: "ann"}); err != nil || p.GetProfile().GetAlert() != label {
		t.Fatalf("GetProfile = %v, %v", p, err)
	}
	found, err := s.client.SearchUsers(admin, &userv1.SearchUsersRequest{Query: "ann"})
	if err != nil || len(found.GetUsers()) != 1 || found.GetUsers()[0].GetAlert() != label {
		t.Fatalf("SearchUsers = %v, %v", found, err)
	}

	if err = put(admin, annID, ""); err != nil {
		t.Fatal(err)
	}
	if p, err := s.client.GetProfile(admin, &userv1.GetProfileRequest{Nickname: "ann"}); err != nil || p.GetProfile().GetAlert() != "" {
		t.Fatalf("GetProfile after taking = %v, %v", p, err)
	}
}
