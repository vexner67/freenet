package grpc

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	authv1 "github.com/vexner67/freenet/auth/api/auth/v1"
	"github.com/vexner67/freenet/auth/internal/service"
	"github.com/vexner67/freenet/auth/internal/session"
	"github.com/vexner67/freenet/pkg/jwtauth"
	"github.com/vexner67/freenet/pkg/mtls"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestDevice(t *testing.T) {
	t.Parallel()

	withAgent := func(ctx context.Context, agent string) context.Context {
		md, _ := metadata.FromIncomingContext(ctx)
		return metadata.NewIncomingContext(ctx, metadata.Join(md, metadata.Pairs(authv1.UserAgentMetadata, agent)))
	}

	tests := []struct {
		name      string
		identity  string
		wantIP    string
		wantAgent string
	}{
		{"gateway states the device", mtls.Gateway, "198.51.100.7", "Mozilla/5.0 (test)"},
		{"another service cannot state it", mtls.User, "10.0.0.2", ""},
		{"without mTLS metadata is ignored", "", "10.0.0.2", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			d := device(withAgent(callFrom(tt.identity, "198.51.100.7"), "Mozilla/5.0 (test)"))
			if d.IP() != tt.wantIP || d.Agent() != tt.wantAgent {
				t.Fatalf("device = %q, %q; want %q, %q", d.IP(), d.Agent(), tt.wantIP, tt.wantAgent)
			}
		})
	}
}

// fakeSessions is one user's sessions.
type fakeSessions struct {
	userID   uuid.UUID
	sessions []*session.Session
	ended    []uuid.UUID
	kept     uuid.UUID
}

func (f *fakeSessions) List(_ context.Context, userID uuid.UUID) ([]*session.Session, error) {
	if userID != f.userID {
		return nil, nil
	}

	return f.sessions, nil
}

func (f *fakeSessions) End(_ context.Context, userID, sessionID uuid.UUID) error {
	for _, s := range f.sessions {
		if userID == f.userID && s.ID() == sessionID {
			f.ended = append(f.ended, sessionID)
			return nil
		}
	}

	return service.ErrNoSuchSession
}

func (f *fakeSessions) EndOthers(_ context.Context, _, current uuid.UUID) (int, error) {
	f.kept = current
	return len(f.sessions) - 1, nil
}

func TestAuthHandler_Sessions(t *testing.T) {
	t.Parallel()

	ann := uuid.New()
	signedIn := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

	var list []*session.Session
	for i, agent := range []string{"laptop", "phone"} {
		s, err := session.New(uuid.New(), ann, "secret-hash", session.NewDevice("203.0.113.7", agent), signedIn.Add(time.Duration(i)*time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		list = append(list, s)
	}
	laptop, phone := list[0], list[1]

	fake := &fakeSessions{userID: ann, sessions: list}
	h := NewAuthHandler(nil, nil, fake, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := jwtauth.ContextWithIdentity(context.Background(), jwtauth.Identity{UserID: ann, SessionID: phone.ID()})

	resp, err := h.ListSessions(ctx, &authv1.ListSessionsRequest{})
	if err != nil || len(resp.GetSessions()) != 2 {
		t.Fatalf("ListSessions = %v, %v", resp, err)
	}
	first, second := resp.GetSessions()[0], resp.GetSessions()[1]
	if first.GetId() != laptop.ID().String() || first.GetCurrent() || first.GetUserAgent() != "laptop" || first.GetIp() != "203.0.113.7" {
		t.Fatalf("the laptop = %v", first)
	}
	if !first.GetCreatedAt().AsTime().Equal(signedIn) || !first.GetLastUsedAt().AsTime().Equal(signedIn) {
		t.Fatalf("the times of the laptop = %v, %v", first.GetCreatedAt().AsTime(), first.GetLastUsedAt().AsTime())
	}
	if second.GetId() != phone.ID().String() || !second.GetCurrent() {
		t.Fatalf("the phone, which asks = %v", second)
	}

	if _, err = h.ListSessions(context.Background(), &authv1.ListSessionsRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("ListSessions without a token: %v", err)
	}

	if _, err = h.EndSession(ctx, &authv1.EndSessionRequest{SessionId: laptop.ID().String()}); err != nil || len(fake.ended) != 1 || fake.ended[0] != laptop.ID() {
		t.Fatalf("EndSession: %v, ended %v", err, fake.ended)
	}
	if _, err = h.EndSession(ctx, &authv1.EndSessionRequest{SessionId: "not-an-id"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("EndSession of a bad ID: %v", err)
	}
	if _, err = h.EndSession(ctx, &authv1.EndSessionRequest{SessionId: uuid.NewString()}); err == nil {
		t.Fatal("EndSession of someone else's session succeeded")
	} else if st, ok := toStatus(err); !ok || st.Code() != codes.NotFound {
		t.Fatalf("EndSession of someone else's session: %v", err)
	}

	others, err := h.EndOtherSessions(ctx, &authv1.EndOtherSessionsRequest{})
	if err != nil || others.GetEnded() != 1 || fake.kept != phone.ID() {
		t.Fatalf("EndOtherSessions = %v, %v; kept %v", others, err, fake.kept)
	}
}

// TestPublicMethods: signing in needs no access token, sessions do.
func TestPublicMethods(t *testing.T) {
	t.Parallel()

	public := PublicMethods()
	for _, method := range []string{
		authv1.AuthService_ListSessions_FullMethodName,
		authv1.AuthService_EndSession_FullMethodName,
		authv1.AuthService_EndOtherSessions_FullMethodName,
		authv1.AuthService_GetTwoFactor_FullMethodName,
		authv1.AuthService_SetUpTwoFactor_FullMethodName,
		authv1.AuthService_EnableTwoFactor_FullMethodName,
		authv1.AuthService_DisableTwoFactor_FullMethodName,
		authv1.AuthService_RenewRecoveryCodes_FullMethodName,
	} {
		for _, p := range public {
			if p == method {
				t.Fatalf("%s is open without an access token", method)
			}
		}
	}
	if len(public) != 6 {
		t.Fatalf("PublicMethods = %v", public)
	}
}
