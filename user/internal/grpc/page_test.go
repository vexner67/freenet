package grpc_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestInterface(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, _ := s.member(t, "ann")

	me, err := s.client.GetMe(ann, &userv1.GetMeRequest{})
	if ui := me.GetUser().GetInterface(); err != nil || ui.GetAvatarStyle() != "square" || ui.GetMainPage() != "page" || len(ui.GetHiddenMenu()) != 0 {
		t.Fatalf("a new user = %v, %v", ui, err)
	}

	got, err := s.client.UpdateInterface(ann, &userv1.UpdateInterfaceRequest{Interface: &userv1.InterfaceSettings{
		AvatarStyle: "round", MainPage: "feed", HiddenMenu: []string{"notifications", "groups"},
	}})
	if ui := got.GetUser().GetInterface(); err != nil || ui.GetAvatarStyle() != "round" || ui.GetMainPage() != "feed" ||
		!slices.Equal(ui.GetHiddenMenu(), []string{"groups", "notifications"}) {
		t.Fatalf("UpdateInterface = %v, %v", ui, err)
	}
	if me, err = s.client.GetMe(ann, &userv1.GetMeRequest{}); err != nil || me.GetUser().GetInterface().GetMainPage() != "feed" {
		t.Fatalf("GetMe after the change = %v, %v", me, err)
	}

	for name, bad := range map[string]*userv1.InterfaceSettings{
		"no settings":      nil,
		"an unknown style": {AvatarStyle: "oval", MainPage: "page"},
		"an unknown page":  {AvatarStyle: "square", MainPage: "search"},
		"an unknown link":  {AvatarStyle: "square", MainPage: "page", HiddenMenu: []string{"settings"}},
	} {
		if _, err = s.client.UpdateInterface(ann, &userv1.UpdateInterfaceRequest{Interface: bad}); status.Code(err) != codes.InvalidArgument {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err = s.client.UpdateInterface(context.Background(), &userv1.UpdateInterfaceRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("without a token: %v", err)
	}
}

func TestStatusAndCompleteness(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, _ := s.member(t, "ann")

	got, err := s.client.UpdateStatus(ann, &userv1.UpdateStatusRequest{Status: "  Привет  "})
	if u := got.GetUser(); err != nil || u.GetStatus() != "Привет" || u.GetName() != "ann" || u.GetCompleteness() != 25 ||
		!slices.Equal(u.GetUnfilled(), []string{"interests", "email", "telegram", "city"}) {
		t.Fatalf("UpdateStatus = %v, %v", got, err)
	}

	if _, err = s.client.UpdateStatus(ann, &userv1.UpdateStatusRequest{Status: strings.Repeat("я", 201)}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("too long: %v", err)
	}
	if _, err = s.client.UpdateStatus(context.Background(), &userv1.UpdateStatusRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("without a token: %v", err)
	}

	page, err := s.client.GetProfile(context.Background(), &userv1.GetProfileRequest{Nickname: "ann"})
	if err != nil || page.GetProfile().GetCompleteness() != 25 || page.GetProfile().GetStatus() != "Привет" {
		t.Fatalf("GetProfile = %v, %v", page, err)
	}
}

func TestCommonFollowing(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, annID := s.member(t, "ann")
	bob, _ := s.member(t, "bob")
	_, catID := s.member(t, "cat")
	_, danID := s.member(t, "dan")

	follow := func(ctx context.Context, ids ...string) {
		t.Helper()
		for _, id := range ids {
			if _, err := s.client.Follow(ctx, &userv1.FollowRequest{UserId: id}); err != nil {
				t.Fatal(err)
			}
		}
	}
	follow(ann, catID, danID)
	follow(bob, catID)

	common, err := s.client.ListFollowing(bob, &userv1.ListFollowsRequest{UserId: annID, Common: true})
	if err != nil || common.GetTotal() != 1 || len(common.GetUsers()) != 1 || common.GetUsers()[0].GetId() != catID {
		t.Fatalf("common = %v, %v", common, err)
	}

	if _, err = s.client.ListFollowing(context.Background(), &userv1.ListFollowsRequest{UserId: annID, Common: true}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("anonymous: %v", err)
	}
}
