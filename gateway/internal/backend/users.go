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

// withToken forwards the caller's access token, if any; the service
// verifies it. Public methods answer anonymous callers without one.
func withToken(ctx context.Context, accessToken string) context.Context {
	if accessToken == "" {
		return ctx
	}

	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+accessToken)
}

func (u *Users) GetMe(ctx context.Context, accessToken string) (model.User, error) {
	resp, err := u.api.GetMe(withToken(ctx, accessToken), &userv1.GetMeRequest{})
	if err != nil {
		return model.User{}, errs.Wrap("u.api.GetMe", fromStatus(err))
	}

	return toUser(resp.GetUser()), nil
}

// UpdateProfile: partner is the nickname of whom the relationship names,
// "" for nobody.
func (u *Users) UpdateProfile(ctx context.Context, accessToken string, p model.Profile, partner string) (model.User, error) {
	req, err := toUpdateProfileRequest(p, partner)
	if err != nil {
		return model.User{}, errs.Wrap("toUpdateProfileRequest", err)
	}

	resp, err := u.api.UpdateProfile(withToken(ctx, accessToken), req)
	if err != nil {
		return model.User{}, errs.Wrap("u.api.UpdateProfile", fromStatus(err))
	}

	return toUser(resp.GetUser()), nil
}

func (u *Users) UpdateContacts(ctx context.Context, accessToken string, c model.Contacts) (model.User, error) {
	resp, err := u.api.UpdateContacts(withToken(ctx, accessToken), &userv1.UpdateContactsRequest{Contacts: toProtoContacts(c)})
	if err != nil {
		return model.User{}, errs.Wrap("u.api.UpdateContacts", fromStatus(err))
	}

	return toUser(resp.GetUser()), nil
}

// UpdateInterface replaces the caller's settings of "Интерфейс".
func (u *Users) UpdateInterface(ctx context.Context, accessToken string, ui model.Interface) (model.User, error) {
	resp, err := u.api.UpdateInterface(withToken(ctx, accessToken), &userv1.UpdateInterfaceRequest{Interface: &userv1.InterfaceSettings{
		AvatarStyle: ui.AvatarStyle, MainPage: ui.MainPage, HiddenMenu: ui.HiddenMenu,
		Theme: ui.Theme, HideRating: ui.HideRating, Nsfw: ui.NSFW, PostView: ui.PostView,
		TimezoneOffset: toTimezoneOffset(ui.Timezone),
	}})
	if err != nil {
		return model.User{}, errs.Wrap("u.api.UpdateInterface", fromStatus(err))
	}

	return toUser(resp.GetUser()), nil
}

// UpdateStatus replaces the caller's status alone.
func (u *Users) UpdateStatus(ctx context.Context, accessToken, status string) (model.User, error) {
	resp, err := u.api.UpdateStatus(withToken(ctx, accessToken), &userv1.UpdateStatusRequest{Status: status})
	if err != nil {
		return model.User{}, errs.Wrap("u.api.UpdateStatus", fromStatus(err))
	}

	return toUser(resp.GetUser()), nil
}

func (u *Users) UpdateInterests(ctx context.Context, accessToken string, in model.Interests) (model.User, error) {
	resp, err := u.api.UpdateInterests(withToken(ctx, accessToken), &userv1.UpdateInterestsRequest{Interests: toProtoInterests(in)})
	if err != nil {
		return model.User{}, errs.Wrap("u.api.UpdateInterests", fromStatus(err))
	}

	return toUser(resp.GetUser()), nil
}

func (u *Users) UpdateExtraFields(ctx context.Context, accessToken string, fields []model.ExtraField) (model.User, error) {
	req, err := toProtoExtraFields(fields)
	if err != nil {
		return model.User{}, errs.Wrap("toProtoExtraFields", err)
	}

	resp, err := u.api.UpdateExtraFields(withToken(ctx, accessToken), &userv1.UpdateExtraFieldsRequest{Fields: req})
	if err != nil {
		return model.User{}, errs.Wrap("u.api.UpdateExtraFields", fromStatus(err))
	}

	return toUser(resp.GetUser()), nil
}

