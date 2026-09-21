package grpc

import (
	"context"
	"log/slog"

	authv1 "github.com/vexner67/freenet/auth/api/auth/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type AuthService interface {
	RequestCode(context.Context, string) error
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
	if err := h.authService.RequestCode(ctx, req.GetEmail()); err != nil {
		return nil, status.Error(codes.InvalidArgument, "failed to request code")
	}

	return nil, nil
}
