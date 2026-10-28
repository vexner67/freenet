package grpc_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"io"
	"log/slog"
	"net"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	clubv1 "github.com/vexner67/freenet/club/api/club/v1"
	clubgrpc "github.com/vexner67/freenet/club/internal/grpc"
	"github.com/vexner67/freenet/club/internal/inmem"
	"github.com/vexner67/freenet/club/internal/notifyclient"
	"github.com/vexner67/freenet/club/internal/postclient"
	"github.com/vexner67/freenet/club/internal/service"
	"github.com/vexner67/freenet/pkg/jwtauth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const issuer = "freenet-test"

type testServer struct {
	client clubv1.ClubServiceClient
	key    *rsa.PrivateKey
}

// newServer wires the handler the same way app.New does, without mTLS.
func newServer(t *testing.T) testServer {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	lis := bufconn.Listen(1 << 20)
	verifier := jwtauth.NewVerifier(&key.PublicKey, issuer)
	srv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			clubgrpc.ErrorInterceptor(log),
			jwtauth.UnaryServerInterceptor(verifier, jwtauth.Access{Public: clubgrpc.PublicMethods()}),
		),
	)
	store := inmem.NewStore()
	clubv1.RegisterClubServiceServer(srv, clubgrpc.NewClubHandler(service.NewClubService(service.ClubDeps{
		Clubs: store.Clubs(), Members: store.Members(), Requests: store.Requests(), Tx: inmem.TxManager{}, Notifier: notifyclient.Nop{}, Readers: postclient.Nop{},
	})))

	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	return testServer{client: clubv1.NewClubServiceClient(conn), key: key}
}

// as returns a context of the given user with a signed access token.
func (s testServer) as(t *testing.T, userID uuid.UUID) context.Context {
	t.Helper()

	token, err := jwt.NewWithClaims(jwt.SigningMethodRS256, jwtauth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			Issuer:    issuer,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		},
		TokenType: jwtauth.AccessToken,
		SessionID: uuid.NewString(),
	}).SignedString(s.key)
	if err != nil {
		t.Fatal(err)
	}

	return metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+token)
}

