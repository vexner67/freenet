package grpc

import (
	"context"
	"log/slog"
	"math"
	"net"
	"net/netip"

	"github.com/google/uuid"
	authv1 "github.com/vexner67/freenet/auth/api/auth/v1"
	"github.com/vexner67/freenet/auth/internal/account"
	"github.com/vexner67/freenet/auth/internal/service"
	"github.com/vexner67/freenet/auth/internal/session"
	"github.com/vexner67/freenet/pkg/jwtauth"
	"github.com/vexner67/freenet/pkg/mtls"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type AuthService interface {
	RequestCode(ctx context.Context, email account.Email, clientIP string) error
	VerifyCode(ctx context.Context, email account.Email, code string, device session.Device) (service.TokenPair, error)
	RefreshToken(ctx context.Context, refreshToken string, device session.Device) (service.TokenPair, error)
	Logout(ctx context.Context, refreshToken string) error
}

type SessionService interface {
	List(ctx context.Context, userID uuid.UUID) ([]*session.Session, error)
	End(ctx context.Context, userID, sessionID uuid.UUID) error
	EndOthers(ctx context.Context, userID, current uuid.UUID) (int, error)
}

type AuthHandler struct {
	authv1.UnimplementedAuthServiceServer
	authService    AuthService
	sessionService SessionService
	logger         *slog.Logger
}

func NewAuthHandler(svc AuthService, sessions SessionService, logger *slog.Logger) *AuthHandler {
	return &AuthHandler{
		authService:    svc,
		sessionService: sessions,
		logger:         logger,
	}
}

func (h *AuthHandler) RequestCode(ctx context.Context, req *authv1.RequestCodeRequest) (*emptypb.Empty, error) {
	email, err := account.NewEmail(req.GetEmail())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if err = h.authService.RequestCode(ctx, email, clientIP(ctx)); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *AuthHandler) VerifyCode(ctx context.Context, req *authv1.VerifyCodeRequest) (*authv1.TokenPair, error) {
	email, err := account.NewEmail(req.GetEmail())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	tokenPair, err := h.authService.VerifyCode(ctx, email, req.GetCode(), device(ctx))
	if err != nil {
		return nil, err
	}

	return toProtoTokenPair(tokenPair), nil
}

func (h *AuthHandler) RefreshToken(ctx context.Context, req *authv1.RefreshTokenRequest) (*authv1.TokenPair, error) {
	tokenPair, err := h.authService.RefreshToken(ctx, req.GetRefreshToken(), device(ctx))
	if err != nil {
		return nil, err
	}

	return toProtoTokenPair(tokenPair), nil
}

func (h *AuthHandler) Logout(ctx context.Context, req *authv1.LogoutRequest) (*emptypb.Empty, error) {
	if err := h.authService.Logout(ctx, req.GetRefreshToken()); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *AuthHandler) ListSessions(ctx context.Context, _ *authv1.ListSessionsRequest) (*authv1.ListSessionsResponse, error) {
	identity, err := caller(ctx)
	if err != nil {
		return nil, err
	}

	sessions, err := h.sessionService.List(ctx, identity.UserID)
	if err != nil {
		return nil, err
	}

	return &authv1.ListSessionsResponse{Sessions: toProtoSessions(sessions, identity.SessionID)}, nil
}

func (h *AuthHandler) EndSession(ctx context.Context, req *authv1.EndSessionRequest) (*emptypb.Empty, error) {
	identity, err := caller(ctx)
	if err != nil {
		return nil, err
	}

	id, err := uuid.Parse(req.GetSessionId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid session id")
	}

	if err = h.sessionService.End(ctx, identity.UserID, id); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *AuthHandler) EndOtherSessions(ctx context.Context, _ *authv1.EndOtherSessionsRequest) (*authv1.EndOtherSessionsResponse, error) {
	identity, err := caller(ctx)
	if err != nil {
		return nil, err
	}

	ended, err := h.sessionService.EndOthers(ctx, identity.UserID, identity.SessionID)
	if err != nil {
		return nil, err
	}

	return &authv1.EndOtherSessionsResponse{Ended: int32(min(ended, math.MaxInt32))}, nil
}

// caller is who the verified access token says is calling, set by
// jwtauth.UnaryServerInterceptor; never trust an ID from the request body.
func caller(ctx context.Context) (jwtauth.Identity, error) {
	identity, ok := jwtauth.IdentityFromContext(ctx)
	if !ok {
		return jwtauth.Identity{}, status.Error(codes.Unauthenticated, "missing access token")
	}

	return identity, nil
}

// device is where the end user signs in or refreshes from. As with the
// address, only the gateway may say what the browser calls itself.
func device(ctx context.Context) session.Device {
	agent := ""
	if identity, ok := mtls.PeerIdentity(ctx); ok && identity == mtls.Gateway {
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if values := md.Get(authv1.UserAgentMetadata); len(values) == 1 {
				agent = values[0]
			}
		}
	}

	return session.NewDevice(clientIP(ctx), agent)
}

// clientIP is the end user's address for rate limiting. Only the gateway,
// proven by its mTLS certificate, may state it in metadata; for anyone
// else, metadata could be forged, so the connection address is used.
func clientIP(ctx context.Context) string {
	if identity, ok := mtls.PeerIdentity(ctx); ok && identity == mtls.Gateway {
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if values := md.Get(authv1.ClientIPMetadata); len(values) == 1 {
				if addr, err := netip.ParseAddr(values[0]); err == nil {
					return addr.String()
				}
			}
		}
	}

	p, ok := peer.FromContext(ctx)
	if !ok || p.Addr == nil {
		return ""
	}

	host, _, err := net.SplitHostPort(p.Addr.String())
	if err != nil {
		return ""
	}

	return host
}
