package backend

import (
	"context"
	"strings"

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

// VerifyCode says where the user signs in from, for their list of sessions.
func (a *Auth) VerifyCode(ctx context.Context, email, code string, client model.Client) (model.Tokens, error) {
	pair, err := a.api.VerifyCode(withClient(ctx, client), &authv1.VerifyCodeRequest{Email: email, Code: code})
	if err != nil {
		return model.Tokens{}, errs.Wrap("a.api.VerifyCode", fromStatus(err))
	}

	return toTokens(pair), nil
}

func (a *Auth) RefreshToken(ctx context.Context, refreshToken string, client model.Client) (model.Tokens, error) {
	pair, err := a.api.RefreshToken(withClient(ctx, client), &authv1.RefreshTokenRequest{RefreshToken: refreshToken})
	if err != nil {
		return model.Tokens{}, errs.Wrap("a.api.RefreshToken", fromStatus(err))
	}

	return toTokens(pair), nil
}

// maxUserAgentLen is as much of the browser's name as auth keeps.
const maxUserAgentLen = 255

// withClient passes the end user's address and browser in metadata, which
// auth trusts only from the gateway's mTLS identity.
func withClient(ctx context.Context, client model.Client) context.Context {
	if client.IP != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, authv1.ClientIPMetadata, client.IP)
	}

	if agent := printable(client.UserAgent, maxUserAgentLen); agent != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, authv1.UserAgentMetadata, agent)
	}

	return ctx
}

// printable keeps the printable ASCII of s, at most limit bytes: gRPC
// refuses a call with anything else in its metadata, and the header is
// whatever the client sent.
func printable(s string, limit int) string {
	var b strings.Builder
	for _, r := range s {
		if b.Len() == limit {
			break
		}
		if r >= 0x20 && r <= 0x7e {
			b.WriteRune(r)
		}
	}

	return strings.TrimSpace(b.String())
}

func (a *Auth) Logout(ctx context.Context, refreshToken string) error {
	if _, err := a.api.Logout(ctx, &authv1.LogoutRequest{RefreshToken: refreshToken}); err != nil {
		return errs.Wrap("a.api.Logout", fromStatus(err))
	}

	return nil
}

func (a *Auth) ListSessions(ctx context.Context, accessToken string) ([]model.Session, error) {
	resp, err := a.api.ListSessions(withToken(ctx, accessToken), &authv1.ListSessionsRequest{})
	if err != nil {
		return nil, errs.Wrap("a.api.ListSessions", fromStatus(err))
	}

	sessions := make([]model.Session, 0, len(resp.GetSessions()))
	for _, s := range resp.GetSessions() {
		sessions = append(sessions, model.Session{
			ID:         s.GetId(),
			CreatedAt:  s.GetCreatedAt().AsTime(),
			LastUsedAt: s.GetLastUsedAt().AsTime(),
			IP:         s.GetIp(),
			UserAgent:  s.GetUserAgent(),
			Current:    s.GetCurrent(),
		})
	}

	return sessions, nil
}

func (a *Auth) EndSession(ctx context.Context, accessToken, sessionID string) error {
	if _, err := a.api.EndSession(withToken(ctx, accessToken), &authv1.EndSessionRequest{SessionId: sessionID}); err != nil {
		return errs.Wrap("a.api.EndSession", fromStatus(err))
	}

	return nil
}

// EndOtherSessions returns how many sessions ended.
func (a *Auth) EndOtherSessions(ctx context.Context, accessToken string) (int, error) {
	resp, err := a.api.EndOtherSessions(withToken(ctx, accessToken), &authv1.EndOtherSessionsRequest{})
	if err != nil {
		return 0, errs.Wrap("a.api.EndOtherSessions", fromStatus(err))
	}

	return int(resp.GetEnded()), nil
}

func toTokens(pair *authv1.TokenPair) model.Tokens {
	return model.Tokens{
		AccessToken:  pair.GetAccessToken(),
		RefreshToken: pair.GetRefreshToken(),
	}
}
