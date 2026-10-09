package grpc_test

import (
	"context"
	"testing"

	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestPresence(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, annID := s.member(t, "ann")
	bob, bobID := s.member(t, "bob")
	_, catID := s.member(t, "cat")

	for _, id := range []string{bobID, catID} {
		if _, err := s.client.Follow(ann, &userv1.FollowRequest{UserId: id}); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := s.client.MarkOnline(context.Background(), &userv1.MarkOnlineRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("without a token: %v", err)
	}
	if _, err := s.client.MarkOnline(bob, &userv1.MarkOnlineRequest{}); err != nil {
		t.Fatalf("MarkOnline: %v", err)
	}

	got, err := s.client.GetProfile(context.Background(), &userv1.GetProfileRequest{Nickname: "bob"})
	if p := got.GetProfile(); err != nil || !p.GetOnline() || p.GetLastSeenAt() == nil {
		t.Fatalf("bob: %v, %v", p, err)
	}
	if got, err = s.client.GetProfile(context.Background(), &userv1.GetProfileRequest{Nickname: "cat"}); err != nil ||
		got.GetProfile().GetOnline() || got.GetProfile().GetLastSeenAt() != nil {
		t.Fatalf("cat, never seen: %v, %v", got, err)
	}

	online, err := s.client.ListFollowing(context.Background(), &userv1.ListFollowsRequest{UserId: annID, Online: true})
	if err != nil || online.GetTotal() != 1 || len(online.GetUsers()) != 1 || online.GetUsers()[0].GetId() != bobID {
		t.Fatalf("online following = %v, %v", online, err)
	}

	all, err := s.client.ListFollowing(context.Background(), &userv1.ListFollowsRequest{UserId: annID})
	if err != nil || all.GetTotal() != 2 {
		t.Fatalf("following = %v, %v", all, err)
	}

	_, err = s.client.UpdatePrivacy(ann, &userv1.UpdatePrivacyRequest{Privacy: &userv1.PrivacySettings{
		WallWrite: userv1.WallWrite_WALL_WRITE_EVERYONE, MessagesWrite: userv1.MessagesWrite_MESSAGES_WRITE_EVERYONE,
		ClubsRead: userv1.ClubsRead_CLUBS_READ_EVERYONE, PageRead: userv1.Audience_AUDIENCE_EVERYONE,
		InfoRead: userv1.Audience_AUDIENCE_EVERYONE, PhotosRead: userv1.Audience_AUDIENCE_EVERYONE,
		FriendsRead: userv1.Audience_AUDIENCE_EVERYONE,
	}})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("without who sees the absence: %v", err)
	}
}
