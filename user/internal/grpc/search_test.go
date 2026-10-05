package grpc_test

import (
	"context"
	"testing"

	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestSearchUsers(t *testing.T) {
	t.Parallel()

	s := newServer(t)

	for _, p := range []struct{ email, name, surname, nickname string }{
		{"ann@example.com", "Анна", "Ли", "ann"},
		{"anya@example.com", "Анна", "Петрова", "anya"},
		{"bob@example.com", "Борис", "Ли", "bob"},
	} {
		ctx := s.signIn(t, p.email)
		if _, err := s.client.UpdateProfile(ctx, &userv1.UpdateProfileRequest{Name: p.name, Surname: p.surname, Nickname: p.nickname}); err != nil {
			t.Fatal(err)
		}
	}
	s.signIn(t, "ghost@example.com") // no profile: must not be found

	search := func(query, cursor string, limit int32) *userv1.SearchUsersResponse {
		t.Helper()

		resp, err := s.client.SearchUsers(context.Background(), &userv1.SearchUsersRequest{Query: query, Cursor: cursor, Limit: limit})
		if err != nil {
			t.Fatalf("SearchUsers(%q): %v", query, err)
		}
		return resp
	}

	nicknames := func(resp *userv1.SearchUsersResponse) []string {
		var out []string
		for _, u := range resp.GetUsers() {
			out = append(out, u.GetNickname())
		}
		return out
	}

	if got := nicknames(search("ли анна", "", 0)); len(got) != 1 || got[0] != "ann" {
		t.Fatalf("any word order: %v", got)
	}
	if got := nicknames(search("АННА", "", 0)); len(got) != 2 {
		t.Fatalf("case-insensitive: %v", got)
	}
	if got := nicknames(search("ли", "", 0)); len(got) != 2 || got[0] != "bob" {
		t.Fatalf("newest first: %v", got)
	}
	if got := search("ghost", "", 0); len(got.GetUsers()) != 0 {
		t.Fatalf("users without a profile are not found: %v", nicknames(got))
	}

	first := search("анна", "", 1)
	if len(first.GetUsers()) != 1 || first.GetNextCursor() == "" {
		t.Fatalf("first page: %v", first)
	}
	if last := search("анна", first.GetNextCursor(), 1); len(last.GetUsers()) != 1 || last.GetNextCursor() != "" ||
		last.GetUsers()[0].GetId() == first.GetUsers()[0].GetId() {
		t.Fatalf("last page: %v", last)
	}

	_, err := s.client.SearchUsers(context.Background(), &userv1.SearchUsersRequest{Query: "  "})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("blank query: %v", err)
	}
}

func TestSetAvatar(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, annID := s.member(t, "ann")
	avatar := "01a10000-0000-7000-8000-00000000a0a0"

	resp, err := s.client.SetAvatar(ann, &userv1.SetAvatarRequest{AvatarId: avatar})
	if err != nil || resp.GetUser().GetAvatarId() != avatar {
		t.Fatalf("SetAvatar: %v, %v", resp, err)
	}

	profiles, err := s.client.GetProfiles(context.Background(), &userv1.GetProfilesRequest{UserIds: []string{annID}})
	if err != nil || profiles.GetProfiles()[0].GetAvatarId() != avatar {
		t.Fatalf("the photo is public: %v, %v", profiles, err)
	}

	resp, err = s.client.SetAvatar(ann, &userv1.SetAvatarRequest{})
	if err != nil || resp.GetUser().GetAvatarId() != "" {
		t.Fatalf("remove: %v, %v", resp, err)
	}

	if _, err = s.client.SetAvatar(ann, &userv1.SetAvatarRequest{AvatarId: "x"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("bad id: %v", err)
	}
	if _, err = s.client.SetAvatar(context.Background(), &userv1.SetAvatarRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("without token: %v", err)
	}
}
