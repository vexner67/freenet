package grpc

import (
	"context"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/jwtauth"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"github.com/vexner67/freenet/user/internal/service"
	"github.com/vexner67/freenet/user/internal/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

const maxProfilesPerRequest = 100

type FollowService interface {
	Follow(ctx context.Context, followerID, followeeID uuid.UUID) error
	AcceptInvite(ctx context.Context, inviteeID, inviterID uuid.UUID) error
	Unfollow(ctx context.Context, followerID, followeeID uuid.UUID) error
	Decline(ctx context.Context, ownerID, followerID uuid.UUID) error
	Stats(ctx context.Context, userID, viewerID uuid.UUID) (service.FollowStats, error)
	ListFriends(
		ctx context.Context,
		userID, viewerID uuid.UUID,
		section user.FriendSection,
		cursor uuid.UUID,
		size int,
	) (service.FollowPage, error)
}

func parseID(s, what string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, status.Error(codes.InvalidArgument, "invalid "+what)
	}

	return id, nil
}

func (h *UserHandler) GetProfiles(ctx context.Context, req *userv1.GetProfilesRequest) (*userv1.GetProfilesResponse, error) {
	if len(req.GetUserIds()) > maxProfilesPerRequest {
		return nil, status.Error(codes.InvalidArgument, "too many user ids")
	}

	ids := make([]uuid.UUID, 0, len(req.GetUserIds()))
	for _, s := range req.GetUserIds() {
		id, err := parseID(s, "user id")
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}

	profiles, err := h.userService.GetPublicProfiles(ctx, ids)
	if err != nil {
		return nil, err
	}

	resp := &userv1.GetProfilesResponse{Profiles: make([]*userv1.PublicProfile, 0, len(profiles))}
	for _, p := range profiles {
		resp.Profiles = append(resp.Profiles, toProtoPublicProfile(p))
	}

	return resp, nil
}

func (h *UserHandler) Follow(ctx context.Context, req *userv1.FollowRequest) (*emptypb.Empty, error) {
	return h.changeFollow(ctx, req.GetUserId(), h.followService.Follow)
}

func (h *UserHandler) Unfollow(ctx context.Context, req *userv1.UnfollowRequest) (*emptypb.Empty, error) {
	return h.changeFollow(ctx, req.GetUserId(), h.followService.Unfollow)
}

func (h *UserHandler) AcceptInvite(ctx context.Context, req *userv1.AcceptInviteRequest) (*emptypb.Empty, error) {
	return h.changeFollow(ctx, req.GetUserId(), h.followService.AcceptInvite)
}

// DeclineFriend leaves the user's request among the caller's followers.
func (h *UserHandler) DeclineFriend(ctx context.Context, req *userv1.DeclineFriendRequest) (*emptypb.Empty, error) {
	ownerID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	followerID, err := parseID(req.GetUserId(), "user id")
	if err != nil {
		return nil, err
	}

	if err = h.followService.Decline(ctx, ownerID, followerID); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *UserHandler) changeFollow(
	ctx context.Context,
	target string,
	change func(ctx context.Context, followerID, followeeID uuid.UUID) error,
) (*emptypb.Empty, error) {
	followerID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	followeeID, err := parseID(target, "user id")
	if err != nil {
		return nil, err
	}

	if err = change(ctx, followerID, followeeID); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *UserHandler) GetFollowStats(
	ctx context.Context,
	req *userv1.GetFollowStatsRequest,
) (*userv1.GetFollowStatsResponse, error) {
	userID, err := parseID(req.GetUserId(), "user id")
	if err != nil {
		return nil, err
	}

	// A public method: the viewer is known only if a token was sent.
	var viewerID uuid.UUID
	if identity, ok := jwtauth.IdentityFromContext(ctx); ok {
		viewerID = identity.UserID
	}

	stats, err := h.followService.Stats(ctx, userID, viewerID)
	if err != nil {
		return nil, err
	}

	return &userv1.GetFollowStatsResponse{
		FollowersCount: int32(stats.Followers), //nolint:gosec // counts of people fit in int32
		FollowingCount: int32(stats.Following), //nolint:gosec // counts of people fit in int32
		IsFollowing:    stats.IsFollowing,
		FriendsCount:   int32(stats.Friends),  //nolint:gosec // counts of people fit in int32
		RequestsCount:  int32(stats.Requests), //nolint:gosec // counts of people fit in int32
		FollowsCaller:  stats.FollowsViewer,
	}, nil
}

// ListFriends is a section of openvk's "Друзья"; the viewer is known only
// if a token was sent.
func (h *UserHandler) ListFriends(ctx context.Context, req *userv1.ListFriendsRequest) (*userv1.ListFriendsResponse, error) {
	userID, err := parseID(req.GetUserId(), "user id")
	if err != nil {
		return nil, err
	}

	section, ok := friendSectionFromProto[req.GetSection()]
	if !ok {
		return nil, status.Error(codes.InvalidArgument, user.ErrInvalidFriendSection.Error())
	}

	var viewerID uuid.UUID
	if identity, ok := jwtauth.IdentityFromContext(ctx); ok {
		viewerID = identity.UserID
	}

	if viewerID == uuid.Nil && (section == user.FriendsCommon || section.OwnOnly()) {
		return nil, status.Error(codes.Unauthenticated, "access token required")
	}

	var cursor uuid.UUID
	if req.GetCursor() != "" {
		if cursor, err = parseID(req.GetCursor(), "cursor"); err != nil {
			return nil, err
		}
	}

	page, err := h.followService.ListFriends(ctx, userID, viewerID, section, cursor, int(req.GetLimit()))
	if err != nil {
		return nil, err
	}

	resp := &userv1.ListFriendsResponse{
		Users: make([]*userv1.PublicProfile, 0, len(page.Users)),
		Total: int32(page.Total), //nolint:gosec // counts of people fit in int32
	}

	for _, p := range page.Users {
		resp.Users = append(resp.Users, toProtoPublicProfile(p))
	}

	if page.NextCursor != uuid.Nil {
		resp.NextCursor = page.NextCursor.String()
	}

	return resp, nil
}
