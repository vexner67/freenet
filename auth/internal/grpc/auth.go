package grpc

import (
	"context"
	"log/slog"
	"net"
	"net/netip"

	authv1 "github.com/vexner67/freenet/auth/api/auth/v1"
	"github.com/vexner67/freenet/auth/internal/account"
	"github.com/vexner67/freenet/auth/internal/service"
	"github.com/vexner67/freenet/pkg/mtls"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type AuthService interface {
	RequestCode(ctx context.Context, email account.Email, clientIP string) error
	VerifyCode(ctx context.Context, email account.Email, code string) (service.TokenPair, error)
	RefreshToken(ctx context.Context, refreshToken string) (service.TokenPair, error)
	Logout(ctx context.Context, refreshToken string) error
}

type AuthHandler struct {
	authv1.UnimplementedAuthServiceServer
	authService AuthService
	logger      *slog.Logger
}

func NewAuthHandler(svc AuthService, logger *slog.Logger) *AuthHandler {
	return &AuthHandler{
		authService: svc,
		logger:      logger,
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

	tokenPair, err := h.authService.VerifyCode(ctx, email, req.GetCode())
	if err != nil {
		return nil, err
	}

	return toProtoTokenPair(tokenPair), nil
}

func (h *AuthHandler) RefreshToken(ctx context.Context, req *authv1.RefreshTokenRequest) (*authv1.TokenPair, error) {
	tokenPair, err := h.authService.RefreshToken(ctx, req.GetRefreshToken())
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

func toProtoTokenPair(tokenPair service.TokenPair) *authv1.TokenPair {
	return &authv1.TokenPair{
		AccessToken:  tokenPair.Access,
		RefreshToken: tokenPair.Refresh,
	}
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
