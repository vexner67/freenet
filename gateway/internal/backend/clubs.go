package backend

import (
	"context"

	clubv1 "github.com/vexner67/freenet/club/api/club/v1"
	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/pkg/errs"
)

// Clubs talks to the club service. Methods that read a club take an
// optional access token: with it, the club tells what the viewer is in it.
type Clubs struct {
	api clubv1.ClubServiceClient
}

func NewClubs(api clubv1.ClubServiceClient) *Clubs {
	return &Clubs{
		api: api,
	}
}

var (
	walls = map[clubv1.Wall]string{
		clubv1.Wall_WALL_OPEN:    "open",
		clubv1.Wall_WALL_CLOSED:  "closed",
		clubv1.Wall_WALL_SUGGEST: "suggest",
	}
	adminsDisplays = map[clubv1.AdminsDisplay]string{
		clubv1.AdminsDisplay_ADMINS_DISPLAY_CREATOR: "creator",
		clubv1.AdminsDisplay_ADMINS_DISPLAY_ALL:     "all",
		clubv1.AdminsDisplay_ADMINS_DISPLAY_NONE:    "none",
	}
	roles = map[clubv1.Role]string{
		clubv1.Role_ROLE_NONE:   "",
		clubv1.Role_ROLE_MEMBER: "member",
		clubv1.Role_ROLE_ADMIN:  "admin",
		clubv1.Role_ROLE_OWNER:  "owner",
	}
)

func (c *Clubs) Create(ctx context.Context, accessToken, name, description string) (model.ClubView, error) {
	resp, err := c.api.CreateClub(withToken(ctx, accessToken), &clubv1.CreateClubRequest{Name: name, Description: description})
	if err != nil {
		return model.ClubView{}, errs.Wrap("c.api.CreateClub", fromStatus(err))
	}

	return toClubView(resp.GetClub()), nil
}

// Get is the club ref names: its ID or its address.
func (c *Clubs) Get(ctx context.Context, accessToken, ref string) (model.ClubView, error) {
	resp, err := c.api.GetClub(withToken(ctx, accessToken), &clubv1.GetClubRequest{Ref: ref})
	if err != nil {
		return model.ClubView{}, errs.Wrap("c.api.GetClub", fromStatus(err))
	}

	return toClubView(resp.GetClub()), nil
}

// GetMany returns the clubs found among ids, at most 100, in no order.
func (c *Clubs) GetMany(ctx context.Context, ids []string) ([]model.Club, error) {
	resp, err := c.api.GetClubs(ctx, &clubv1.GetClubsRequest{Ids: ids})
	if err != nil {
		return nil, errs.Wrap("c.api.GetClubs", fromStatus(err))
	}

	return toClubs(resp.GetClubs()), nil
}

func (c *Clubs) Update(ctx context.Context, accessToken, clubID string, changes model.ClubChanges) (model.ClubView, error) {
	var wall clubv1.Wall
	for pw, w := range walls {
		if w == changes.Wall {
			wall = pw
		}
	}

	if wall == clubv1.Wall_WALL_UNSPECIFIED {
		return model.ClubView{}, failure.New(failure.InvalidArgument, "invalid wall mode")
	}

	var adminsDisplay clubv1.AdminsDisplay
	for pd, d := range adminsDisplays {
		if d == changes.AdminsDisplay {
			adminsDisplay = pd
		}
	}

	if adminsDisplay == clubv1.AdminsDisplay_ADMINS_DISPLAY_UNSPECIFIED {
		return model.ClubView{}, failure.New(failure.InvalidArgument, "invalid admins display")
	}

	resp, err := c.api.UpdateClub(withToken(ctx, accessToken), &clubv1.UpdateClubRequest{
		ClubId:        clubID,
		Name:          changes.Name,
		Description:   changes.Description,
		Address:       changes.Address,
		Website:       changes.Website,
		Wall:          wall,
		AdminsDisplay: adminsDisplay,

		OpenTopics:      changes.OpenTopics,
		TopicsAboveWall: changes.TopicsAboveWall,
	})
	if err != nil {
		return model.ClubView{}, errs.Wrap("c.api.UpdateClub", fromStatus(err))
	}

	return toClubView(resp.GetClub()), nil
}

func (c *Clubs) Join(ctx context.Context, accessToken, clubID string) (model.ClubView, error) {
	resp, err := c.api.JoinClub(withToken(ctx, accessToken), &clubv1.JoinClubRequest{ClubId: clubID})
	if err != nil {
		return model.ClubView{}, errs.Wrap("c.api.JoinClub", fromStatus(err))
	}

	return toClubView(resp.GetClub()), nil
}

