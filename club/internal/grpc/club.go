// Package grpc is the gRPC transport of the club service.
package grpc

import (
	"context"

	"github.com/google/uuid"
	clubv1 "github.com/vexner67/freenet/club/api/club/v1"
	"github.com/vexner67/freenet/club/internal/club"
	"github.com/vexner67/freenet/club/internal/service"
	"github.com/vexner67/freenet/pkg/jwtauth"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type ClubService interface {
	Create(ctx context.Context, ownerID uuid.UUID, name club.Name, description club.Description) (service.View, error)
	Find(ctx context.Context, viewerID uuid.UUID, ref service.Ref) (service.View, error)
	GetMany(ctx context.Context, ids []uuid.UUID) ([]*club.Club, error)
	Update(ctx context.Context, userID, clubID uuid.UUID, changes club.Changes) (service.View, error)
	CreateEvent(ctx context.Context, ownerID uuid.UUID, name club.Name, description club.Description, schedule club.Schedule) (service.View, error)
	Join(ctx context.Context, userID, clubID uuid.UUID, maybe bool) (service.View, error)
	Leave(ctx context.Context, userID, clubID uuid.UUID) (service.View, error)
	Appoint(ctx context.Context, requesterID, clubID, userID uuid.UUID, role club.Role) error
	Transfer(ctx context.Context, requesterID, clubID, userID uuid.UUID) error
	Describe(ctx context.Context, requesterID, clubID, userID uuid.UUID, title club.Title, hidden bool) error
	Members(ctx context.Context, viewerID, clubID, cursor uuid.UUID, size int, pick service.MemberPick) (service.MemberPage, error)
	Clubs(ctx context.Context, userID, cursor uuid.UUID, size int, filter service.ClubFilter) (service.ClubPage, error)
	Search(ctx context.Context, query club.SearchQuery, cursor uuid.UUID, size int) (service.SearchPage, error)
	Count(ctx context.Context) (int, error)
	SetAvatar(ctx context.Context, userID, clubID, avatarID uuid.UUID) (service.View, error)
	SetBackdrop(ctx context.Context, userID, clubID uuid.UUID, side club.BackdropSide, photoID uuid.UUID) (service.View, error)
	Pin(ctx context.Context, userID, clubID uuid.UUID, pinned bool) error
	Pinned(ctx context.Context, userID uuid.UUID) ([]*club.Club, error)
	ClubIDs(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error)
	CanPostOnWall(ctx context.Context, userID, clubID uuid.UUID, fromClub bool) (service.WallAccess, error)
	CanModerateWall(ctx context.Context, wallID, userID uuid.UUID) (bool, error)
	WallModerators(ctx context.Context, wallID uuid.UUID) ([]uuid.UUID, error)
	PurgeUser(ctx context.Context, userID uuid.UUID) error
}

type ClubHandler struct {
	clubv1.UnimplementedClubServiceServer
	clubService ClubService
}

func NewClubHandler(clubService ClubService) *ClubHandler {
	return &ClubHandler{
		clubService: clubService,
	}
}

func (h *ClubHandler) CreateClub(ctx context.Context, req *clubv1.CreateClubRequest) (*clubv1.CreateClubResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	name, err := club.NewName(req.GetName())
	if err != nil {
		return nil, err
	}

	description, err := club.NewDescription(req.GetDescription())
	if err != nil {
		return nil, err
	}

	v, err := h.clubService.Create(ctx, userID, name, description)
	if err != nil {
		return nil, err
	}

	return &clubv1.CreateClubResponse{Club: toProtoView(v)}, nil
}

func (h *ClubHandler) CreateEvent(ctx context.Context, req *clubv1.CreateEventRequest) (*clubv1.CreateEventResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	name, err := club.NewName(req.GetName())
	if err != nil {
		return nil, err
	}

	description, err := club.NewDescription(req.GetDescription())
	if err != nil {
		return nil, err
	}

	schedule, err := fromProtoSchedule(req.GetStartsAt(), req.GetEndsAt())
	if err != nil {
		return nil, err
	}

	v, err := h.clubService.CreateEvent(ctx, userID, name, description, schedule)
	if err != nil {
		return nil, err
	}

	return &clubv1.CreateEventResponse{Club: toProtoView(v)}, nil
}

func (h *ClubHandler) GetClub(ctx context.Context, req *clubv1.GetClubRequest) (*clubv1.GetClubResponse, error) {
	ref, err := fromProtoRef(req.GetRef())
	if err != nil {
		return nil, err
	}

	v, err := h.clubService.Find(ctx, viewerID(ctx), ref)
	if err != nil {
		return nil, err
	}

	return &clubv1.GetClubResponse{Club: toProtoView(v)}, nil
}

func (h *ClubHandler) GetClubs(ctx context.Context, req *clubv1.GetClubsRequest) (*clubv1.GetClubsResponse, error) {
	ids := make([]uuid.UUID, 0, len(req.GetIds()))
	for _, s := range req.GetIds() {
		id, err := parseID(s, "invalid club id")
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}

	clubs, err := h.clubService.GetMany(ctx, ids)
	if err != nil {
		return nil, err
	}

	return &clubv1.GetClubsResponse{Clubs: toProtoClubs(clubs)}, nil
}

func (h *ClubHandler) UpdateClub(ctx context.Context, req *clubv1.UpdateClubRequest) (*clubv1.UpdateClubResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	clubID, err := parseID(req.GetClubId(), "invalid club id")
	if err != nil {
		return nil, err
	}

	changes, err := fromProtoChanges(req)
	if err != nil {
		return nil, err
	}

	v, err := h.clubService.Update(ctx, userID, clubID, changes)
	if err != nil {
		return nil, err
	}

	return &clubv1.UpdateClubResponse{Club: toProtoView(v)}, nil
}

func (h *ClubHandler) JoinClub(ctx context.Context, req *clubv1.JoinClubRequest) (*clubv1.JoinClubResponse, error) {
	v, err := h.membership(ctx, req.GetClubId(), func(ctx context.Context, userID, clubID uuid.UUID) (service.View, error) {
		return h.clubService.Join(ctx, userID, clubID, req.GetMaybe())
	})
	if err != nil {
		return nil, err
	}

	return &clubv1.JoinClubResponse{Club: toProtoView(v)}, nil
}

func (h *ClubHandler) LeaveClub(ctx context.Context, req *clubv1.LeaveClubRequest) (*clubv1.LeaveClubResponse, error) {
	v, err := h.membership(ctx, req.GetClubId(), h.clubService.Leave)
	if err != nil {
		return nil, err
	}

	return &clubv1.LeaveClubResponse{Club: toProtoView(v)}, nil
}

func (h *ClubHandler) membership(ctx context.Context, id string, change func(ctx context.Context, userID, clubID uuid.UUID) (service.View, error)) (service.View, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return service.View{}, err
	}

	clubID, err := parseID(id, "invalid club id")
	if err != nil {
		return service.View{}, err
	}

	return change(ctx, userID, clubID)
}

