package grpc_test

import (
	"context"
	"testing"

	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestTitles(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	admin := s.signIn(t, adminEmail)
	ann, _ := s.member(t, "ann")
	anonymous := context.Background()

	if _, err := s.client.GrantTitle(ann, &userv1.GrantTitleRequest{Nickname: "ann", Name: "Король", Icon: "crown"}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("GrantTitle by ann: %v", err)
	}
	for _, bad := range []*userv1.GrantTitleRequest{
		{Nickname: "ann", Name: " ", Icon: "crown"},
		{Nickname: "ann", Name: "Король", Icon: "skull"},
	} {
		if _, err := s.client.GrantTitle(admin, bad); status.Code(err) != codes.InvalidArgument {
			t.Fatalf("GrantTitle(%v): %v", bad, err)
		}
	}
	if _, err := s.client.GrantTitle(admin, &userv1.GrantTitleRequest{Nickname: "nobody", Name: "Король", Icon: "crown"}); status.Code(err) != codes.NotFound {
		t.Fatalf("GrantTitle to nobody: %v", err)
	}

	granted, err := s.client.GrantTitle(admin, &userv1.GrantTitleRequest{Nickname: "ann", Name: "Король шахты", Icon: "crown"})
	if err != nil || granted.GetTitle().GetName() != "Король шахты" || granted.GetTitle().GetIcon() != "crown" || granted.GetTitle().GetGrantedAt() == nil {
		t.Fatalf("GrantTitle = %v, %v", granted, err)
	}
	if _, err = s.client.GrantTitle(admin, &userv1.GrantTitleRequest{Nickname: "ann", Name: "Король шахты", Icon: "crown"}); err != nil {
		t.Fatalf("GrantTitle again: %v", err)
	}

	// The titles are public.
	got, err := s.client.GetUserTitles(anonymous, &userv1.GetUserTitlesRequest{Nickname: "ann"})
	if err != nil || len(got.GetTitles()) != 2 {
		t.Fatalf("GetUserTitles = %v, %v", got, err)
	}
	if _, err = s.client.GetUserTitles(anonymous, &userv1.GetUserTitlesRequest{Nickname: "-"}); status.Code(err) != codes.NotFound {
		t.Fatalf("GetUserTitles of a bad nickname: %v", err)
	}

	if _, err = s.client.RevokeTitle(ann, &userv1.RevokeTitleRequest{TitleId: granted.GetTitle().GetId()}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("RevokeTitle by ann: %v", err)
	}
	if _, err = s.client.RevokeTitle(admin, &userv1.RevokeTitleRequest{TitleId: "bad"}); status.Code(err) != codes.NotFound {
		t.Fatalf("RevokeTitle of a bad ID: %v", err)
	}
	if _, err = s.client.RevokeTitle(admin, &userv1.RevokeTitleRequest{TitleId: granted.GetTitle().GetId()}); err != nil {
		t.Fatal(err)
	}
	if got, err = s.client.GetUserTitles(anonymous, &userv1.GetUserTitlesRequest{Nickname: "ann"}); err != nil || len(got.GetTitles()) != 1 {
		t.Fatalf("GetUserTitles after RevokeTitle = %v, %v", got, err)
	}
}
