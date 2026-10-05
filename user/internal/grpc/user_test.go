package grpc_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/jwtauth"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	usergrpc "github.com/vexner67/freenet/user/internal/grpc"
	"github.com/vexner67/freenet/user/internal/inmem"
	"github.com/vexner67/freenet/user/internal/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const issuer = "freenet-test"

type testServer struct {
	client userv1.UserServiceClient
	key    *rsa.PrivateKey
}

// newServer wires the handler the same way app.New does.
func newServer(t *testing.T) testServer {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	users := inmem.NewUserRepository()
	follows := inmem.NewFollowRepository(users)

	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(
		usergrpc.ErrorInterceptor(log),
		jwtauth.UnaryServerInterceptor(
			jwtauth.NewVerifier(&key.PublicKey, issuer),
			usergrpc.PublicMethods()...,
		),
	))
	userv1.RegisterUserServiceServer(srv, usergrpc.NewUserHandler(
		service.NewUserService(users),
		service.NewFollowService(users, follows, follows),
		log,
	))

	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("grpc.NewClient: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	return testServer{
		client: userv1.NewUserServiceClient(conn),
		key:    key,
	}
}

// signIn registers the email like auth does and returns a context
// carrying an access token for that user.
func (s testServer) signIn(t *testing.T, email string) context.Context {
	t.Helper()

	resp, err := s.client.GetOrCreateByEmail(context.Background(), &userv1.GetOrCreateByEmailRequest{Email: email})
	if err != nil {
		t.Fatalf("GetOrCreateByEmail: %v", err)
	}

	token, err := jwt.NewWithClaims(jwt.SigningMethodRS256, jwtauth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   resp.GetUserId(),
			Issuer:    issuer,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		},
		TokenType: jwtauth.AccessToken,
		SessionID: uuid.NewString(),
	}).SignedString(s.key)
	if err != nil {
		t.Fatalf("SignedString: %v", err)
	}

	return metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+token)
}

func TestUserService_GetOrCreateByEmail(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ctx := context.Background()

	first, err := s.client.GetOrCreateByEmail(ctx, &userv1.GetOrCreateByEmailRequest{Email: "ann@example.com"})
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if _, err = uuid.Parse(first.GetUserId()); err != nil {
		t.Fatalf("user_id %q is not a uuid: %v", first.GetUserId(), err)
	}

	second, err := s.client.GetOrCreateByEmail(ctx, &userv1.GetOrCreateByEmailRequest{Email: "ann@example.com"})
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if second.GetUserId() != first.GetUserId() {
		t.Fatalf("id changed: %s != %s", second.GetUserId(), first.GetUserId())
	}

	_, err = s.client.GetOrCreateByEmail(ctx, &userv1.GetOrCreateByEmailRequest{Email: "not an email"})
	if got := status.Code(err); got != codes.InvalidArgument {
		t.Fatalf("invalid email: code = %v, want InvalidArgument", got)
	}
}

// TestProfileFlow walks the scenario a client goes through: sign in,
// see an empty profile, fill it in, and be found by others.
func TestProfileFlow(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann := s.signIn(t, "ann@example.com")

	me, err := s.client.GetMe(ann, &userv1.GetMeRequest{})
	if err != nil {
		t.Fatalf("GetMe: %v", err)
	}
	if me.GetUser().GetProfileCompleted() || me.GetUser().GetEmail() != "ann@example.com" {
		t.Fatalf("fresh user: %+v", me.GetUser())
	}

	updated, err := s.client.UpdateProfile(ann, &userv1.UpdateProfileRequest{
		Name:           "Ann",
		Nickname:       "ann",
		Status:         "hello",
		Gender:         userv1.Gender_GENDER_FEMALE,
		Worldview:      userv1.Worldview_WORLDVIEW_BUDDHISM,
		PoliticalViews: userv1.PoliticalViews_POLITICAL_VIEWS_LIBERAL,
	})
	if err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	if !updated.GetUser().GetProfileCompleted() || updated.GetUser().GetNickname() != "ann" {
		t.Fatalf("after update: %+v", updated.GetUser())
	}

	me, err = s.client.GetMe(ann, &userv1.GetMeRequest{})
	if err != nil {
		t.Fatalf("GetMe after update: %v", err)
	}
	if me.GetUser().GetWorldview() != userv1.Worldview_WORLDVIEW_BUDDHISM {
		t.Fatalf("owner must see the whole profile: %+v", me.GetUser())
	}

	// Anyone, even without a token, sees only the public part.
	public, err := s.client.GetProfile(context.Background(), &userv1.GetProfileRequest{Nickname: "ANN"})
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	if p := public.GetProfile(); p.GetId() != me.GetUser().GetId() || p.GetName() != "Ann" || p.GetGender() != userv1.Gender_GENDER_FEMALE {
		t.Fatalf("public profile: %+v", p)
	}

	bob := s.signIn(t, "bob@example.com")
	_, err = s.client.UpdateProfile(bob, &userv1.UpdateProfileRequest{Name: "Bob", Nickname: "Ann"})
	if got := status.Code(err); got != codes.AlreadyExists {
		t.Fatalf("taken nickname: code = %v, want AlreadyExists", got)
	}
}