func (h *ClubHandler) ListMembers(ctx context.Context, req *clubv1.ListMembersRequest) (*clubv1.ListMembersResponse, error) {
	clubID, err := parseID(req.GetClubId(), "invalid club id")
	if err != nil {
		return nil, err
	}

	cursor, err := parseOptionalID(req.GetCursor(), "invalid cursor")
	if err != nil {
		return nil, err
	}

	pick := picks[req.GetGoing()]
	if req.GetAdminsOnly() {
		pick = service.PickAdmins
	}

	page, err := h.clubService.Members(ctx, viewerID(ctx), clubID, cursor, int(req.GetLimit()), pick)
	if err != nil {
		return nil, err
	}

	resp := &clubv1.ListMembersResponse{
		Members:    make([]*clubv1.Member, 0, len(page.Members)),
		NextCursor: optionalID(page.NextCursor),
		Total:      int32(page.Total), //nolint:gosec // a count of rows
	}
	for _, m := range page.Members {
		resp.Members = append(resp.Members, &clubv1.Member{
			UserId: m.UserID.String(), Role: roles[m.Role], Title: m.Title.String(), Hidden: m.Hidden, Maybe: m.Maybe,
		})
	}

	return resp, nil
}

func (h *ClubHandler) SetMemberRole(ctx context.Context, req *clubv1.SetMemberRoleRequest) (*emptypb.Empty, error) {
	requesterID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	clubID, err := parseID(req.GetClubId(), "invalid club id")
	if err != nil {
		return nil, err
	}

	userID, err := parseID(req.GetUserId(), "invalid user id")
	if err != nil {
		return nil, err
	}

	role, err := fromProtoRole(req.GetRole())
	if err != nil {
		return nil, err
	}

	if err = h.clubService.Appoint(ctx, requesterID, clubID, userID, role); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *ClubHandler) DescribeMember(ctx context.Context, req *clubv1.DescribeMemberRequest) (*emptypb.Empty, error) {
	requesterID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	clubID, err := parseID(req.GetClubId(), "invalid club id")
	if err != nil {
		return nil, err
	}

	userID, err := parseID(req.GetUserId(), "invalid user id")
	if err != nil {
		return nil, err
	}

	title, err := club.NewTitle(req.GetTitle())
	if err != nil {
		return nil, err
	}

	if err = h.clubService.Describe(ctx, requesterID, clubID, userID, title, req.GetHidden()); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *ClubHandler) TransferClub(ctx context.Context, req *clubv1.TransferClubRequest) (*emptypb.Empty, error) {
	requesterID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	clubID, err := parseID(req.GetClubId(), "invalid club id")
	if err != nil {
		return nil, err
	}

	userID, err := parseID(req.GetUserId(), "invalid user id")
	if err != nil {
		return nil, err
	}

	if err = h.clubService.Transfer(ctx, requesterID, clubID, userID); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *ClubHandler) ListUserClubs(ctx context.Context, req *clubv1.ListUserClubsRequest) (*clubv1.ListUserClubsResponse, error) {
	userID, err := parseID(req.GetUserId(), "invalid user id")
	if err != nil {
		return nil, err
	}

	cursor, err := parseOptionalID(req.GetCursor(), "invalid cursor")
	if err != nil {
		return nil, err
	}

	page, err := h.clubService.Clubs(ctx, userID, cursor, int(req.GetLimit()), service.ClubFilter{
		Kind: fromProtoKind(req.GetKind()), Period: fromProtoPeriod(req.GetPeriod()),
	})
	if err != nil {
		return nil, err
	}

	// Only the clubs: what the user is in them and their menu are theirs.
	resp := &clubv1.ListUserClubsResponse{
		Clubs:      make([]*clubv1.Club, 0, len(page.Clubs)),
		NextCursor: optionalID(page.NextCursor),
		Total:      int32(page.Total), //nolint:gosec // a count of rows
	}
	for _, m := range page.Clubs {
		resp.Clubs = append(resp.Clubs, toProtoClub(m.Club))
	}

	return resp, nil
}

