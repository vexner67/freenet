package backend

import (
	"context"

	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/pkg/errs"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc/metadata"
)

type Users struct {
	api userv1.UserServiceClient
}

func NewUsers(api userv1.UserServiceClient) *Users {
	return &Users{
		api: api,
	}
}

// withToken forwards the caller's access token; the user service verifies it.
func withToken(ctx context.Context, accessToken string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+accessToken)
}

func (u *Users) GetMe(ctx context.Context, accessToken string) (model.User, error) {
	resp, err := u.api.GetMe(withToken(ctx, accessToken), &userv1.GetMeRequest{})
	if err != nil {
		return model.User{}, errs.Wrap("u.api.GetMe", fromStatus(err))
	}

	return toUser(resp.GetUser()), nil
}

func (u *Users) UpdateProfile(ctx context.Context, accessToken string, p model.Profile) (model.User, error) {
	req, err := toUpdateProfileRequest(p)
	if err != nil {
		return model.User{}, errs.Wrap("toUpdateProfileRequest", err)
	}

	resp, err := u.api.UpdateProfile(withToken(ctx, accessToken), req)
	if err != nil {
		return model.User{}, errs.Wrap("u.api.UpdateProfile", fromStatus(err))
	}

	return toUser(resp.GetUser()), nil
}

func (u *Users) UpdatePrivacy(ctx context.Context, accessToken string, p model.Privacy) (model.User, error) {
	resp, err := u.api.UpdatePrivacy(withToken(ctx, accessToken), &userv1.UpdatePrivacyRequest{
		Privacy: &userv1.PrivacySettings{
			ShowBirthday:       p.ShowBirthday,
			ShowRelationship:   p.ShowRelationship,
			ShowWorldview:      p.ShowWorldview,
			ShowPoliticalViews: p.ShowPoliticalViews,
		},
	})
	if err != nil {
		return model.User{}, errs.Wrap("u.api.UpdatePrivacy", fromStatus(err))
	}

	return toUser(resp.GetUser()), nil
}

func (u *Users) GetProfile(ctx context.Context, nickname string) (model.PublicProfile, error) {
	resp, err := u.api.GetProfile(ctx, &userv1.GetProfileRequest{Nickname: nickname})
	if err != nil {
		return model.PublicProfile{}, errs.Wrap("u.api.GetProfile", fromStatus(err))
	}

	return toPublicProfile(resp.GetProfile()), nil
}

func (u *Users) GetProfiles(ctx context.Context, ids []string) ([]model.PublicProfile, error) {
	resp, err := u.api.GetProfiles(ctx, &userv1.GetProfilesRequest{UserIds: ids})
	if err != nil {
		return nil, errs.Wrap("u.api.GetProfiles", fromStatus(err))
	}

	profiles := make([]model.PublicProfile, 0, len(resp.GetProfiles()))
	for _, p := range resp.GetProfiles() {
		profiles = append(profiles, toPublicProfile(p))
	}

	return profiles, nil
}

func (u *Users) Follow(ctx context.Context, accessToken, userID string) error {
	if _, err := u.api.Follow(withToken(ctx, accessToken), &userv1.FollowRequest{UserId: userID}); err != nil {
		return errs.Wrap("u.api.Follow", fromStatus(err))
	}

	return nil
}

func (u *Users) Unfollow(ctx context.Context, accessToken, userID string) error {
	if _, err := u.api.Unfollow(withToken(ctx, accessToken), &userv1.UnfollowRequest{UserId: userID}); err != nil {
		return errs.Wrap("u.api.Unfollow", fromStatus(err))
	}

	return nil
}

func (u *Users) FollowingIDs(ctx context.Context, accessToken string) ([]string, error) {
	resp, err := u.api.ListFollowingIDs(withToken(ctx, accessToken), &userv1.ListFollowingIDsRequest{})
	if err != nil {
		return nil, errs.Wrap("u.api.ListFollowingIDs", fromStatus(err))
	}

	return resp.GetUserIds(), nil
}

// FollowStats: accessToken may be empty for an anonymous viewer.
func (u *Users) FollowStats(ctx context.Context, accessToken, userID string) (model.FollowStats, error) {
	if accessToken != "" {
		ctx = withToken(ctx, accessToken)
	}

	resp, err := u.api.GetFollowStats(ctx, &userv1.GetFollowStatsRequest{UserId: userID})
	if err != nil {
		return model.FollowStats{}, errs.Wrap("u.api.GetFollowStats", fromStatus(err))
	}

	return model.FollowStats{
		Followers:   int(resp.GetFollowersCount()),
		Following:   int(resp.GetFollowingCount()),
		IsFollowing: resp.GetIsFollowing(),
	}, nil
}

func (u *Users) ListFollowers(ctx context.Context, userID, cursor string, limit int) (model.UserPage, error) {
	resp, err := u.api.ListFollowers(ctx, listFollowsRequest(userID, cursor, limit))
	if err != nil {
		return model.UserPage{}, errs.Wrap("u.api.ListFollowers", fromStatus(err))
	}

	return toUserPage(resp), nil
}

func (u *Users) ListFollowing(ctx context.Context, userID, cursor string, limit int) (model.UserPage, error) {
	resp, err := u.api.ListFollowing(ctx, listFollowsRequest(userID, cursor, limit))
	if err != nil {
		return model.UserPage{}, errs.Wrap("u.api.ListFollowing", fromStatus(err))
	}

	return toUserPage(resp), nil
}

func listFollowsRequest(userID, cursor string, limit int) *userv1.ListFollowsRequest {
	return &userv1.ListFollowsRequest{
		UserId: userID,
		Cursor: cursor,
		Limit:  int32(min(max(limit, 0), 1<<10)), //nolint:gosec // clamped above
	}
}

func toUserPage(resp *userv1.ListFollowsResponse) model.UserPage {
	page := model.UserPage{
		Users:      make([]model.PublicProfile, 0, len(resp.GetUsers())),
		NextCursor: resp.GetNextCursor(),
		Total:      int(resp.GetTotal()),
	}

	for _, p := range resp.GetUsers() {
		page.Users = append(page.Users, toPublicProfile(p))
	}

	return page
}

func toPublicProfile(p *userv1.PublicProfile) model.PublicProfile {
	return model.PublicProfile{
		ID:       p.GetId(),
		AvatarID: p.GetAvatarId(),
		Profile:  toProfile(p),
	}
}

// SetAvatar points the caller's profile to an uploaded avatar; "" removes it.
func (u *Users) SetAvatar(ctx context.Context, accessToken, avatarID string) (model.User, error) {
	resp, err := u.api.SetAvatar(withToken(ctx, accessToken), &userv1.SetAvatarRequest{AvatarId: avatarID})
	if err != nil {
		return model.User{}, errs.Wrap("u.api.SetAvatar", fromStatus(err))
	}

	return toUser(resp.GetUser()), nil
}

func (u *Users) Search(ctx context.Context, query, cursor string, limit int) (model.UserPage, error) {
	resp, err := u.api.SearchUsers(ctx, &userv1.SearchUsersRequest{
		Query:  query,
		Cursor: cursor,
		Limit:  int32(min(max(limit, 0), 1<<10)), //nolint:gosec // clamped above
	})
	if err != nil {
		return model.UserPage{}, errs.Wrap("u.api.SearchUsers", fromStatus(err))
	}

	page := model.UserPage{
		Users:      make([]model.PublicProfile, 0, len(resp.GetUsers())),
		NextCursor: resp.GetNextCursor(),
	}

	for _, p := range resp.GetUsers() {
		page.Users = append(page.Users, toPublicProfile(p))
	}

	return page, nil
}