func TestPrivacyFlow(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann := s.signIn(t, "ann@example.com")

	_, err := s.client.UpdateProfile(ann, &userv1.UpdateProfileRequest{
		Name:           "Ann",
		Nickname:       "ann",
		Relationship:   userv1.Relationship_RELATIONSHIP_MARRIED,
		Worldview:      userv1.Worldview_WORLDVIEW_BUDDHISM,
		PoliticalViews: userv1.PoliticalViews_POLITICAL_VIEWS_LIBERAL,
	})
	if err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}

	public, err := s.client.GetProfile(context.Background(), &userv1.GetProfileRequest{Nickname: "ann"})
	if err != nil {
		t.Fatal(err)
	}
	if p := public.GetProfile(); p.GetWorldview() != userv1.Worldview_WORLDVIEW_NONE ||
		p.GetRelationship() != userv1.Relationship_RELATIONSHIP_NONE {
		t.Fatalf("sensitive fields must be hidden by default: %+v", p)
	}

	updated, err := s.client.UpdatePrivacy(ann, &userv1.UpdatePrivacyRequest{
		Privacy: &userv1.PrivacySettings{ShowWorldview: true},
	})
	if err != nil {
		t.Fatalf("UpdatePrivacy: %v", err)
	}
	if !updated.GetUser().GetPrivacy().GetShowWorldview() {
		t.Fatalf("settings not returned: %+v", updated.GetUser().GetPrivacy())
	}

	public, err = s.client.GetProfile(context.Background(), &userv1.GetProfileRequest{Nickname: "ann"})
	if err != nil {
		t.Fatal(err)
	}
	if p := public.GetProfile(); p.GetWorldview() != userv1.Worldview_WORLDVIEW_BUDDHISM ||
		p.GetPoliticalViews() != userv1.PoliticalViews_POLITICAL_VIEWS_NONE {
		t.Fatalf("only the opened field must become visible: %+v", p)
	}

	_, err = s.client.UpdatePrivacy(context.Background(), &userv1.UpdatePrivacyRequest{})
	if got := status.Code(err); got != codes.Unauthenticated {
		t.Fatalf("UpdatePrivacy without token: code = %v", got)
	}
}

func TestProfileErrors(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann := s.signIn(t, "ann@example.com")

	tests := []struct {
		name string
		call func() error
		want codes.Code
	}{
		{
			name: "GetMe without token",
			call: func() error {
				_, err := s.client.GetMe(context.Background(), &userv1.GetMeRequest{})
				return err
			},
			want: codes.Unauthenticated,
		},
		{
			name: "UpdateProfile without token",
			call: func() error {
				_, err := s.client.UpdateProfile(context.Background(), &userv1.UpdateProfileRequest{Name: "A", Nickname: "a"})
				return err
			},
			want: codes.Unauthenticated,
		},
		{
			name: "UpdateProfile without nickname",
			call: func() error {
				_, err := s.client.UpdateProfile(ann, &userv1.UpdateProfileRequest{Name: "Ann"})
				return err
			},
			want: codes.InvalidArgument,
		},
		{
			name: "UpdateProfile with invalid nickname",
			call: func() error {
				_, err := s.client.UpdateProfile(ann, &userv1.UpdateProfileRequest{Name: "Ann", Nickname: "1 bad"})
				return err
			},
			want: codes.InvalidArgument,
		},
		{
			name: "GetProfile of unknown nickname",
			call: func() error {
				_, err := s.client.GetProfile(context.Background(), &userv1.GetProfileRequest{Nickname: "nobody"})
				return err
			},
			want: codes.NotFound,
		},
		{
			name: "GetProfile with invalid nickname",
			call: func() error {
				_, err := s.client.GetProfile(context.Background(), &userv1.GetProfileRequest{Nickname: ""})
				return err
			},
			want: codes.InvalidArgument,
		},
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