func (h *ClubHandler) ListMyClubs(ctx context.Context, req *clubv1.ListMyClubsRequest) (*clubv1.ListMyClubsResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	cursor, err := parseOptionalID(req.GetCursor(), "invalid cursor")
	if err != nil {
		return nil, err
	}

	filter := service.ClubFilter{
		Managed: req.GetManaged(), Kind: fromProtoKind(req.GetKind()), Period: fromProtoPeriod(req.GetPeriod()),
	}

	page, err := h.clubService.Clubs(ctx, userID, cursor, int(req.GetLimit()), filter)
	if err != nil {
		return nil, err
	}

	resp := &clubv1.ListMyClubsResponse{
		Clubs:      make([]*clubv1.Membership, 0, len(page.Clubs)),
		NextCursor: optionalID(page.NextCursor),
		Total:      int32(page.Total), //nolint:gosec // a count of rows
	}
	for _, m := range page.Clubs {
		resp.Clubs = append(resp.Clubs, &clubv1.Membership{Club: toProtoClub(m.Club), Role: roles[m.Role], Pinned: m.Pinned})
	}

	return resp, nil
}

func (h *ClubHandler) SearchClubs(ctx context.Context, req *clubv1.SearchClubsRequest) (*clubv1.SearchClubsResponse, error) {
	query, err := club.NewSearchQuery(req.GetQuery())
	if err != nil {
		return nil, err
	}

	cursor, err := parseOptionalID(req.GetCursor(), "invalid cursor")
	if err != nil {
		return nil, err
	}

	page, err := h.clubService.Search(ctx, query, cursor, int(req.GetLimit()))
	if err != nil {
		return nil, err
	}

	return &clubv1.SearchClubsResponse{Clubs: toProtoClubs(page.Clubs), NextCursor: optionalID(page.NextCursor)}, nil
}

