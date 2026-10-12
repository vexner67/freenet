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
	ui := me.GetUser().GetInterface()
	if err != nil || ui.GetAvatarStyle() != "square" || ui.GetMainPage() != "page" || len(ui.GetHiddenMenu()) != 0 ||
		ui.GetTheme() != "default" || ui.GetHideRating() || ui.GetNsfw() != "blur" || ui.GetPostView() != "old" || ui.TimezoneOffset != nil {
		t.Fatalf("a new user = %v, %v", ui, err)
	}

	offset := int32(180)
	got, err := s.client.UpdateInterface(ann, &userv1.UpdateInterfaceRequest{Interface: &userv1.InterfaceSettings{
		AvatarStyle: "round", MainPage: "feed", HiddenMenu: []string{"notifications", "groups"},
		Theme: "dark", HideRating: true, Nsfw: "show", PostView: "microblog", TimezoneOffset: &offset,
	}})
	ui = got.GetUser().GetInterface()
	if err != nil || ui.GetAvatarStyle() != "round" || ui.GetMainPage() != "feed" ||
		!slices.Equal(ui.GetHiddenMenu(), []string{"groups", "notifications"}) ||
		ui.GetTheme() != "dark" || !ui.GetHideRating() || ui.GetNsfw() != "show" || ui.GetPostView() != "microblog" ||
		ui.TimezoneOffset == nil || ui.GetTimezoneOffset() != 180 {
		t.Fatalf("UpdateInterface = %v, %v", ui, err)
	}
	if me, err = s.client.GetMe(ann, &userv1.GetMeRequest{}); err != nil || me.GetUser().GetInterface().GetTheme() != "dark" {
		t.Fatalf("GetMe after the change = %v, %v", me, err)
	}

	// A client older than the new settings names none of them: defaults.
	got, err = s.client.UpdateInterface(ann, &userv1.UpdateInterfaceRequest{Interface: &userv1.InterfaceSettings{
		AvatarStyle: "square", MainPage: "page",
	}})
	ui = got.GetUser().GetInterface()
	if err != nil || ui.GetTheme() != "default" || ui.GetNsfw() != "blur" || ui.GetPostView() != "old" || ui.TimezoneOffset != nil {
		t.Fatalf("an older client = %v, %v", ui, err)
	}

	off := int32(1000)
	for name, bad := range map[string]*userv1.InterfaceSettings{
		"no settings":      nil,
		"an unknown style": {AvatarStyle: "oval", MainPage: "page"},
		"an unknown page":  {AvatarStyle: "square", MainPage: "search"},
		"an unknown link":  {AvatarStyle: "square", MainPage: "page", HiddenMenu: []string{"settings"}},
		"an unknown theme": {AvatarStyle: "square", MainPage: "page", Theme: "pink"},
		"an unknown nsfw":  {AvatarStyle: "square", MainPage: "page", Nsfw: "maybe"},
		"an unknown view":  {AvatarStyle: "square", MainPage: "page", PostView: "blog"},
		"a clock too far":  {AvatarStyle: "square", MainPage: "page", TimezoneOffset: &off},
	} {
		if _, err = s.client.UpdateInterface(ann, &userv1.UpdateInterfaceRequest{Interface: bad}); status.Code(err) != codes.InvalidArgument {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err = s.client.UpdateInterface(context.Background(), &userv1.UpdateInterfaceRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("without a token: %v", err)
	}
}

// The address of the page changes alone: the rest of the profile stays
// as another request left it.
func TestUpdateNickname(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, _ := s.member(t, "ann")
	s.member(t, "bob")

	if _, err := s.client.UpdateStatus(ann, &userv1.UpdateStatusRequest{Status: "Привет"}); err != nil {
		t.Fatal(err)
	}

	got, err := s.client.UpdateNickname(ann, &userv1.UpdateNicknameRequest{Nickname: "anna"})
	if u := got.GetUser(); err != nil || u.GetNickname() != "anna" || u.GetName() != "ann" || u.GetStatus() != "Привет" {
		t.Fatalf("UpdateNickname = %v, %v", got, err)
	}
	if _, err = s.client.GetProfile(context.Background(), &userv1.GetProfileRequest{Nickname: "ann"}); status.Code(err) != codes.NotFound {
		t.Fatalf("the old address: %v", err)
	}

	tests := []struct {
		name     string
		ctx      context.Context
		nickname string
		want     codes.Code
	}{
		{name: "taken", ctx: ann, nickname: "Bob", want: codes.AlreadyExists},
		{name: "empty", ctx: ann, nickname: "", want: codes.InvalidArgument},
		{name: "malformed", ctx: ann, nickname: "1ann", want: codes.InvalidArgument},
		{name: "reserved", ctx: ann, nickname: "settings", want: codes.InvalidArgument},
		{name: "unfilled page", ctx: s.signIn(t, "new@example.com"), nickname: "newcomer", want: codes.InvalidArgument},
		{name: "without a token", ctx: context.Background(), nickname: "anna2", want: codes.Unauthenticated},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := s.client.UpdateNickname(tt.ctx, &userv1.UpdateNicknameRequest{Nickname: tt.nickname}); status.Code(err) != tt.want {
				t.Errorf("UpdateNickname(%q) = %v, want %v", tt.nickname, err, tt.want)
			}
		})
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
