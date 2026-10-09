package grpc_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestBackdrop(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, _ := s.member(t, "ann")
	first, second := uuid.NewString(), uuid.NewString()

	set := func(side userv1.BackdropSide, photo string) *userv1.SetBackdropResponse {
		t.Helper()

		resp, err := s.client.SetBackdrop(ann, &userv1.SetBackdropRequest{Side: side, PhotoId: photo})
		if err != nil {
			t.Fatalf("SetBackdrop: %v", err)
		}
		return resp
	}

	if resp := set(userv1.BackdropSide_BACKDROP_SIDE_LEFT, first); resp.GetPreviousPhotoId() != "" || resp.GetUser().GetBackdropLeft() != first {
		t.Fatalf("first: %v", resp)
	}
	if resp := set(userv1.BackdropSide_BACKDROP_SIDE_LEFT, second); resp.GetPreviousPhotoId() != first {
		t.Fatalf("second replaced %q", resp.GetPreviousPhotoId())
	}
	set(userv1.BackdropSide_BACKDROP_SIDE_RIGHT, first)

	got, err := s.client.GetProfile(context.Background(), &userv1.GetProfileRequest{Nickname: "ann"})
	if p := got.GetProfile(); err != nil || p.GetBackdropLeft() != second || p.GetBackdropRight() != first {
		t.Fatalf("GetProfile = %v, %v", p, err)
	}

	if resp := set(userv1.BackdropSide_BACKDROP_SIDE_RIGHT, ""); resp.GetPreviousPhotoId() != first || resp.GetUser().GetBackdropRight() != "" {
		t.Fatalf("clear: %v", resp)
	}

	for name, req := range map[string]*userv1.SetBackdropRequest{
		"no side":  {PhotoId: first},
		"a bad id": {Side: userv1.BackdropSide_BACKDROP_SIDE_LEFT, PhotoId: "x"},
	} {
		if _, err = s.client.SetBackdrop(ann, req); status.Code(err) != codes.InvalidArgument {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err = s.client.SetBackdrop(context.Background(), &userv1.SetBackdropRequest{Side: userv1.BackdropSide_BACKDROP_SIDE_LEFT}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("without a token: %v", err)
	}
}