// UpdatePrivacy: an empty WallWrite, MessagesWrite or ClubsRead is
// rejected by the user service.
func (u *Users) UpdatePrivacy(ctx context.Context, accessToken string, p model.Privacy) (model.User, error) {
	wallWrite, err := enumValue(p.WallWrite, wallWritePrefix, "wall write", userv1.WallWrite_value)
	if err != nil {
		return model.User{}, errs.Wrap("enumValue", err)
	}

	messagesWrite, err := enumValue(p.MessagesWrite, messagesWritePrefix, "messages write", userv1.MessagesWrite_value)
	if err != nil {
		return model.User{}, errs.Wrap("enumValue", err)
	}

	clubsRead, err := enumValue(p.ClubsRead, clubsReadPrefix, "clubs read", userv1.ClubsRead_value)
	if err != nil {
		return model.User{}, errs.Wrap("enumValue", err)
	}

	var reads [5]userv1.Audience
	for i, a := range []string{p.PageRead, p.InfoRead, p.PhotosRead, p.FriendsRead, p.OnlineRead} {
		v, err := enumValue(a, audiencePrefix, "audience", userv1.Audience_value)
		if err != nil {
			return model.User{}, errs.Wrap("enumValue", err)
		}
		reads[i] = userv1.Audience(v)
	}

	resp, err := u.api.UpdatePrivacy(withToken(ctx, accessToken), &userv1.UpdatePrivacyRequest{
		Privacy: &userv1.PrivacySettings{
			ShowBirthday:       p.ShowBirthday,
			ShowRelationship:   p.ShowRelationship,
			ShowWorldview:      p.ShowWorldview,
			ShowPoliticalViews: p.ShowPoliticalViews,
			WallWrite:          userv1.WallWrite(wallWrite),
			MessagesWrite:      userv1.MessagesWrite(messagesWrite),
			ClubsRead:          userv1.ClubsRead(clubsRead),
			PageRead:           reads[0],
			InfoRead:           reads[1],
			PhotosRead:         reads[2],
			FriendsRead:        reads[3],
			OnlineRead:         reads[4],
		},
	})
	if err != nil {
		return model.User{}, errs.Wrap("u.api.UpdatePrivacy", fromStatus(err))
	}

	return toUser(resp.GetUser()), nil
}