func (c *Clubs) Leave(ctx context.Context, accessToken, clubID string) (model.ClubView, error) {
	resp, err := c.api.LeaveClub(withToken(ctx, accessToken), &clubv1.LeaveClubRequest{ClubId: clubID})
	if err != nil {
		return model.ClubView{}, errs.Wrap("c.api.LeaveClub", fromStatus(err))
	}

	return toClubView(resp.GetClub()), nil
}

// Members pages the members of a club; only the owner and the admins with
// adminsOnly.
func (c *Clubs) Members(ctx context.Context, clubID, cursor string, limit int, adminsOnly bool) (model.ClubMemberPage, error) {
	resp, err := c.api.ListMembers(ctx, &clubv1.ListMembersRequest{
		ClubId:     clubID,
		Cursor:     cursor,
		Limit:      toLimit(limit),
		AdminsOnly: adminsOnly,
	})
	if err != nil {
		return model.ClubMemberPage{}, errs.Wrap("c.api.ListMembers", fromStatus(err))
	}

	page := model.ClubMemberPage{
		Members:    make([]model.ClubMember, 0, len(resp.GetMembers())),
		NextCursor: resp.GetNextCursor(),
		Total:      int(resp.GetTotal()),
	}
	for _, m := range resp.GetMembers() {
		page.Members = append(page.Members, model.ClubMember{UserID: m.GetUserId(), Role: roles[m.GetRole()]})
	}

	return page, nil
}

// SetAdmin makes a member of the club an admin, or an admin a member.
func (c *Clubs) SetAdmin(ctx context.Context, accessToken, clubID, userID string, admin bool) error {
	role := clubv1.Role_ROLE_MEMBER
	if admin {
		role = clubv1.Role_ROLE_ADMIN
	}

	_, err := c.api.SetMemberRole(withToken(ctx, accessToken), &clubv1.SetMemberRoleRequest{ClubId: clubID, UserId: userID, Role: role})
	if err != nil {
		return errs.Wrap("c.api.SetMemberRole", fromStatus(err))
	}

	return nil
}

// Transfer hands the club over to one of its admins.
func (c *Clubs) Transfer(ctx context.Context, accessToken, clubID, userID string) error {
	_, err := c.api.TransferClub(withToken(ctx, accessToken), &clubv1.TransferClubRequest{ClubId: clubID, UserId: userID})
	if err != nil {
		return errs.Wrap("c.api.TransferClub", fromStatus(err))
	}

	return nil
}

// OfUser pages the clubs of a user; whether the viewer may see them is
// for the caller to check.
func (c *Clubs) OfUser(ctx context.Context, userID, cursor string, limit int) (model.UserClubPage, error) {
	resp, err := c.api.ListUserClubs(ctx, &clubv1.ListUserClubsRequest{UserId: userID, Cursor: cursor, Limit: toLimit(limit)})
	if err != nil {
		return model.UserClubPage{}, errs.Wrap("c.api.ListUserClubs", fromStatus(err))
	}

	return model.UserClubPage{Clubs: toClubs(resp.GetClubs()), NextCursor: resp.GetNextCursor(), Total: int(resp.GetTotal())}, nil
}

// Mine pages the caller's clubs; with managed only those they run.
func (c *Clubs) Mine(ctx context.Context, accessToken, cursor string, limit int, managed bool) (model.ClubPage, error) {
	resp, err := c.api.ListMyClubs(withToken(ctx, accessToken), &clubv1.ListMyClubsRequest{Cursor: cursor, Limit: toLimit(limit), Managed: managed})
	if err != nil {
		return model.ClubPage{}, errs.Wrap("c.api.ListMyClubs", fromStatus(err))
	}

	page := model.ClubPage{
		Clubs:      make([]model.Membership, 0, len(resp.GetClubs())),
		NextCursor: resp.GetNextCursor(),
		Total:      int(resp.GetTotal()),
	}
	for _, m := range resp.GetClubs() {
		page.Clubs = append(page.Clubs, model.Membership{Club: toClub(m.GetClub()), Role: roles[m.GetRole()], Pinned: m.GetPinned()})
	}

	return page, nil
}

// Search pages the clubs whose name or address has every word of query.
func (c *Clubs) Search(ctx context.Context, query, cursor string, limit int) (model.ClubSearchPage, error) {
	resp, err := c.api.SearchClubs(ctx, &clubv1.SearchClubsRequest{Query: query, Cursor: cursor, Limit: toLimit(limit)})
	if err != nil {
		return model.ClubSearchPage{}, errs.Wrap("c.api.SearchClubs", fromStatus(err))
	}

	return model.ClubSearchPage{Clubs: toClubs(resp.GetClubs()), NextCursor: resp.GetNextCursor()}, nil
}

