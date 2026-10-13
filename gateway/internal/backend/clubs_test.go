package backend

import (
	"context"
	"slices"
	"testing"
	"time"

	clubv1 "github.com/vexner67/freenet/club/api/club/v1"
	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type fakeClubAPI struct {
	clubv1.ClubServiceClient
	md        metadata.MD
	update    *clubv1.UpdateClubRequest
	role      *clubv1.SetMemberRoleRequest
	described *clubv1.DescribeMemberRequest
	// hiddenOf are the clubs ListHiddenManagers was asked about.
	hiddenOf []string
	event    *clubv1.CreateEventRequest
	join     *clubv1.JoinClubRequest
	mine     *clubv1.ListMyClubsRequest
}

var protoCats = &clubv1.Club{
	Id: "k1", Name: "Кошки", Address: "cats", Website: "https://cats.example", Wall: clubv1.Wall_WALL_CLOSED,
	AdminsDisplay: clubv1.AdminsDisplay_ADMINS_DISPLAY_ALL, MemberCount: 3, CreatedAt: timestamppb.Now(),
}

func (f *fakeClubAPI) GetClub(ctx context.Context, req *clubv1.GetClubRequest, _ ...grpc.CallOption) (*clubv1.GetClubResponse, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	if req.GetRef() != "cats" {
		return nil, status.Error(codes.NotFound, "club not found")
	}
	return &clubv1.GetClubResponse{Club: &clubv1.ClubView{Club: protoCats, Role: clubv1.Role_ROLE_ADMIN, OwnerId: "u1"}}, nil
}

func (f *fakeClubAPI) UpdateClub(_ context.Context, req *clubv1.UpdateClubRequest, _ ...grpc.CallOption) (*clubv1.UpdateClubResponse, error) {
	f.update = req
	return &clubv1.UpdateClubResponse{Club: &clubv1.ClubView{Club: protoCats}}, nil
}

func (f *fakeClubAPI) SetMemberRole(_ context.Context, req *clubv1.SetMemberRoleRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	f.role = req
	return &emptypb.Empty{}, nil
}

func (f *fakeClubAPI) ListMembers(_ context.Context, _ *clubv1.ListMembersRequest, _ ...grpc.CallOption) (*clubv1.ListMembersResponse, error) {
	return &clubv1.ListMembersResponse{
		Members:    []*clubv1.Member{{UserId: "u1", Role: clubv1.Role_ROLE_OWNER, Title: "главная", Hidden: true}},
		NextCursor: "m1", Total: 2,
	}, nil
}

func (f *fakeClubAPI) ListHiddenManagers(ctx context.Context, req *clubv1.ListHiddenManagersRequest, _ ...grpc.CallOption) (*clubv1.ListHiddenManagersResponse, error) {
	f.hiddenOf = req.GetClubIds()
	f.md, _ = metadata.FromOutgoingContext(ctx)

	return &clubv1.ListHiddenManagersResponse{Clubs: []*clubv1.HiddenManagers{{ClubId: "k1", UserIds: []string{"u2"}}}}, nil
}

func (f *fakeClubAPI) DescribeMember(_ context.Context, req *clubv1.DescribeMemberRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	f.described = req
	return &emptypb.Empty{}, nil
}

func TestClubs(t *testing.T) {
	t.Parallel()

	api := &fakeClubAPI{}
	clubs := NewClubs(api)
	ctx := context.Background()

	v, err := clubs.Get(ctx, "tok", "cats")
	if err != nil || v.Club.ID != "k1" || v.Club.Wall != "closed" || v.Club.Members != 3 || v.Role != "admin" || v.OwnerID != "u1" ||
		v.Club.Website != "https://cats.example" || v.Club.AdminsDisplay != "all" {
		t.Fatalf("Get = %+v, %v", v, err)
	}
	if got := api.md.Get("authorization"); len(got) != 1 || got[0] != "Bearer tok" {
		t.Fatalf("authorization metadata = %v", got)
	}
	if _, err = clubs.Get(ctx, "", "dogs"); failure.From(err).Kind != failure.NotFound {
		t.Fatalf("unknown club: %v", err)
	}

	changes := model.ClubChanges{Name: "Коты", Website: "cats.example", Wall: "open", AdminsDisplay: "none"}
	if _, err = clubs.Update(ctx, "tok", "k1", changes); err != nil || api.update.GetWall() != clubv1.Wall_WALL_OPEN || api.update.GetName() != "Коты" ||
		api.update.GetWebsite() != "cats.example" || api.update.GetAdminsDisplay() != clubv1.AdminsDisplay_ADMINS_DISPLAY_NONE {
		t.Fatalf("Update = %v, request %v", err, api.update)
	}
	if _, err = clubs.Update(ctx, "tok", "k1", model.ClubChanges{Name: "Коты", Wall: "limited", AdminsDisplay: "all"}); failure.From(err).Kind != failure.InvalidArgument {
		t.Fatalf("an unknown wall mode: %v", err)
	}
	if _, err = clubs.Update(ctx, "tok", "k1", model.ClubChanges{Name: "Коты", Wall: "open"}); failure.From(err).Kind != failure.InvalidArgument {
		t.Fatalf("no admins display: %v", err)
	}

	if err = clubs.SetAdmin(ctx, "tok", "k1", "u2", true); err != nil || api.role.GetRole() != clubv1.Role_ROLE_ADMIN || api.role.GetUserId() != "u2" {
		t.Fatalf("SetAdmin = %v, request %v", err, api.role)
	}
	if err = clubs.SetAdmin(ctx, "tok", "k1", "u2", false); err != nil || api.role.GetRole() != clubv1.Role_ROLE_MEMBER {
		t.Fatalf("SetAdmin(false) = %v, request %v", err, api.role)
	}

	if err = clubs.Describe(ctx, "tok", "k1", "u2", model.SeatChange{Title: new("кормит котов"), Hidden: new(true)}); err != nil || api.described.GetUserId() != "u2" ||
		api.described.GetTitle() != "кормит котов" || !api.described.GetHidden() {
		t.Fatalf("Describe = %v, request %v", err, api.described)
	}
	if api.described.BaseTitle != nil {
		t.Fatalf("a change that names no title it started from sent %q", api.described.GetBaseTitle())
	}
	if err = clubs.Describe(ctx, "tok", "k1", "u2", model.SeatChange{Title: new("повар"), BaseTitle: new("")}); err != nil || api.described.BaseTitle == nil || api.described.GetBaseTitle() != "" {
		t.Fatalf("Describe from no title = %v, request %v", err, api.described)
	}
	hid, err := clubs.HiddenManagers(ctx, "tok", []string{"k1", "k2"})
	if err != nil || len(hid) != 1 || !slices.Equal(hid["k1"], []string{"u2"}) || !slices.Equal(api.hiddenOf, []string{"k1", "k2"}) ||
		!slices.Equal(api.md.Get("authorization"), []string{"Bearer tok"}) {
		t.Fatalf("HiddenManagers = %v, %v; asked of %v with %v", hid, err, api.hiddenOf, api.md)
	}
	// What the change does not name is not sent.
	if err = clubs.Describe(ctx, "tok", "k1", "u2", model.SeatChange{Hidden: new(false)}); err != nil || api.described.Title != nil || api.described.Hidden == nil {
		t.Fatalf("Describe of the hiding = %v, request %v", err, api.described)
	}

	page, err := clubs.Members(ctx, "tok", "k1", "", 0, model.ClubMemberFilter{AdminsOnly: true})
	if err != nil || len(page.Members) != 1 || page.Members[0].Role != "owner" || page.Members[0].Title != "главная" || !page.Members[0].Hidden || page.NextCursor != "m1" || page.Total != 2 {
		t.Fatalf("Members = %+v, %v", page, err)
	}
}

func (f *fakeClubAPI) CreateEvent(_ context.Context, req *clubv1.CreateEventRequest, _ ...grpc.CallOption) (*clubv1.CreateEventResponse, error) {
	f.event = req
	return &clubv1.CreateEventResponse{Club: &clubv1.ClubView{
		Club:  &clubv1.Club{Id: "e1", Name: req.GetName(), Kind: clubv1.Kind_KIND_EVENT, StartsAt: req.GetStartsAt(), EndsAt: req.GetEndsAt(), MemberCount: 3, MaybeCount: 1},
		Role:  clubv1.Role_ROLE_OWNER,
		Maybe: true,
	}}, nil
}

func (f *fakeClubAPI) JoinClub(_ context.Context, req *clubv1.JoinClubRequest, _ ...grpc.CallOption) (*clubv1.JoinClubResponse, error) {
	f.join = req
	return &clubv1.JoinClubResponse{Club: &clubv1.ClubView{Club: protoCats}}, nil
}

func (f *fakeClubAPI) ListMyClubs(_ context.Context, req *clubv1.ListMyClubsRequest, _ ...grpc.CallOption) (*clubv1.ListMyClubsResponse, error) {
	f.mine = req
	return &clubv1.ListMyClubsResponse{}, nil
}

func TestClubs_Events(t *testing.T) {
	t.Parallel()

	api := &fakeClubAPI{}
	clubs := NewClubs(api)
	ctx := context.Background()
	start := time.Date(2026, 12, 31, 20, 0, 0, 0, time.UTC)

	v, err := clubs.CreateEvent(ctx, "tok", model.NewEvent{Name: "Новый год", StartsAt: start})
	if err != nil || v.Club.Kind != model.ClubKindEvent || !v.Club.StartsAt.Equal(start) || !v.Club.EndsAt.IsZero() || v.Club.Maybes != 1 || !v.Maybe {
		t.Fatalf("CreateEvent = %+v, %v", v, err)
	}
	if api.event.GetEndsAt() != nil || api.event.GetStartsAt().AsTime() != start {
		t.Fatalf("request = %v; want no end", api.event)
	}

	if _, err = clubs.Join(ctx, "tok", "e1", true); err != nil || !api.join.GetMaybe() {
		t.Fatalf("Join = %v, request %v", err, api.join)
	}

	if _, err = clubs.Mine(ctx, "tok", "", 0, model.ClubFilter{Managed: true, Kind: model.ClubKindEvent}); err != nil ||
		api.mine.GetKind() != clubv1.Kind_KIND_EVENT || !api.mine.GetManaged() {
		t.Fatalf("Mine = %v, request %v", err, api.mine)
	}
	if _, err = clubs.Mine(ctx, "tok", "", 0, model.ClubFilter{}); err != nil || api.mine.GetKind() != clubv1.Kind_KIND_UNSPECIFIED {
		t.Fatalf("Mine of any kind = %v, request %v", err, api.mine)
	}

	if _, err = clubs.Members(ctx, "", "e1", "", 0, model.ClubMemberFilter{Going: "later"}); failure.From(err).Kind != failure.InvalidArgument {
		t.Fatalf("an unknown going: %v", err)
	}
	if _, err = clubs.Members(ctx, "", "e1", "", 0, model.ClubMemberFilter{Going: "maybe"}); err != nil {
		t.Fatalf("those who might come: %v", err)
	}
}