// CountClubs counts the groups of the site, for its page "о сайте".
func (h *ClubHandler) CountClubs(ctx context.Context, _ *clubv1.CountClubsRequest) (*clubv1.CountClubsResponse, error) {
	n, err := h.clubService.Count(ctx)
	if err != nil {
		return nil, err
	}

	return &clubv1.CountClubsResponse{Count: int32(n)}, nil //nolint:gosec // a count of rows
}

func (h *ClubHandler) SetClubAvatar(ctx context.Context, req *clubv1.SetClubAvatarRequest) (*clubv1.SetClubAvatarResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	clubID, err := parseID(req.GetClubId(), "invalid club id")
	if err != nil {
		return nil, err
	}

	avatarID, err := parseOptionalID(req.GetAvatarId(), "invalid avatar id")
	if err != nil {
		return nil, err
	}

	v, err := h.clubService.SetAvatar(ctx, userID, clubID, avatarID)
	if err != nil {
		return nil, err
	}

	return &clubv1.SetClubAvatarResponse{Club: toProtoView(v)}, nil
}

var backdropSides = map[clubv1.BackdropSide]club.BackdropSide{
	clubv1.BackdropSide_BACKDROP_SIDE_LEFT:  club.BackdropLeft,
	clubv1.BackdropSide_BACKDROP_SIDE_RIGHT: club.BackdropRight,
}

func (h *ClubHandler) SetClubBackdrop(ctx context.Context, req *clubv1.SetClubBackdropRequest) (*clubv1.SetClubBackdropResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	clubID, err := parseID(req.GetClubId(), "invalid club id")
	if err != nil {
		return nil, err
	}

	photoID, err := parseOptionalID(req.GetPhotoId(), "invalid photo id")
	if err != nil {
		return nil, err
	}

	side, ok := backdropSides[req.GetSide()]
	if !ok {
		return nil, club.ErrInvalidBackdropSide
	}

	v, err := h.clubService.SetBackdrop(ctx, userID, clubID, side, photoID)
	if err != nil {
		return nil, err
	}

	return &clubv1.SetClubBackdropResponse{Club: toProtoView(v)}, nil
}

