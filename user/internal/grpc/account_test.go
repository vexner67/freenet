package grpc_test

import (
	"context"
	"testing"

	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestAccountFlow(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann := s.signIn(t, "ann@example.com")
	bob := s.signIn(t, "bob@example.com")

	for ctx, nickname := range map[context.Context]string{ann: "ann", bob: "bob"} {
		if _, err := s.client.UpdateProfile(ctx, &userv1.UpdateProfileRequest{Name: "Имя", Nickname: nickname, Status: "привет"}); err != nil {
			t.Fatalf("UpdateProfile(%s): %v", nickname, err)
		}
	}

	resp, err := s.client.DeactivateAccount(ann, &userv1.DeactivateAccountRequest{Reason: "надоело"})
	if err != nil {
		t.Fatalf("DeactivateAccount: %v", err)
	}
	if u := resp.GetUser(); !u.GetDeactivated() || u.GetPurgeAt() == nil {
		t.Fatalf("deactivated user = %v", u)
	}

	// Signing in again gives the restricted token of a deleted page.
	ann = s.signIn(t, "ann@example.com")

	if _, err = s.client.UpdateProfile(ann, &userv1.UpdateProfileRequest{Name: "Имя", Nickname: "ann"}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("UpdateProfile of a deleted page: %v, want PermissionDenied", err)
	}
	if me, err := s.client.GetMe(ann, &userv1.GetMeRequest{}); err != nil || !me.GetUser().GetDeactivated() {
		t.Fatalf("GetMe of a deleted page = %v, %v", me, err)
	}
	if _, err = s.client.GetProfile(ann, &userv1.GetProfileRequest{Nickname: "bob"}); err != nil {
		t.Fatalf("public GetProfile with the token of a deleted page: %v", err)
	}

	// Others see the name only and cannot follow or write.
	p, err := s.client.GetProfile(bob, &userv1.GetProfileRequest{Nickname: "ann"})
	if err != nil || !p.GetProfile().GetDeactivated() || p.GetProfile().GetName() != "Имя" || p.GetProfile().GetStatus() != "" {
		t.Fatalf("profile of a deleted page = %v, %v", p, err)
	}
	annID := p.GetProfile().GetId()
	if _, err = s.client.Follow(bob, &userv1.FollowRequest{UserId: annID}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("Follow a deleted page: %v, want PermissionDenied", err)
	}
	if can, err := s.client.CanMessage(bob, &userv1.CanMessageRequest{UserId: annID}); err != nil || can.GetAllowed() {
		t.Fatalf("CanMessage a deleted page = %v, %v", can, err)
	}

	if state, err := s.client.GetAccountState(context.Background(), &userv1.GetAccountStateRequest{UserId: annID}); err != nil || !state.GetDeactivated() {
		t.Fatalf("GetAccountState = %v, %v", state, err)
	}

	if r, err := s.client.ReactivateAccount(ann, &userv1.ReactivateAccountRequest{}); err != nil || r.GetUser().GetDeactivated() {
		t.Fatalf("ReactivateAccount = %v, %v", r, err)
	}
	if _, err = s.client.ReactivateAccount(ann, &userv1.ReactivateAccountRequest{}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("ReactivateAccount twice: %v, want FailedPrecondition", err)
	}
	if _, err = s.client.Follow(bob, &userv1.FollowRequest{UserId: annID}); err != nil {
		t.Fatalf("Follow the restored page: %v", err)
	}
}