func TestClub(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, bob := uuid.New(), uuid.New()

	created, err := s.client.CreateClub(s.as(t, ann), &clubv1.CreateClubRequest{Name: " Любители котов ", Description: "про котов"})
	if err != nil {
		t.Fatalf("CreateClub: %v", err)
	}
	v := created.GetClub()
	id := v.GetClub().GetId()
	if v.GetRole() != clubv1.Role_ROLE_OWNER || v.GetOwnerId() != ann.String() || v.GetClub().GetName() != "Любители котов" ||
		v.GetClub().GetWall() != clubv1.Wall_WALL_OPEN || v.GetClub().GetMemberCount() != 1 || v.GetClub().GetCreatedAt() == nil {
		t.Fatalf("created = %v", v)
	}
	if !v.GetCanStartTopic() {
		t.Fatal("the owner cannot start topics")
	}

	updated, err := s.client.UpdateClub(s.as(t, ann), &clubv1.UpdateClubRequest{
		ClubId: id, Name: "Кошки", Description: "мяу", Address: "cats", Wall: clubv1.Wall_WALL_CLOSED,
		Website: "cats.example", AdminsDisplay: clubv1.AdminsDisplay_ADMINS_DISPLAY_ALL, OpenTopics: true, TopicsAboveWall: true,
	})
	if err != nil || updated.GetClub().GetClub().GetAddress() != "cats" || updated.GetClub().GetClub().GetWall() != clubv1.Wall_WALL_CLOSED ||
		updated.GetClub().GetClub().GetWebsite() != "https://cats.example" ||
		updated.GetClub().GetClub().GetAdminsDisplay() != clubv1.AdminsDisplay_ADMINS_DISPLAY_ALL ||
		!updated.GetClub().GetClub().GetOpenTopics() || !updated.GetClub().GetClub().GetTopicsAboveWall() {
		t.Fatalf("UpdateClub = %v, %v", updated, err)
	}

	// Public, by the ID or the address, anonymous or not.
	for _, ref := range []string{id, "cats", "CATS"} {
		got, err := s.client.GetClub(context.Background(), &clubv1.GetClubRequest{Ref: ref})
		if err != nil || got.GetClub().GetClub().GetId() != id || got.GetClub().GetRole() != clubv1.Role_ROLE_NONE || !got.GetClub().GetCanStartTopic() {
			t.Fatalf("GetClub(%s) = %v, %v", ref, got, err)
		}
	}

	joined, err := s.client.JoinClub(s.as(t, bob), &clubv1.JoinClubRequest{ClubId: id})
	if err != nil || joined.GetClub().GetRole() != clubv1.Role_ROLE_MEMBER || joined.GetClub().GetClub().GetMemberCount() != 2 {
		t.Fatalf("JoinClub = %v, %v", joined, err)
	}

	can, err := s.client.CanPostOnWall(s.as(t, bob), &clubv1.CanPostOnWallRequest{ClubId: id})
	if err != nil || can.GetAllowed() {
		t.Fatalf("a member on a closed wall = %v, %v", can, err)
	}

	if _, err = s.client.SetMemberRole(s.as(t, ann), &clubv1.SetMemberRoleRequest{ClubId: id, UserId: bob.String(), Role: clubv1.Role_ROLE_ADMIN}); err != nil {
		t.Fatalf("SetMemberRole: %v", err)
	}
	if can, err = s.client.CanPostOnWall(s.as(t, bob), &clubv1.CanPostOnWallRequest{ClubId: id, FromClub: true}); err != nil || !can.GetAllowed() {
		t.Fatalf("an admin for the club = %v, %v", can, err)
	}
	mod, err := s.client.CanModerateWall(context.Background(), &clubv1.CanModerateWallRequest{WallId: id, UserId: bob.String()})
	if err != nil || !mod.GetAllowed() {
		t.Fatalf("CanModerateWall = %v, %v", mod, err)
	}
	mods, err := s.client.ListWallModerators(context.Background(), &clubv1.ListWallModeratorsRequest{WallId: id})
	if err != nil || !slices.Equal(mods.GetUserIds(), []string{bob.String(), ann.String()}) {
		t.Fatalf("ListWallModerators = %v, %v", mods, err)
	}

	admins, err := s.client.ListMembers(context.Background(), &clubv1.ListMembersRequest{ClubId: id, AdminsOnly: true})
	if err != nil || len(admins.GetMembers()) != 2 || admins.GetTotal() != 2 || admins.GetMembers()[0].GetUserId() != bob.String() || admins.GetMembers()[0].GetRole() != clubv1.Role_ROLE_ADMIN {
		t.Fatalf("admins = %v, %v", admins, err)
	}

	// Ann gives Bob a title and hides him: only those who run the club see it.
	if _, err = s.client.DescribeMember(s.as(t, ann), &clubv1.DescribeMemberRequest{ClubId: id, UserId: bob.String(), Title: new(" кормит  котов "), Hidden: new(true)}); err != nil {
		t.Fatalf("DescribeMember: %v", err)
	}
	if _, err = s.client.DescribeMember(s.as(t, ann), &clubv1.DescribeMemberRequest{ClubId: id, UserId: bob.String(), Title: new(strings.Repeat("я", 37))}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("a long title: %v", err)
	}
	if _, err = s.client.DescribeMember(s.as(t, uuid.New()), &clubv1.DescribeMemberRequest{ClubId: id, UserId: bob.String()}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("DescribeMember by a stranger: %v", err)
	}
	if _, err = s.client.DescribeMember(s.as(t, ann), &clubv1.DescribeMemberRequest{ClubId: id, UserId: uuid.NewString()}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("DescribeMember of a stranger: %v", err)
	}
	if _, err = s.client.DescribeMember(context.Background(), &clubv1.DescribeMemberRequest{ClubId: id, UserId: bob.String()}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("DescribeMember without a token: %v", err)
	}
	seen, err := s.client.ListMembers(s.as(t, ann), &clubv1.ListMembersRequest{ClubId: id, AdminsOnly: true})
	if err != nil || seen.GetTotal() != 2 || seen.GetMembers()[0].GetTitle() != "кормит котов" || !seen.GetMembers()[0].GetHidden() {
		t.Fatalf("admins to the owner = %v, %v", seen, err)
	}
	if admins, err = s.client.ListMembers(context.Background(), &clubv1.ListMembersRequest{ClubId: id, AdminsOnly: true}); err != nil || admins.GetTotal() != 1 || admins.GetMembers()[0].GetUserId() != ann.String() {
		t.Fatalf("admins to anyone = %v, %v; want only the owner", admins, err)
	}
	// Whom the club hides: told to those who do not run it, for gateway
	// to leave his name out.
	for name, ctx := range map[string]context.Context{"anyone": context.Background(), "a stranger": s.as(t, uuid.New())} {
		hid, err := s.client.ListHiddenManagers(ctx, &clubv1.ListHiddenManagersRequest{ClubIds: []string{id, uuid.NewString(), id}})
		if err != nil || len(hid.GetClubs()) != 1 || hid.GetClubs()[0].GetClubId() != id || len(hid.GetClubs()[0].GetUserIds()) != 1 || hid.GetClubs()[0].GetUserIds()[0] != bob.String() {
			t.Fatalf("ListHiddenManagers for %s = %v, %v; want Bob", name, hid, err)
		}
	}
	if hid, err := s.client.ListHiddenManagers(s.as(t, ann), &clubv1.ListHiddenManagersRequest{ClubIds: []string{id}}); err != nil || len(hid.GetClubs()) != 0 {
		t.Fatalf("ListHiddenManagers for the owner = %v, %v; want none", hid, err)
	}
	if _, err = s.client.ListHiddenManagers(context.Background(), &clubv1.ListHiddenManagersRequest{ClubIds: []string{"x"}}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("ListHiddenManagers of a bad id: %v", err)
	}

	// A title is not written over one the request did not start from.
	if _, err = s.client.DescribeMember(s.as(t, ann), &clubv1.DescribeMemberRequest{ClubId: id, UserId: bob.String(), Title: new("повар"), BaseTitle: new("")}); status.Code(err) != codes.FailedPrecondition || status.Convert(err).Message() != "the title was changed" {
		t.Fatalf("a title over one the request did not see: %v", err)
	}
	if _, err = s.client.DescribeMember(s.as(t, ann), &clubv1.DescribeMemberRequest{ClubId: id, UserId: bob.String(), Title: new("кормит котов"), BaseTitle: new(" кормит котов ")}); err != nil {
		t.Fatalf("a title over the one the request saw: %v", err)
	}
	if _, err = s.client.DescribeMember(s.as(t, ann), &clubv1.DescribeMemberRequest{ClubId: id, UserId: bob.String(), Title: new("x"), BaseTitle: new(strings.Repeat("я", 37))}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("a long base title: %v", err)
	}
	// Showing himself, Bob keeps the title: a request changes what it names.
	if _, err = s.client.DescribeMember(s.as(t, bob), &clubv1.DescribeMemberRequest{ClubId: id, UserId: bob.String(), Hidden: new(false)}); err != nil {
		t.Fatalf("an admin shows himself: %v", err)
	}
	if admins, err = s.client.ListMembers(context.Background(), &clubv1.ListMembersRequest{ClubId: id, AdminsOnly: true}); err != nil || admins.GetTotal() != 2 || admins.GetMembers()[0].GetTitle() != "кормит котов" {
		t.Fatalf("admins once Bob is shown = %v, %v", admins, err)
	}

	// Public: anyone sees the clubs of a user, but not their role or menu.
	theirs, err := s.client.ListUserClubs(context.Background(), &clubv1.ListUserClubsRequest{UserId: bob.String()})
	if err != nil || len(theirs.GetClubs()) != 1 || theirs.GetClubs()[0].GetId() != id || theirs.GetTotal() != 1 {
		t.Fatalf("ListUserClubs = %v, %v", theirs, err)
	}

	mine, err := s.client.ListMyClubs(s.as(t, bob), &clubv1.ListMyClubsRequest{})
	if err != nil || len(mine.GetClubs()) != 1 || mine.GetClubs()[0].GetRole() != clubv1.Role_ROLE_ADMIN {
		t.Fatalf("ListMyClubs = %v, %v", mine, err)
	}
	roles, err := s.client.ListMyRoles(s.as(t, bob), &clubv1.ListMyRolesRequest{ClubIds: []string{uuid.NewString(), id}})
	if err != nil || len(roles.GetRoles()) != 1 || roles.GetRoles()[0].GetClubId() != id || roles.GetRoles()[0].GetRole() != clubv1.Role_ROLE_ADMIN {
		t.Fatalf("ListMyRoles = %v, %v", roles, err)
	}
	if _, err = s.client.ListMyRoles(context.Background(), &clubv1.ListMyRolesRequest{ClubIds: []string{id}}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("ListMyRoles without a token: %v", err)
	}
	many, err := s.client.GetClubs(context.Background(), &clubv1.GetClubsRequest{Ids: []string{id, uuid.NewString()}})
	if err != nil || len(many.GetClubs()) != 1 {
		t.Fatalf("GetClubs = %v, %v", many, err)
	}

	// Ann hands the club over to Bob and leaves it to him.
	if _, err = s.client.TransferClub(s.as(t, ann), &clubv1.TransferClubRequest{ClubId: id, UserId: bob.String()}); err != nil {
		t.Fatalf("TransferClub: %v", err)
	}
	left, err := s.client.LeaveClub(s.as(t, ann), &clubv1.LeaveClubRequest{ClubId: id})
	if err != nil || left.GetClub().GetRole() != clubv1.Role_ROLE_NONE || left.GetClub().GetOwnerId() != bob.String() || left.GetClub().GetClub().GetMemberCount() != 1 {
		t.Fatalf("LeaveClub = %v, %v", left, err)
	}

	if _, err = s.client.PurgeUser(context.Background(), &clubv1.PurgeUserRequest{UserId: ann.String()}); err != nil {
		t.Fatalf("PurgeUser: %v", err)
	}
}

