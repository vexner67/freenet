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
	cat, catID := s.member(t, "cat")

	// Ann is friends with bob and cat.
	for _, id := range []string{bobID, catID} {
		if _, err := s.client.Follow(ann, &userv1.FollowRequest{UserId: id}); err != nil {
			t.Fatal(err)
		}
	}
	for _, friend := range []context.Context{bob, cat} {
		if _, err := s.client.Follow(friend, &userv1.FollowRequest{UserId: annID}); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := s.client.MarkOnline(context.Background(), &userv1.MarkOnlineRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("without a token: %v", err)
	}
	if _, err := s.client.MarkOnline(bob, &userv1.MarkOnlineRequest{}); err != nil {
		t.Fatalf("MarkOnline: %v", err)
	}
	// The app writes the visits every few seconds.
	if err := s.presence.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	// Anyone may ask how many people the site has.
	stats, err := s.client.GetStatistics(context.Background(), &userv1.GetStatisticsRequest{})
	if err != nil || stats.GetOnline() != 1 || stats.GetUsers() < 2 {
		t.Fatalf("GetStatistics = %v, %v", stats, err)
	}

	got, err := s.client.GetProfile(context.Background(), &userv1.GetProfileRequest{Nickname: "bob"})
	if p := got.GetProfile(); err != nil || !p.GetOnline() || p.GetLastSeenAt() == nil {
		t.Fatalf("bob: %v, %v", p, err)
	}
	if got, err = s.client.GetProfile(context.Background(), &userv1.GetProfileRequest{Nickname: "cat"}); err != nil ||
		got.GetProfile().GetOnline() || got.GetProfile().GetLastSeenAt() != nil {
		t.Fatalf("cat, never seen: %v, %v", got, err)
	}

	online, err := s.client.ListFriends(context.Background(), &userv1.ListFriendsRequest{UserId: annID, Section: userv1.FriendSection_FRIEND_SECTION_ONLINE})
	if err != nil || online.GetTotal() != 1 || len(online.GetUsers()) != 1 || online.GetUsers()[0].GetId() != bobID {
		t.Fatalf("online friends = %v, %v", online, err)
	}

	all, err := s.client.ListFriends(context.Background(), &userv1.ListFriendsRequest{UserId: annID})
	if err != nil || all.GetTotal() != 2 {
		t.Fatalf("friends = %v, %v", all, err)
	}

	_, err = s.client.UpdatePrivacy(ann, &userv1.UpdatePrivacyRequest{Privacy: &userv1.PrivacySettings{
		WallWrite: userv1.WallWrite_WALL_WRITE_EVERYONE, MessagesWrite: userv1.MessagesWrite_MESSAGES_WRITE_EVERYONE,
		ClubsRead: userv1.ClubsRead_CLUBS_READ_EVERYONE, PageRead: userv1.Audience_AUDIENCE_EVERYONE,
		InfoRead:    userv1.Audience_AUDIENCE_EVERYONE,
		FriendsRead: userv1.Audience_AUDIENCE_EVERYONE,
	}})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("without who sees the absence: %v", err)
	}
}