func (h *ClubHandler) PinClub(ctx context.Context, req *clubv1.PinClubRequest) (*emptypb.Empty, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	clubID, err := parseID(req.GetClubId(), "invalid club id")
	if err != nil {
		return nil, err
	}

	if err = h.clubService.Pin(ctx, userID, clubID, req.GetPinned()); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *ClubHandler) ListPinnedClubs(ctx context.Context, _ *clubv1.ListPinnedClubsRequest) (*clubv1.ListPinnedClubsResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	clubs, err := h.clubService.Pinned(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &clubv1.ListPinnedClubsResponse{Clubs: toProtoClubs(clubs)}, nil
}

func (h *ClubHandler) ListMyClubIDs(ctx context.Context, _ *clubv1.ListMyClubIDsRequest) (*clubv1.ListMyClubIDsResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	ids, err := h.clubService.ClubIDs(ctx, userID)
	if err != nil {
		return nil, err
	}

	resp := &clubv1.ListMyClubIDsResponse{ClubIds: make([]string, len(ids))}
	for i, id := range ids {
		resp.ClubIds[i] = id.String()
	}

	return resp, nil
}

func (h *ClubHandler) CanPostOnWall(ctx context.Context, req *clubv1.CanPostOnWallRequest) (*clubv1.CanPostOnWallResponse, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	clubID, err := parseID(req.GetClubId(), "invalid club id")
	if err != nil {
		return nil, err
	}

	access, err := h.clubService.CanPostOnWall(ctx, userID, clubID, req.GetFromClub())
	if err != nil {
		return nil, err
	}

	return &clubv1.CanPostOnWallResponse{Allowed: access.Post, Suggest: access.Suggest}, nil
}

func (h *ClubHandler) CanModerateWall(ctx context.Context, req *clubv1.CanModerateWallRequest) (*clubv1.CanModerateWallResponse, error) {
	wallID, err := parseID(req.GetWallId(), "invalid wall id")
	if err != nil {
		return nil, err
	}

	userID, err := parseID(req.GetUserId(), "invalid user id")
	if err != nil {
		return nil, err
	}

	ok, err := h.clubService.CanModerateWall(ctx, wallID, userID)
	if err != nil {
		return nil, err
	}

	return &clubv1.CanModerateWallResponse{Allowed: ok}, nil
}

func (h *ClubHandler) ListWallModerators(ctx context.Context, req *clubv1.ListWallModeratorsRequest) (*clubv1.ListWallModeratorsResponse, error) {
	wallID, err := parseID(req.GetWallId(), "invalid wall id")
	if err != nil {
		return nil, err
	}

	ids, err := h.clubService.WallModerators(ctx, wallID)
	if err != nil {
		return nil, err
	}

	resp := &clubv1.ListWallModeratorsResponse{UserIds: make([]string, 0, len(ids))}
	for _, id := range ids {
		resp.UserIds = append(resp.UserIds, id.String())
	}

	return resp, nil
}

func (h *ClubHandler) PurgeUser(ctx context.Context, req *clubv1.PurgeUserRequest) (*emptypb.Empty, error) {
	userID, err := parseID(req.GetUserId(), "invalid user id")
	if err != nil {
		return nil, err
	}

	if err = h.clubService.PurgeUser(ctx, userID); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func parseID(s, invalid string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, status.Error(codes.InvalidArgument, invalid)
	}

	return id, nil
}

// parseOptionalID reads a cursor; "" is uuid.Nil, the first page.
func parseOptionalID(s, invalid string) (uuid.UUID, error) {
	if s == "" {
		return uuid.Nil, nil
	}

	return parseID(s, invalid)
}

// viewerID is the caller of a public method, uuid.Nil if anonymous.
func viewerID(ctx context.Context) uuid.UUID {
	identity, ok := jwtauth.IdentityFromContext(ctx)
	if !ok {
		return uuid.Nil
	}

	return identity.UserID
}

// currentUserID is the caller's ID from the access token verified by
// jwtauth.UnaryServerInterceptor; never trust an ID from the request body.
func currentUserID(ctx context.Context) (uuid.UUID, error) {
	identity, ok := jwtauth.IdentityFromContext(ctx)
	if !ok {
		return uuid.Nil, status.Error(codes.Unauthenticated, "missing access token")
	}

	return identity.UserID, nil
}
