package backend

import (
	"context"

	authv1 "github.com/vexner67/freenet/auth/api/auth/v1"
	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/pkg/errs"
	"google.golang.org/grpc/metadata"
)

type Auth struct {
	api authv1.AuthServiceClient
}

func NewAuth(api authv1.AuthServiceClient) *Auth {
	return &Auth{
		api: api,
	}
}

// RequestCode passes the end user's address, which auth trusts only from
// the gateway's mTLS identity, for its per-address rate limit.
func (a *Auth) RequestCode(ctx context.Context, email, clientIP string) error {
	if clientIP != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, authv1.ClientIPMetadata, clientIP)
	}

	if _, err := a.api.RequestCode(ctx, &authv1.RequestCodeRequest{Email: email}); err != nil {
		return errs.Wrap("a.api.RequestCode", fromStatus(err))
	}

	return nil
}

func (a *Auth) VerifyCode(ctx context.Context, email, code string) (model.Tokens, error) {
	pair, err := a.api.VerifyCode(ctx, &authv1.VerifyCodeRequest{Email: email, Code: code})
	if err != nil {
		return model.Tokens{}, errs.Wrap("a.api.VerifyCode", fromStatus(err))
	}

	return toTokens(pair), nil
}

func (a *Auth) RefreshToken(ctx context.Context, refreshToken string) (model.Tokens, error) {
	pair, err := a.api.RefreshToken(ctx, &authv1.RefreshTokenRequest{RefreshToken: refreshToken})
	if err != nil {
		return model.Tokens{}, errs.Wrap("a.api.RefreshToken", fromStatus(err))
	}

	return toTokens(pair), nil
}

func (a *Auth) Logout(ctx context.Context, refreshToken string) error {
	if _, err := a.api.Logout(ctx, &authv1.LogoutRequest{RefreshToken: refreshToken}); err != nil {
		return errs.Wrap("a.api.Logout", fromStatus(err))
	}

	return nil
}

func toTokens(pair *authv1.TokenPair) model.Tokens {
	return model.Tokens{
		AccessToken:  pair.GetAccessToken(),
		RefreshToken: pair.GetRefreshToken(),
	}
}
