package grpc_test

import (
	"context"
	"testing"

	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestDecor(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	admin := s.signIn(t, adminEmail)
	ann, _ := s.member(t, "ann")

	// Anyone, even without a token, asks how the site is dressed.
	if got, err := s.client.GetDecor(context.Background(), &userv1.GetDecorRequest{}); err != nil || got.GetDecor() != userv1.Decor_DECOR_AUTO {
		t.Fatalf("GetDecor = %v, %v", got, err)
	}

	if _, err := s.client.SetDecor(ann, &userv1.SetDecorRequest{Decor: userv1.Decor_DECOR_ON}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("SetDecor by ann: %v", err)
	}
	if _, err := s.client.SetDecor(admin, &userv1.SetDecorRequest{}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("SetDecor unspecified: %v", err)
	}
	if _, err := s.client.SetDecor(admin, &userv1.SetDecorRequest{Decor: userv1.Decor_DECOR_ON}); err != nil {
		t.Fatalf("SetDecor: %v", err)
	}
	if got, err := s.client.GetDecor(ann, &userv1.GetDecorRequest{}); err != nil || got.GetDecor() != userv1.Decor_DECOR_ON || !got.GetFestive() {
		t.Fatalf("GetDecor after = %v, %v", got, err)
	}
}
