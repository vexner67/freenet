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
	"google.golang.org/protobuf/types/known/timestamppb"
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

func TestEvents(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, bob := uuid.New(), uuid.New()
	start := time.Now().Add(time.Hour)

	if _, err := s.client.CreateEvent(s.as(t, ann), &clubv1.CreateEventRequest{Name: "Новый год"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("an event with no start: %v", err)
	}
	if _, err := s.client.CreateEvent(s.as(t, ann), &clubv1.CreateEventRequest{
		Name: "Новый год", StartsAt: timestamppb.New(start), EndsAt: timestamppb.New(start.Add(-time.Minute)),
	}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("an event ending before it starts: %v", err)
	}
	if _, err := s.client.CreateEvent(context.Background(), &clubv1.CreateEventRequest{Name: "Новый год", StartsAt: timestamppb.New(start)}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("CreateEvent without a token: %v", err)
	}

	created, err := s.client.CreateEvent(s.as(t, ann), &clubv1.CreateEventRequest{Name: "Новый год", Description: "ёлка", StartsAt: timestamppb.New(start)})
	if err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}
	event := created.GetClub().GetClub()
	if event.GetKind() != clubv1.Kind_KIND_EVENT || event.GetStartsAt().AsTime().Unix() != start.Unix() || event.GetEndsAt() != nil ||
		created.GetClub().GetRole() != clubv1.Role_ROLE_OWNER {
		t.Fatalf("CreateEvent = %v", created)
	}
	id := event.GetId()

	joined, err := s.client.JoinClub(s.as(t, bob), &clubv1.JoinClubRequest{ClubId: id, Maybe: true})
	if err != nil || !joined.GetClub().GetMaybe() || joined.GetClub().GetClub().GetMemberCount() != 2 || joined.GetClub().GetClub().GetMaybeCount() != 1 {
		t.Fatalf("JoinClub as a maybe = %v, %v", joined, err)
	}
	// Joining again changes the answer.
	if joined, err = s.client.JoinClub(s.as(t, bob), &clubv1.JoinClubRequest{ClubId: id}); err != nil || joined.GetClub().GetMaybe() || joined.GetClub().GetClub().GetMaybeCount() != 0 {
		t.Fatalf("a maybe now sure = %v, %v", joined, err)
	}
	if joined, err = s.client.JoinClub(s.as(t, bob), &clubv1.JoinClubRequest{ClubId: id, Maybe: true}); err != nil || !joined.GetClub().GetMaybe() || joined.GetClub().GetClub().GetMaybeCount() != 1 {
		t.Fatalf("a sure one now a maybe = %v, %v", joined, err)
	}

	sure, err := s.client.ListMembers(context.Background(), &clubv1.ListMembersRequest{ClubId: id, Going: clubv1.Going_GOING_SURE})
	if err != nil || sure.GetTotal() != 1 || len(sure.GetMembers()) != 1 || sure.GetMembers()[0].GetUserId() != ann.String() {
		t.Fatalf("sure = %v, %v", sure, err)
	}
	maybe, err := s.client.ListMembers(context.Background(), &clubv1.ListMembersRequest{ClubId: id, Going: clubv1.Going_GOING_MAYBE})
	if err != nil || maybe.GetTotal() != 1 || len(maybe.GetMembers()) != 1 || !maybe.GetMembers()[0].GetMaybe() {
		t.Fatalf("maybe = %v, %v", maybe, err)
	}

	group, err := s.client.CreateClub(s.as(t, ann), &clubv1.CreateClubRequest{Name: "Кошки"})
	if err != nil || group.GetClub().GetClub().GetKind() != clubv1.Kind_KIND_GROUP || group.GetClub().GetClub().GetStartsAt() != nil {
		t.Fatalf("CreateClub = %v, %v", group, err)
	}

	if n, err := s.client.CountClubs(context.Background(), &clubv1.CountClubsRequest{}); err != nil || n.GetCount() != 1 {
		t.Fatalf("CountClubs = %v, %v; the group, not the event", n, err)
	}

	events, err := s.client.ListMyClubs(s.as(t, ann), &clubv1.ListMyClubsRequest{Kind: clubv1.Kind_KIND_EVENT})
	if err != nil || events.GetTotal() != 1 || events.GetClubs()[0].GetClub().GetId() != id {
		t.Fatalf("my events = %v, %v", events, err)
	}
	groups, err := s.client.ListUserClubs(context.Background(), &clubv1.ListUserClubsRequest{UserId: ann.String(), Kind: clubv1.Kind_KIND_GROUP})
	if err != nil || groups.GetTotal() != 1 || groups.GetClubs()[0].GetKind() != clubv1.Kind_KIND_GROUP {
		t.Fatalf("their groups = %v, %v", groups, err)
	}
	if all, err := s.client.ListMyClubs(s.as(t, ann), &clubv1.ListMyClubsRequest{}); err != nil || all.GetTotal() != 2 {
		t.Fatalf("all my clubs = %v, %v", all, err)
	}

	// The event moves with its settings; without a start it is refused.
	update := &clubv1.UpdateClubRequest{
		ClubId: id, Name: "Новый год", Wall: clubv1.Wall_WALL_OPEN, AdminsDisplay: clubv1.AdminsDisplay_ADMINS_DISPLAY_CREATOR,
		StartsAt: timestamppb.New(start.Add(time.Hour)), EndsAt: timestamppb.New(start.Add(2 * time.Hour)),
	}
	moved, err := s.client.UpdateClub(s.as(t, ann), update)
	if err != nil || moved.GetClub().GetClub().GetEndsAt().AsTime().Unix() != start.Add(2*time.Hour).Unix() {
		t.Fatalf("UpdateClub = %v, %v", moved, err)
	}
	update.StartsAt, update.EndsAt = nil, nil
	if _, err = s.client.UpdateClub(s.as(t, ann), update); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("an event updated to no start: %v", err)
	}
}
