package grpc_test

import (
	"context"
	"testing"

	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// member signs in and fills in a profile, returning their context and ID.
func (s testServer) member(t *testing.T, nickname string) (context.Context, string) {
	t.Helper()

	ctx := s.signIn(t, nickname+"@example.com")

	resp, err := s.client.UpdateProfile(ctx, &userv1.UpdateProfileRequest{Name: nickname, Nickname: nickname})
	if err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}

	return ctx, resp.GetUser().GetId()
}

func TestFollows(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, annID := s.member(t, "ann")
	bob, bobID := s.member(t, "bob")
	cat, catID := s.member(t, "cat")

	for _, follower := range []context.Context{bob, cat, bob} { // bob twice: idempotent
		if _, err := s.client.Follow(follower, &userv1.FollowRequest{UserId: annID}); err != nil {
			t.Fatalf("Follow: %v", err)
		}
	}
	if _, err := s.client.Follow(ann, &userv1.FollowRequest{UserId: catID}); err != nil {
		t.Fatal(err)
	}

	stats, err := s.client.GetFollowStats(bob, &userv1.GetFollowStatsRequest{UserId: annID})
	if err != nil {
		t.Fatal(err)
	}
	if stats.GetFollowersCount() != 2 || stats.GetFollowingCount() != 1 || !stats.GetIsFollowing() {
		t.Fatalf("stats seen by bob: %v", stats)
	}

	anonymous, err := s.client.GetFollowStats(context.Background(), &userv1.GetFollowStatsRequest{UserId: annID})
	if err != nil || anonymous.GetIsFollowing() || anonymous.GetFollowersCount() != 2 {
		t.Fatalf("anonymous stats: %v, %v", anonymous, err)
	}

	page, err := s.client.ListFollowers(context.Background(), &userv1.ListFollowsRequest{UserId: annID, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if page.GetTotal() != 2 || len(page.GetUsers()) != 1 || page.GetUsers()[0].GetNickname() != "cat" || page.GetNextCursor() == "" {
		t.Fatalf("first page of followers (newest first): %v", page)
	}

	last, err := s.client.ListFollowers(context.Background(), &userv1.ListFollowsRequest{UserId: annID, Limit: 1, Cursor: page.GetNextCursor()})
	if err != nil || len(last.GetUsers()) != 1 || last.GetUsers()[0].GetNickname() != "bob" || last.GetNextCursor() != "" {
		t.Fatalf("last page: %v, %v", last, err)
	}

	ids, err := s.client.ListFollowingIDs(ann, &userv1.ListFollowingIDsRequest{})
	if err != nil || len(ids.GetUserIds()) != 1 || ids.GetUserIds()[0] != catID {
		t.Fatalf("ann follows: %v, %v", ids, err)
	}

	if _, err = s.client.Unfollow(bob, &userv1.UnfollowRequest{UserId: annID}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.client.Unfollow(bob, &userv1.UnfollowRequest{UserId: annID}); err != nil {
		t.Fatalf("unfollow twice: %v", err)
	}

	stats, err = s.client.GetFollowStats(bob, &userv1.GetFollowStatsRequest{UserId: annID})
	if err != nil || stats.GetIsFollowing() || stats.GetFollowersCount() != 1 {
		t.Fatalf("after unfollow: %v, %v", stats, err)
	}

	profiles, err := s.client.GetProfiles(context.Background(), &userv1.GetProfilesRequest{UserIds: []string{annID, bobID, "01a10000-0000-7000-8000-000000000000"}})
	if err != nil || len(profiles.GetProfiles()) != 2 {
		t.Fatalf("GetProfiles: %v, %v", profiles, err)
	}
}

func TestFollowErrors(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, annID := s.member(t, "ann")

	tests := []struct {
		name string
		call func() error
		want codes.Code
	}{
		{"follow yourself", func() error {
			_, err := s.client.Follow(ann, &userv1.FollowRequest{UserId: annID})
			return err
		}, codes.InvalidArgument},
		{"follow nobody", func() error {
			_, err := s.client.Follow(ann, &userv1.FollowRequest{UserId: "01a10000-0000-7000-8000-000000000000"})
			return err
		}, codes.NotFound},
		{"follow without token", func() error {
			_, err := s.client.Follow(context.Background(), &userv1.FollowRequest{UserId: annID})
			return err
		}, codes.Unauthenticated},
		{"following ids without token", func() error {
			_, err := s.client.ListFollowingIDs(context.Background(), &userv1.ListFollowingIDsRequest{})
			return err
		}, codes.Unauthenticated},
		{"bad user id", func() error {
			_, err := s.client.GetFollowStats(context.Background(), &userv1.GetFollowStatsRequest{UserId: "ann"})
			return err
		}, codes.InvalidArgument},
		{"bad cursor", func() error {
			_, err := s.client.ListFollowing(context.Background(), &userv1.ListFollowsRequest{UserId: annID, Cursor: "x"})
			return err
		}, codes.InvalidArgument},
		{"too many profiles", func() error {
			ids := make([]string, 101)
			for i := range ids {
				ids[i] = annID
			}
			_, err := s.client.GetProfiles(context.Background(), &userv1.GetProfilesRequest{UserIds: ids})
			return err
		}, codes.InvalidArgument},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := status.Code(tt.call()); got != tt.want {
				t.Fatalf("code = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCanPostOnWall(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, annID := s.member(t, "ann")
	bob, bobID := s.member(t, "bob")

	canPost := func(ctx context.Context, ownerID string) bool {
		t.Helper()

		resp, err := s.client.CanPostOnWall(ctx, &userv1.CanPostOnWallRequest{UserId: ownerID})
		if err != nil {
			t.Fatalf("CanPostOnWall: %v", err)
		}

		return resp.GetAllowed()
	}

	me, err := s.client.GetMe(ann, &userv1.GetMeRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if got := me.GetUser().GetPrivacy().GetWallWrite(); got != userv1.WallWrite_WALL_WRITE_FOLLOWING {
		t.Fatalf("default wall_write = %v, want FOLLOWING", got)
	}

	if !canPost(ann, annID) || canPost(bob, annID) {
		t.Fatal("a new wall is open to its owner and to the people the owner follows")
	}

	if _, err = s.client.Follow(ann, &userv1.FollowRequest{UserId: bobID}); err != nil {
		t.Fatal(err)
	}
	if !canPost(bob, annID) {
		t.Fatal("ann follows bob, so he may post on her wall")
	}

	_, err = s.client.CanPostOnWall(context.Background(), &userv1.CanPostOnWallRequest{UserId: annID})
	if got := status.Code(err); got != codes.Unauthenticated {
		t.Fatalf("without token: code = %v", got)
	}

	_, err = s.client.CanPostOnWall(bob, &userv1.CanPostOnWallRequest{UserId: "nope"})
	if got := status.Code(err); got != codes.InvalidArgument {
		t.Fatalf("bad id: code = %v", got)
	}

	_, err = s.client.CanPostOnWall(bob, &userv1.CanPostOnWallRequest{UserId: "0192f0a0-0000-7000-8000-000000000000"})
	if got := status.Code(err); got != codes.NotFound {
		t.Fatalf("unknown owner: code = %v", got)
	}
}

func TestBlacklist(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, annID := s.member(t, "ann")
	bob, bobID := s.member(t, "bob")

	if _, err := s.client.Follow(bob, &userv1.FollowRequest{UserId: annID}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.client.Block(ann, &userv1.BlockRequest{UserId: bobID}); err != nil {
		t.Fatalf("Block: %v", err)
	}

	_, err := s.client.Follow(bob, &userv1.FollowRequest{UserId: annID})
	if got := status.Code(err); got != codes.PermissionDenied {
		t.Fatalf("follow across a block: code = %v", got)
	}

	stats, err := s.client.GetFollowStats(bob, &userv1.GetFollowStatsRequest{UserId: annID})
	if err != nil || stats.GetFollowersCount() != 0 || stats.GetIsFollowing() {
		t.Fatalf("bob's follow must be dropped: %v, %v", stats, err)
	}

	rel, err := s.client.GetRelation(bob, &userv1.GetRelationRequest{UserId: annID})
	if err != nil || rel.GetBlockedByMe() || !rel.GetBlockedMe() {
		t.Fatalf("bob's relation to ann = %v, %v", rel, err)
	}

	msg, err := s.client.CanMessage(bob, &userv1.CanMessageRequest{UserId: annID, WroteFirst: true})
	if err != nil || msg.GetAllowed() {
		t.Fatalf("bob may not write to ann: %v, %v", msg, err)
	}

	list, err := s.client.ListBlocked(ann, &userv1.ListBlockedRequest{})
	if err != nil || list.GetTotal() != 1 || list.GetUsers()[0].GetNickname() != "bob" {
		t.Fatalf("ann's blacklist = %v, %v", list, err)
	}

	if _, err = s.client.Unblock(ann, &userv1.UnblockRequest{UserId: bobID}); err != nil {
		t.Fatal(err)
	}
	if msg, err = s.client.CanMessage(bob, &userv1.CanMessageRequest{UserId: annID}); err != nil || !msg.GetAllowed() {
		t.Fatalf("after unblocking: %v, %v", msg, err)
	}

	tests := []struct {
		name string
		call func() error
		code codes.Code
	}{
		{"block yourself", func() error {
			_, err := s.client.Block(ann, &userv1.BlockRequest{UserId: annID})
			return err
		}, codes.InvalidArgument},
		{"block nobody", func() error {
			_, err := s.client.Block(ann, &userv1.BlockRequest{UserId: "0192f0a0-0000-7000-8000-000000000000"})
			return err
		}, codes.NotFound},
		{"bad id", func() error {
			_, err := s.client.GetRelation(ann, &userv1.GetRelationRequest{UserId: "x"})
			return err
		}, codes.InvalidArgument},
		{"without a token", func() error {
			_, err := s.client.ListBlocked(context.Background(), &userv1.ListBlockedRequest{})
			return err
		}, codes.Unauthenticated},
	}

	for _, tt := range tests {
		if got := status.Code(tt.call()); got != tt.code {
			t.Fatalf("%s: code = %v, want %v", tt.name, got, tt.code)
		}
	}
}
