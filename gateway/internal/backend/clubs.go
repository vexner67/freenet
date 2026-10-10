package backend

import (
	"context"
	"time"

	clubv1 "github.com/vexner67/freenet/club/api/club/v1"
	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/pkg/errs"
	"google.golang.org/protobuf/types/known/timestamppb"
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
	kinds = map[clubv1.Kind]string{
		clubv1.Kind_KIND_GROUP: model.ClubKindGroup,
		clubv1.Kind_KIND_EVENT: model.ClubKindEvent,
	}
	// Anything else is no period: all the clubs.
	periods = map[string]clubv1.Period{
		model.ClubPeriodUpcoming: clubv1.Period_PERIOD_UPCOMING,
		model.ClubPeriodPast:     clubv1.Period_PERIOD_PAST,
	}
	goings = map[string]clubv1.Going{
		"":      clubv1.Going_GOING_UNSPECIFIED,
		"sure":  clubv1.Going_GOING_SURE,
		"maybe": clubv1.Going_GOING_MAYBE,
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

// CreateEvent makes an event ("Создать встречу").
func (c *Clubs) CreateEvent(ctx context.Context, accessToken string, e model.NewEvent) (model.ClubView, error) {
	resp, err := c.api.CreateEvent(withToken(ctx, accessToken), &clubv1.CreateEventRequest{
		Name: e.Name, Description: e.Description, StartsAt: optionalTimestamp(e.StartsAt), EndsAt: optionalTimestamp(e.EndsAt),
	})
	if err != nil {
		return model.ClubView{}, errs.Wrap("c.api.CreateEvent", fromStatus(err))
	}

	return toClubView(resp.GetClub()), nil
}

// optionalTimestamp is unset for the zero time.
func optionalTimestamp(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}

	return timestamppb.New(t)
}

// optionalTime is the zero time for an unset one.
func optionalTime(t *timestamppb.Timestamp) time.Time {
	if t == nil {
		return time.Time{}
	}

	return t.AsTime()
}

// kindOf is the proto of a kind of clubs; "" is both.
func kindOf(kind string) clubv1.Kind {
	for pk, k := range kinds {
		if k == kind {
			return pk
		}
	}

	return clubv1.Kind_KIND_UNSPECIFIED
}

// Get is the club ref names: its ID or its address.
func (c *Clubs) Get(ctx context.Context, accessToken, ref string) (model.ClubView, error) {
	resp, err := c.api.GetClub(withToken(ctx, accessToken), &clubv1.GetClubRequest{Ref: ref})
	if err != nil {
		return model.ClubView{}, errs.Wrap("c.api.GetClub", fromStatus(err))
	}

	return toClubView(resp.GetClub()), nil
}

// Count counts the groups of the site.
func (c *Clubs) Count(ctx context.Context) (int, error) {
	resp, err := c.api.CountClubs(ctx, &clubv1.CountClubsRequest{})
	if err != nil {
		return 0, errs.Wrap("c.api.CountClubs", fromStatus(err))
	}

	return int(resp.GetCount()), nil
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
		StartsAt:        optionalTimestamp(changes.StartsAt),
		EndsAt:          optionalTimestamp(changes.EndsAt),
	})
	if err != nil {
		return model.ClubView{}, errs.Wrap("c.api.UpdateClub", fromStatus(err))
	}

	return toClubView(resp.GetClub()), nil
}

// Join makes the caller a member; with maybe, one who only might come to
// the event.
func (c *Clubs) Join(ctx context.Context, accessToken, clubID string, maybe bool) (model.ClubView, error) {
	resp, err := c.api.JoinClub(withToken(ctx, accessToken), &clubv1.JoinClubRequest{ClubId: clubID, Maybe: maybe})
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

// Members pages the members of a club that filter picks, as the caller
// sees them.
func (c *Clubs) Members(ctx context.Context, accessToken, clubID, cursor string, limit int, filter model.ClubMemberFilter) (model.ClubMemberPage, error) {
	going, ok := goings[filter.Going]
	if !ok {
		return model.ClubMemberPage{}, failure.New(failure.InvalidArgument, "going must be sure or maybe")
	}

	resp, err := c.api.ListMembers(withToken(ctx, accessToken), &clubv1.ListMembersRequest{
		ClubId:     clubID,
		Cursor:     cursor,
		Limit:      toLimit(limit),
		AdminsOnly: filter.AdminsOnly,
		Going:      going,
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
		page.Members = append(page.Members, model.ClubMember{
			UserID: m.GetUserId(), Role: roles[m.GetRole()], Title: m.GetTitle(), Hidden: m.GetHidden(),
			Maybe: m.GetMaybe(),
		})
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

// Describe gives one of those who run the club a title and hides or
// shows them.
func (c *Clubs) Describe(ctx context.Context, accessToken, clubID, userID, title string, hidden bool) error {
	_, err := c.api.DescribeMember(withToken(ctx, accessToken), &clubv1.DescribeMemberRequest{
		ClubId: clubID, UserId: userID, Title: title, Hidden: hidden,
	})
	if err != nil {
		return errs.Wrap("c.api.DescribeMember", fromStatus(err))
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
func (c *Clubs) OfUser(ctx context.Context, userID, cursor string, limit int, filter model.ClubFilter) (model.UserClubPage, error) {
	resp, err := c.api.ListUserClubs(ctx, &clubv1.ListUserClubsRequest{
		UserId: userID, Cursor: cursor, Limit: toLimit(limit), Kind: kindOf(filter.Kind), Period: periods[filter.Period],
	})
	if err != nil {
		return model.UserClubPage{}, errs.Wrap("c.api.ListUserClubs", fromStatus(err))
	}

	return model.UserClubPage{Clubs: toClubs(resp.GetClubs()), NextCursor: resp.GetNextCursor(), Total: int(resp.GetTotal())}, nil
}

// Mine pages the caller's clubs; with managed only those they run.
func (c *Clubs) Mine(ctx context.Context, accessToken, cursor string, limit int, filter model.ClubFilter) (model.ClubPage, error) {
	resp, err := c.api.ListMyClubs(withToken(ctx, accessToken), &clubv1.ListMyClubsRequest{
		Cursor: cursor, Limit: toLimit(limit), Managed: filter.Managed, Kind: kindOf(filter.Kind), Period: periods[filter.Period],
	})
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

		Kind:     kinds[c.GetKind()],
		StartsAt: optionalTime(c.GetStartsAt()),
		EndsAt:   optionalTime(c.GetEndsAt()),
		Maybes:   int(c.GetMaybeCount()),
	}
}

func toClubView(v *clubv1.ClubView) model.ClubView {
	return model.ClubView{
		Club:    toClub(v.GetClub()),
		Role:    roles[v.GetRole()],
		OwnerID: v.GetOwnerId(),

		OwnerTitle: v.GetOwnerTitle(),
		Maybe:      v.GetMaybe(),

		CanStartTopic: v.GetCanStartTopic(),
	}
}