// SetAvatar shows a photo stored in media as the club's; "" takes it away.
func (c *Clubs) SetAvatar(ctx context.Context, accessToken, clubID, avatarID string) (model.ClubView, error) {
	resp, err := c.api.SetClubAvatar(withToken(ctx, accessToken), &clubv1.SetClubAvatarRequest{ClubId: clubID, AvatarId: avatarID})
	if err != nil {
		return model.ClubView{}, errs.Wrap("c.api.SetClubAvatar", fromStatus(err))
	}

	return toClubView(resp.GetClub()), nil
}

var backdropSides = map[string]clubv1.BackdropSide{
	"left":  clubv1.BackdropSide_BACKDROP_SIDE_LEFT,
	"right": clubv1.BackdropSide_BACKDROP_SIDE_RIGHT,
}

// SetBackdrop shows photoID ("" for none) on side, "left" or "right", of
// the club's page.
func (c *Clubs) SetBackdrop(ctx context.Context, accessToken, clubID, side, photoID string) (model.ClubView, error) {
	ps, ok := backdropSides[side]
	if !ok {
		return model.ClubView{}, failure.New(failure.InvalidArgument, "invalid backdrop side")
	}

	resp, err := c.api.SetClubBackdrop(withToken(ctx, accessToken), &clubv1.SetClubBackdropRequest{ClubId: clubID, Side: ps, PhotoId: photoID})
	if err != nil {
		return model.ClubView{}, errs.Wrap("c.api.SetClubBackdrop", fromStatus(err))
	}

	return toClubView(resp.GetClub()), nil
}

// Pin puts a club the caller runs in their left menu, or takes it out.
func (c *Clubs) Pin(ctx context.Context, accessToken, clubID string, pinned bool) error {
	if _, err := c.api.PinClub(withToken(ctx, accessToken), &clubv1.PinClubRequest{ClubId: clubID, Pinned: pinned}); err != nil {
		return errs.Wrap("c.api.PinClub", fromStatus(err))
	}

	return nil
}

// Pinned are the clubs in the caller's left menu.
func (c *Clubs) Pinned(ctx context.Context, accessToken string) ([]model.Club, error) {
	resp, err := c.api.ListPinnedClubs(withToken(ctx, accessToken), &clubv1.ListPinnedClubsRequest{})
	if err != nil {
		return nil, errs.Wrap("c.api.ListPinnedClubs", fromStatus(err))
	}

	return toClubs(resp.GetClubs()), nil
}

func toClubs(clubs []*clubv1.Club) []model.Club {
	out := make([]model.Club, 0, len(clubs))
	for _, pc := range clubs {
		out = append(out, toClub(pc))
	}

	return out
}

// MineIDs are the caller's newest 1000 clubs, for the feed.
func (c *Clubs) MineIDs(ctx context.Context, accessToken string) ([]string, error) {
	resp, err := c.api.ListMyClubIDs(withToken(ctx, accessToken), &clubv1.ListMyClubIDsRequest{})
	if err != nil {
		return nil, errs.Wrap("c.api.ListMyClubIDs", fromStatus(err))
	}

	return resp.GetClubIds(), nil
}

// CanPostOnWall tells whether the caller may write on the club's wall, on
// their own behalf or, fromClub, on the club's, or else suggest a post.
func (c *Clubs) CanPostOnWall(ctx context.Context, accessToken, clubID string, fromClub bool) (model.WallAccess, error) {
	resp, err := c.api.CanPostOnWall(withToken(ctx, accessToken), &clubv1.CanPostOnWallRequest{ClubId: clubID, FromClub: fromClub})
	if err != nil {
		return model.WallAccess{}, errs.Wrap("c.api.CanPostOnWall", fromStatus(err))
	}

	return model.WallAccess{Post: resp.GetAllowed(), Suggest: resp.GetSuggest()}, nil
}

func toClub(c *clubv1.Club) model.Club {
	return model.Club{
		ID:            c.GetId(),
		Name:          c.GetName(),
		Description:   c.GetDescription(),
		Address:       c.GetAddress(),
		Website:       c.GetWebsite(),
		Wall:          walls[c.GetWall()],
		AdminsDisplay: adminsDisplays[c.GetAdminsDisplay()],
		Members:       int(c.GetMemberCount()),
		CreatedAt:     c.GetCreatedAt().AsTime(),
		AvatarID:      c.GetAvatarId(),

		OpenTopics:      c.GetOpenTopics(),
		TopicsAboveWall: c.GetTopicsAboveWall(),
		BackdropLeft:    c.GetBackdropLeft(),
		BackdropRight:   c.GetBackdropRight(),
	}
}

func toClubView(v *clubv1.ClubView) model.ClubView {
	return model.ClubView{
		Club:    toClub(v.GetClub()),
		Role:    roles[v.GetRole()],
		OwnerID: v.GetOwnerId(),

		CanStartTopic: v.GetCanStartTopic(),
	}
}