func TestErrors(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, bob := uuid.New(), uuid.New()
	created, err := s.client.CreateClub(s.as(t, ann), &clubv1.CreateClubRequest{Name: "Кошки"})
	if err != nil {
		t.Fatal(err)
	}
	id := created.GetClub().GetClub().GetId()

	tests := []struct {
		name string
		call func() error
		want codes.Code
	}{
		{"create without token", func() error {
			_, err := s.client.CreateClub(context.Background(), &clubv1.CreateClubRequest{Name: "x"})
			return err
		}, codes.Unauthenticated},
		{"empty name", func() error {
			_, err := s.client.CreateClub(s.as(t, ann), &clubv1.CreateClubRequest{Name: " "})
			return err
		}, codes.InvalidArgument},
		{"unknown club", func() error {
			_, err := s.client.GetClub(context.Background(), &clubv1.GetClubRequest{Ref: uuid.NewString()})
			return err
		}, codes.NotFound},
		{"not even an address", func() error {
			_, err := s.client.GetClub(context.Background(), &clubv1.GetClubRequest{Ref: "кошки!"})
			return err
		}, codes.NotFound},
		{"a stranger edits", func() error {
			_, err := s.client.UpdateClub(s.as(t, bob), &clubv1.UpdateClubRequest{ClubId: id, Name: "x", Wall: clubv1.Wall_WALL_OPEN, AdminsDisplay: clubv1.AdminsDisplay_ADMINS_DISPLAY_CREATOR})
			return err
		}, codes.PermissionDenied},
		{"no wall mode", func() error {
			_, err := s.client.UpdateClub(s.as(t, ann), &clubv1.UpdateClubRequest{ClubId: id, Name: "x", AdminsDisplay: clubv1.AdminsDisplay_ADMINS_DISPLAY_CREATOR})
			return err
		}, codes.InvalidArgument},
		{"nothing chosen for the admins", func() error {
			_, err := s.client.UpdateClub(s.as(t, ann), &clubv1.UpdateClubRequest{ClubId: id, Name: "x", Wall: clubv1.Wall_WALL_OPEN})
			return err
		}, codes.InvalidArgument},
		{"a bad website", func() error {
			_, err := s.client.UpdateClub(s.as(t, ann), &clubv1.UpdateClubRequest{ClubId: id, Name: "x", Website: "javascript://x", Wall: clubv1.Wall_WALL_OPEN, AdminsDisplay: clubv1.AdminsDisplay_ADMINS_DISPLAY_CREATOR})
			return err
		}, codes.InvalidArgument},
		{"a bad address", func() error {
			_, err := s.client.UpdateClub(s.as(t, ann), &clubv1.UpdateClubRequest{ClubId: id, Name: "x", Address: "2cats", Wall: clubv1.Wall_WALL_OPEN, AdminsDisplay: clubv1.AdminsDisplay_ADMINS_DISPLAY_CREATOR})
			return err
		}, codes.InvalidArgument},
		{"a member takes the club", func() error {
			_, err := s.client.TransferClub(s.as(t, bob), &clubv1.TransferClubRequest{ClubId: id, UserId: bob.String()})
			return err
		}, codes.PermissionDenied},
		{"a stranger deletes the club", func() error {
			_, err := s.client.DeleteClub(s.as(t, bob), &clubv1.DeleteClubRequest{ClubId: id})
			return err
		}, codes.PermissionDenied},
		{"delete a bad club id", func() error {
			_, err := s.client.DeleteClub(s.as(t, ann), &clubv1.DeleteClubRequest{ClubId: "x"})
			return err
		}, codes.InvalidArgument},
		{"hand it over to a stranger", func() error {
			_, err := s.client.TransferClub(s.as(t, ann), &clubv1.TransferClubRequest{ClubId: id, UserId: bob.String()})
			return err
		}, codes.FailedPrecondition},
		{"the clubs of a bad user id", func() error {
			_, err := s.client.ListUserClubs(context.Background(), &clubv1.ListUserClubsRequest{UserId: "x"})
			return err
		}, codes.InvalidArgument},
		{"the owner leaves", func() error {
			_, err := s.client.LeaveClub(s.as(t, ann), &clubv1.LeaveClubRequest{ClubId: id})
			return err
		}, codes.FailedPrecondition},
		{"make someone the owner", func() error {
			_, err := s.client.SetMemberRole(s.as(t, ann), &clubv1.SetMemberRoleRequest{ClubId: id, UserId: bob.String(), Role: clubv1.Role_ROLE_OWNER})
			return err
		}, codes.InvalidArgument},
		{"appoint a stranger", func() error {
			_, err := s.client.SetMemberRole(s.as(t, ann), &clubv1.SetMemberRoleRequest{ClubId: id, UserId: bob.String(), Role: clubv1.Role_ROLE_ADMIN})
			return err
		}, codes.NotFound},
		{"bad club id", func() error {
			_, err := s.client.JoinClub(s.as(t, ann), &clubv1.JoinClubRequest{ClubId: "x"})
			return err
		}, codes.InvalidArgument},
		{"join an unknown club", func() error {
			_, err := s.client.JoinClub(s.as(t, ann), &clubv1.JoinClubRequest{ClubId: uuid.NewString()})
			return err
		}, codes.NotFound},
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

func TestDeleteClub(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann := uuid.New()

	created, err := s.client.CreateClub(s.as(t, ann), &clubv1.CreateClubRequest{Name: "Кошки"})
	if err != nil {
		t.Fatalf("CreateClub: %v", err)
	}
	id := created.GetClub().GetClub().GetId()

	if _, err = s.client.DeleteClub(s.as(t, ann), &clubv1.DeleteClubRequest{ClubId: id}); err != nil {
		t.Fatalf("DeleteClub: %v", err)
	}
	if _, err = s.client.GetClub(context.Background(), &clubv1.GetClubRequest{Ref: id}); status.Code(err) != codes.NotFound {
		t.Fatalf("GetClub after DeleteClub = %v", err)
	}
}

func TestSetClubVerified(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	created, err := s.client.CreateClub(s.as(t, uuid.New()), &clubv1.CreateClubRequest{Name: "Кошки"})
	if err != nil {
		t.Fatalf("CreateClub: %v", err)
	}
	id := created.GetClub().GetClub().GetId()

	// Internal: the gateway calls it without a token of a user.
	if _, err = s.client.SetClubVerified(context.Background(), &clubv1.SetClubVerifiedRequest{ClubId: id, Verified: true}); err != nil {
		t.Fatalf("SetClubVerified: %v", err)
	}
	got, err := s.client.GetClubs(context.Background(), &clubv1.GetClubsRequest{Ids: []string{id}})
	if err != nil || len(got.GetClubs()) != 1 || !got.GetClubs()[0].GetVerified() {
		t.Fatalf("GetClubs = %v, %v", got, err)
	}

	found, err := s.client.SearchClubs(context.Background(), &clubv1.SearchClubsRequest{Verified: true})
	if err != nil || len(found.GetClubs()) != 1 {
		t.Fatalf("SearchClubs of the verified = %v, %v", found, err)
	}
	if _, err = s.client.SearchClubs(context.Background(), &clubv1.SearchClubsRequest{}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("SearchClubs without a query = %v", err)
	}

	for _, req := range []*clubv1.SetClubVerifiedRequest{{ClubId: "x"}, {ClubId: uuid.NewString()}} {
		want := map[bool]codes.Code{true: codes.InvalidArgument, false: codes.NotFound}[req.GetClubId() == "x"]
		if _, err = s.client.SetClubVerified(context.Background(), req); status.Code(err) != want {
			t.Fatalf("SetClubVerified(%q) = %v, want %v", req.GetClubId(), err, want)
		}
	}
}

func TestSetClubBackdrop(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, bob := uuid.New(), uuid.New()

	created, err := s.client.CreateClub(s.as(t, ann), &clubv1.CreateClubRequest{Name: "Кошки"})
	if err != nil {
		t.Fatal(err)
	}
	id, photo := created.GetClub().GetClub().GetId(), uuid.NewString()

	set, err := s.client.SetClubBackdrop(s.as(t, ann), &clubv1.SetClubBackdropRequest{ClubId: id, Side: clubv1.BackdropSide_BACKDROP_SIDE_RIGHT, PhotoId: photo})
	if err != nil || set.GetClub().GetClub().GetBackdropRight() != photo || set.GetClub().GetClub().GetBackdropLeft() != "" {
		t.Fatalf("SetClubBackdrop = %v, %v", set, err)
	}
	if _, err = s.client.SetClubBackdrop(s.as(t, bob), &clubv1.SetClubBackdropRequest{ClubId: id, Side: clubv1.BackdropSide_BACKDROP_SIDE_LEFT, PhotoId: photo}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("a stranger: %v", err)
	}
	if _, err = s.client.SetClubBackdrop(s.as(t, ann), &clubv1.SetClubBackdropRequest{ClubId: id, PhotoId: photo}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("no side: %v", err)
	}
}

func TestCreateStateClub(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	state, ruler := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	req := &clubv1.CreateStateClubRequest{StateId: state.String(), OwnerId: ruler.String(), Name: "Бореалия", Description: "Группа государства"}

	// Internal: the world service calls it without a token of a user, and
	// asked twice, it makes one group.
	for range 2 {
		got, err := s.client.CreateStateClub(context.Background(), req)
		if err != nil || got.GetClubId() != state.String() {
			t.Fatalf("CreateStateClub = %v, %v", got, err)
		}
	}

	got, err := s.client.GetClub(s.as(t, ruler), &clubv1.GetClubRequest{Ref: state.String()})
	if err != nil || got.GetClub().GetClub().GetName() != "Бореалия" || got.GetClub().GetRole() != clubv1.Role_ROLE_OWNER {
		t.Fatalf("GetClub = %v, %v", got, err)
	}

	for _, bad := range []*clubv1.CreateStateClubRequest{
		{StateId: "x", OwnerId: ruler.String(), Name: "Бореалия"},
		{StateId: state.String(), OwnerId: "x", Name: "Бореалия"},
		{StateId: uuid.NewString(), OwnerId: ruler.String()},
	} {
		if _, err = s.client.CreateStateClub(context.Background(), bad); status.Code(err) != codes.InvalidArgument {
			t.Errorf("CreateStateClub(%v) = %v", bad, err)
		}
	}
}