// GetProfile is the page of nickname as the caller sees it, anonymous
// without accessToken.
func (u *Users) GetProfile(ctx context.Context, accessToken, nickname string) (model.PublicProfile, error) {
	resp, err := u.api.GetProfile(withToken(ctx, accessToken), &userv1.GetProfileRequest{Nickname: nickname})
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

// BlockedIDs are whom the owner of the token blacklisted.
func (u *Users) BlockedIDs(ctx context.Context, accessToken string) ([]string, error) {
	resp, err := u.api.ListBlockedIDs(withToken(ctx, accessToken), &userv1.ListBlockedIDsRequest{})
	if err != nil {
		return nil, errs.Wrap("u.api.ListBlockedIDs", fromStatus(err))
	}

	return resp.GetUserIds(), nil
}

// FollowStats: accessToken may be empty for an anonymous viewer.
func (u *Users) FollowStats(ctx context.Context, accessToken, userID string) (model.FollowStats, error) {
	resp, err := u.api.GetFollowStats(withToken(ctx, accessToken), &userv1.GetFollowStatsRequest{UserId: userID})
	if err != nil {
		return model.FollowStats{}, errs.Wrap("u.api.GetFollowStats", fromStatus(err))
	}

	return model.FollowStats{
		Followers:   int(resp.GetFollowersCount()),
		Following:   int(resp.GetFollowingCount()),
		IsFollowing: resp.GetIsFollowing(),
	}, nil
}

// CanPostOnWall tells whether the caller may write on the wall of userID.
func (u *Users) CanPostOnWall(ctx context.Context, accessToken, userID string) (bool, error) {
	resp, err := u.api.CanPostOnWall(withToken(ctx, accessToken), &userv1.CanPostOnWallRequest{UserId: userID})
	if err != nil {
		return false, errs.Wrap("u.api.CanPostOnWall", fromStatus(err))
	}

	return resp.GetAllowed(), nil
}

// Access is what of userID's page the caller, anonymous without
// accessToken, may see.
func (u *Users) Access(ctx context.Context, accessToken, userID string) (model.Access, error) {
	resp, err := u.api.GetAccess(withToken(ctx, accessToken), &userv1.GetAccessRequest{UserId: userID})
	if err != nil {
		return model.Access{}, errs.Wrap("u.api.GetAccess", fromStatus(err))
	}

	return model.Access{
		Page: resp.GetPage(), Info: resp.GetInfo(), Photos: resp.GetPhotos(), Friends: resp.GetFriends(), Clubs: resp.GetClubs(),
	}, nil
}

// CanMessage tells whether the caller may write to userID; wroteFirst is
// whether userID has written to the caller, from the message service.
func (u *Users) CanMessage(ctx context.Context, accessToken, userID string, wroteFirst bool) (bool, error) {
	resp, err := u.api.CanMessage(withToken(ctx, accessToken), &userv1.CanMessageRequest{UserId: userID, WroteFirst: wroteFirst})
	if err != nil {
		return false, errs.Wrap("u.api.CanMessage", fromStatus(err))
	}

	return resp.GetAllowed(), nil
}

func (u *Users) ResolveMentions(ctx context.Context, accessToken string, nicknames []string) ([]string, error) {
	resp, err := u.api.ResolveMentions(withToken(ctx, accessToken), &userv1.ResolveMentionsRequest{Nicknames: nicknames})
	if err != nil {
		return nil, errs.Wrap("u.api.ResolveMentions", fromStatus(err))
	}

	return resp.GetUserIds(), nil
}

// KnownNicknames are those of nicknames that are someone's.
func (u *Users) KnownNicknames(ctx context.Context, nicknames []string) ([]string, error) {
	resp, err := u.api.FindNicknames(ctx, &userv1.FindNicknamesRequest{Nicknames: nicknames})
	if err != nil {
		return nil, errs.Wrap("u.api.FindNicknames", fromStatus(err))
	}

	return resp.GetNicknames(), nil
}

func (u *Users) Block(ctx context.Context, accessToken, userID string) error {
	if _, err := u.api.Block(withToken(ctx, accessToken), &userv1.BlockRequest{UserId: userID}); err != nil {
		return errs.Wrap("u.api.Block", fromStatus(err))
	}

	return nil
}

func (u *Users) Unblock(ctx context.Context, accessToken, userID string) error {
	if _, err := u.api.Unblock(withToken(ctx, accessToken), &userv1.UnblockRequest{UserId: userID}); err != nil {
		return errs.Wrap("u.api.Unblock", fromStatus(err))
	}

	return nil
}

// Blacklist is the caller's blacklist, newest block first.
func (u *Users) Blacklist(ctx context.Context, accessToken, cursor string, limit int) (model.UserPage, error) {
	resp, err := u.api.ListBlocked(withToken(ctx, accessToken), &userv1.ListBlockedRequest{Cursor: cursor, Limit: toLimit(limit)})
	if err != nil {
		return model.UserPage{}, errs.Wrap("u.api.ListBlocked", fromStatus(err))
	}

	return toUserPage(resp), nil
}

func (u *Users) Relation(ctx context.Context, accessToken, userID string) (model.Relation, error) {
	resp, err := u.api.GetRelation(withToken(ctx, accessToken), &userv1.GetRelationRequest{UserId: userID})
	if err != nil {
		return model.Relation{}, errs.Wrap("u.api.GetRelation", fromStatus(err))
	}

	return model.Relation{BlockedByMe: resp.GetBlockedByMe(), BlockedMe: resp.GetBlockedMe()}, nil
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

// ListOnlineFollowing is openvk's "Друзья онлайн".
func (u *Users) ListOnlineFollowing(ctx context.Context, userID, cursor string, limit int) (model.UserPage, error) {
	req := listFollowsRequest(userID, cursor, limit)
	req.Online = true

	resp, err := u.api.ListFollowing(ctx, req)
	if err != nil {
		return model.UserPage{}, errs.Wrap("u.api.ListFollowing", fromStatus(err))
	}

	return toUserPage(resp), nil
}

// ListCommonFollowing is whom both userID and the caller follow, openvk's
// "Общие друзья".
func (u *Users) ListCommonFollowing(ctx context.Context, accessToken, userID, cursor string, limit int) (model.UserPage, error) {
	req := listFollowsRequest(userID, cursor, limit)
	req.Common = true

	resp, err := u.api.ListFollowing(withToken(ctx, accessToken), req)
	if err != nil {
		return model.UserPage{}, errs.Wrap("u.api.ListFollowing", fromStatus(err))
	}

	return toUserPage(resp), nil
}

// Statistics counts the people of the site and those on it now.
func (u *Users) Statistics(ctx context.Context) (int, int, error) {
	resp, err := u.api.GetStatistics(ctx, &userv1.GetStatisticsRequest{})
	if err != nil {
		return 0, 0, errs.Wrap("u.api.GetStatistics", fromStatus(err))
	}

	return int(resp.GetUsers()), int(resp.GetOnline()), nil
}

// MarkOnline tells that the caller is on the site now ("Онлайн").
func (u *Users) MarkOnline(ctx context.Context, accessToken string) error {
	if _, err := u.api.MarkOnline(withToken(ctx, accessToken), &userv1.MarkOnlineRequest{}); err != nil {
		return errs.Wrap("u.api.MarkOnline", fromStatus(err))
	}

	return nil
}

func listFollowsRequest(userID, cursor string, limit int) *userv1.ListFollowsRequest {
	return &userv1.ListFollowsRequest{
		UserId: userID,
		Cursor: cursor,
		Limit:  int32(min(max(limit, 0), 1<<10)), //nolint:gosec // clamped above
	}
}

// userPageMessage is implemented by the responses that list people.
type userPageMessage interface {
	GetUsers() []*userv1.PublicProfile
	GetNextCursor() string
	GetTotal() int32
}

func toUserPage(resp userPageMessage) model.UserPage {
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
		ID:          p.GetId(),
		AvatarID:    p.GetAvatarId(),
		Profile:     toProfile(p),
		Deactivated: p.GetDeactivated(),

		BirthYearHidden: p.GetBirthYearHidden(),
		PageHidden:      p.GetPageHidden(),
		InfoHidden:      p.GetInfoHidden(),
		LastSeen:        fromTimestamp(p.GetLastSeenAt()),
		Online:          p.GetOnline(),
		Backdrop:        model.Backdrop{Left: p.GetBackdropLeft(), Right: p.GetBackdropRight()},
		Completeness:    int(p.GetCompleteness()),
		About:           toAbout(p),
	}
}

// Deactivate deletes the caller's page; reason may be empty.
func (u *Users) Deactivate(ctx context.Context, accessToken, reason string) (model.User, error) {
	resp, err := u.api.DeactivateAccount(withToken(ctx, accessToken), &userv1.DeactivateAccountRequest{Reason: reason})
	if err != nil {
		return model.User{}, errs.Wrap("u.api.DeactivateAccount", fromStatus(err))
	}

	return toUser(resp.GetUser()), nil
}

// Reactivate restores the caller's deleted page.
func (u *Users) Reactivate(ctx context.Context, accessToken string) (model.User, error) {
	resp, err := u.api.ReactivateAccount(withToken(ctx, accessToken), &userv1.ReactivateAccountRequest{})
	if err != nil {
		return model.User{}, errs.Wrap("u.api.ReactivateAccount", fromStatus(err))
	}

	return toUser(resp.GetUser()), nil
}

// SetAvatar points the caller's profile to an uploaded avatar; "" removes it.
func (u *Users) SetAvatar(ctx context.Context, accessToken, avatarID string) (model.User, error) {
	resp, err := u.api.SetAvatar(withToken(ctx, accessToken), &userv1.SetAvatarRequest{AvatarId: avatarID})
	if err != nil {
		return model.User{}, errs.Wrap("u.api.SetAvatar", fromStatus(err))
	}

	return toUser(resp.GetUser()), nil
}

// SetBackdrop shows photoID ("" for none) on side ("left" or "right") of
// the caller's page; previous is the photo the side showed before.
func (u *Users) SetBackdrop(ctx context.Context, accessToken, side, photoID string) (user model.User, previous string, err error) {
	resp, err := u.api.SetBackdrop(withToken(ctx, accessToken), &userv1.SetBackdropRequest{
		Side:    userBackdropSides[side],
		PhotoId: photoID,
	})
	if err != nil {
		return model.User{}, "", errs.Wrap("u.api.SetBackdrop", fromStatus(err))
	}

	return toUser(resp.GetUser()), resp.GetPreviousPhotoId(), nil
}

var userBackdropSides = map[string]userv1.BackdropSide{
	"left":  userv1.BackdropSide_BACKDROP_SIDE_LEFT,
	"right": userv1.BackdropSide_BACKDROP_SIDE_RIGHT,
}

func (u *Users) Search(ctx context.Context, q model.UserQuery, cursor string, limit int) (model.UserPage, error) {
	resp, err := u.api.SearchUsers(ctx, &userv1.SearchUsersRequest{
		Query:    q.Text,
		Hometown: q.Hometown,
		City:     q.City,
		Film:     q.Film,
		Show:     q.Show,
		Book:     q.Book,
		Cursor:   cursor,
		Limit:    int32(min(max(limit, 0), 1<<10)), //nolint:gosec // clamped above
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
