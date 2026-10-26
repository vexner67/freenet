package grpc

import (
	"context"
	"math"
	"strings"
	"unicode"

	authv1 "github.com/vexner67/freenet/auth/api/auth/v1"
	"google.golang.org/protobuf/types/known/emptypb"
)

// maxAccountLen is as much of the page's name as the app is told.
const maxAccountLen = 64

func (h *AuthHandler) VerifyTwoFactor(ctx context.Context, req *authv1.VerifyTwoFactorRequest) (*authv1.TokenPair, error) {
	pair, err := h.authService.VerifySecondStep(ctx, req.GetTicket(), req.GetCode(), device(ctx))
	if err != nil {
		return nil, err
	}

	return toProtoTokenPair(pair), nil
}

func (h *AuthHandler) GetTwoFactor(ctx context.Context, _ *authv1.GetTwoFactorRequest) (*authv1.GetTwoFactorResponse, error) {
	identity, err := caller(ctx)
	if err != nil {
		return nil, err
	}

	state, err := h.twoFactor.State(ctx, identity.UserID)
	if err != nil {
		return nil, err
	}

	return &authv1.GetTwoFactorResponse{
		Enabled:      state.Enabled,
		RecoveryLeft: int32(min(state.RecoveryLeft, math.MaxInt32)), //nolint:gosec // bounded above
	}, nil
}

func (h *AuthHandler) SetUpTwoFactor(ctx context.Context, req *authv1.SetUpTwoFactorRequest) (*authv1.SetUpTwoFactorResponse, error) {
	identity, err := caller(ctx)
	if err != nil {
		return nil, err
	}

	setup, err := h.twoFactor.SetUp(ctx, identity.UserID, accountName(req.GetAccount()))
	if err != nil {
		return nil, err
	}

	return &authv1.SetUpTwoFactorResponse{Secret: setup.Secret, Uri: setup.URI}, nil
}

func (h *AuthHandler) EnableTwoFactor(ctx context.Context, req *authv1.EnableTwoFactorRequest) (*authv1.EnableTwoFactorResponse, error) {
	identity, err := caller(ctx)
	if err != nil {
		return nil, err
	}

	codes, err := h.twoFactor.Enable(ctx, identity.UserID, req.GetCode())
	if err != nil {
		return nil, err
	}

	return &authv1.EnableTwoFactorResponse{RecoveryCodes: codes}, nil
}

func (h *AuthHandler) DisableTwoFactor(ctx context.Context, req *authv1.DisableTwoFactorRequest) (*emptypb.Empty, error) {
	identity, err := caller(ctx)
	if err != nil {
		return nil, err
	}

	if err = h.twoFactor.Disable(ctx, identity.UserID, req.GetCode()); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *AuthHandler) RenewRecoveryCodes(ctx context.Context, req *authv1.RenewRecoveryCodesRequest) (*authv1.RenewRecoveryCodesResponse, error) {
	identity, err := caller(ctx)
	if err != nil {
		return nil, err
	}

	codes, err := h.twoFactor.RenewRecovery(ctx, identity.UserID, req.GetCode())
	if err != nil {
		return nil, err
	}

	return &authv1.RenewRecoveryCodesResponse{RecoveryCodes: codes}, nil
}

// accountName is the name of the page in the app: printable, one line, at
// most maxAccountLen letters; "Freenet" alone without one. A colon would
// split it from the site's name in the label.
func accountName(s string) string {
	var b strings.Builder
	n := 0
	for _, r := range strings.TrimSpace(s) {
		if n == maxAccountLen {
			break
		}
		if unicode.IsPrint(r) && r != ':' {
			b.WriteRune(r)
			n++
		}
	}

	if name := strings.TrimSpace(b.String()); name != "" {
		return name
	}

	return "freenet"
}
