package grpc_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestBanUser(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	admin := s.signIn(t, adminEmail)
	ann, annID := s.member(t, "ann")
	bob, _ := s.member(t, "bob")
	day := timestamppb.New(time.Now().Add(24 * time.Hour))

	for _, tt := range []struct {
		name string
		call func() error
		want codes.Code
	}{
		{"by ann", func() error {
			_, err := s.client.BanUser(ann, &userv1.BanUserRequest{UserId: annID, Reason: "spam"})
			return err
		}, codes.PermissionDenied},
		{"an unknown page", func() error {
			_, err := s.client.BanUser(admin, &userv1.BanUserRequest{UserId: uuid.NewString(), Reason: "spam"})
			return err
		}, codes.NotFound},
		{"no reason", func() error {
			_, err := s.client.BanUser(admin, &userv1.BanUserRequest{UserId: annID, Reason: " "})
			return err
		}, codes.InvalidArgument},
		{"in the past", func() error {
			_, err := s.client.BanUser(admin, &userv1.BanUserRequest{UserId: annID, Reason: "spam", Until: timestamppb.New(time.Now().Add(-time.Hour))})
			return err
		}, codes.InvalidArgument},
	} {
		if err := tt.call(); status.Code(err) != tt.want {
			t.Fatalf("%s: %v, want %v", tt.name, err, tt.want)
		}
	}

	if _, err := s.client.BanUser(admin, &userv1.BanUserRequest{UserId: annID, Reason: "spam", Until: day}); err != nil {
		t.Fatal(err)
	}

	// Ann reads why; others see her name and the block, not her page.
	me, err := s.client.GetMe(ann, &userv1.GetMeRequest{})
	if err != nil || me.GetUser().GetBan().GetReason() != "spam" || !me.GetUser().GetBan().GetUntil().AsTime().Equal(day.AsTime().Truncate(time.Microsecond)) {
		t.Fatalf("ann's GetMe = %v, %v", me, err)
	}
	p, err := s.client.GetProfile(bob, &userv1.GetProfileRequest{Nickname: "ann"})
	if err != nil || p.GetProfile().GetBan().GetReason() != "spam" || !p.GetProfile().GetInfoHidden() {
		t.Fatalf("GetProfile = %v, %v", p, err)
	}

	found, err := s.client.SearchUsers(admin, &userv1.SearchUsersRequest{Banned: true})
	if err != nil || len(found.GetUsers()) != 1 || found.GetUsers()[0].GetId() != annID || found.GetUsers()[0].GetBan() == nil {
		t.Fatalf("SearchUsers = %v, %v", found, err)
	}

	if _, err = s.client.UnbanUser(admin, &userv1.UnbanUserRequest{UserId: annID}); err != nil {
		t.Fatal(err)
	}
	if p, err = s.client.GetProfile(bob, &userv1.GetProfileRequest{Nickname: "ann"}); err != nil || p.GetProfile().GetBan() != nil {
		t.Fatalf("GetProfile after Unban = %v, %v", p, err)
	}
}
