package backend

import (
	"context"
	"testing"

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
	md     metadata.MD
	update *clubv1.UpdateClubRequest
	role   *clubv1.SetMemberRoleRequest
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
	return &clubv1.ListMembersResponse{Members: []*clubv1.Member{{UserId: "u1", Role: clubv1.Role_ROLE_OWNER}}, NextCursor: "m1", Total: 2}, nil
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

	page, err := clubs.Members(ctx, "k1", "", 0, true)
	if err != nil || len(page.Members) != 1 || page.Members[0].Role != "owner" || page.NextCursor != "m1" || page.Total != 2 {
		t.Fatalf("Members = %+v, %v", page, err)
	}
}
