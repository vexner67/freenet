package jwt

import (
	"crypto/rsa"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/vexner67/freenet/auth/internal/service"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/pkg/jwtauth"
)

type Config struct {
	PrivateKey *rsa.PrivateKey
	PublicKey  *rsa.PublicKey
	Issuer     string
	AccessTTL  time.Duration
	RefreshTTL time.Duration
}

// JWT issues tokens in the format shared through pkg/jwtauth and verifies
// refresh tokens presented back to auth.
type JWT struct {
	privateKey *rsa.PrivateKey
	verifier   *jwtauth.Verifier
	issuer     string
	accessTTL  time.Duration
	refreshTTL time.Duration
}

func New(cfg Config) *JWT {
	return &JWT{
		privateKey: cfg.PrivateKey,
		verifier:   jwtauth.NewVerifier(cfg.PublicKey, cfg.Issuer),
		issuer:     cfg.Issuer,
		accessTTL:  cfg.AccessTTL,
		refreshTTL: cfg.RefreshTTL,
	}
}

func (j *JWT) CreateAccessToken(acc service.Account, sessionID, tokenID string) (string, error) {
	return j.create(token{
		typ:         jwtauth.AccessToken,
		ttl:         j.accessTTL,
		userID:      acc.UserID.String(),
		sessionID:   sessionID,
		id:          tokenID,
		deactivated: acc.Deactivated,
		banned:      acc.Banned,
	})
}

// CreateRefreshToken: a refresh token never says whether the page is
// deleted or blocked, RefreshToken asks the user service each time.
func (j *JWT) CreateRefreshToken(userID, sessionID, tokenID string) (string, error) {
	return j.create(token{typ: jwtauth.RefreshToken, ttl: j.refreshTTL, userID: userID, sessionID: sessionID, id: tokenID})
}

// token is what create signs.
type token struct {
	typ         jwtauth.TokenType
	ttl         time.Duration
	userID      string
	sessionID   string
	id          string
	deactivated bool
	banned      bool
}

func (j *JWT) ParseRefreshToken(token string) (service.RefreshClaims, error) {
	identity, err := j.verifier.Verify(token, jwtauth.RefreshToken)
	if err != nil {
		// The reason stays in the chain for logs; callers only need to know
		// the token is not acceptable.
		return service.RefreshClaims{}, errs.Wrap("j.verifier.Verify", errors.Join(service.ErrInvalidRefreshToken, err))
	}

	return service.RefreshClaims{
		UserID:    identity.UserID,
		SessionID: identity.SessionID,
	}, nil
}

func (j *JWT) create(t token) (string, error) {
	now := time.Now()

	claims := jwtauth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   t.userID,
			ID:        t.id,
			Issuer:    j.issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(t.ttl)),
		},
		TokenType:   t.typ,
		SessionID:   t.sessionID,
		Deactivated: t.deactivated,
		Banned:      t.banned,
	}

	token, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(j.privateKey)
	if err != nil {
		return "", errs.Wrap("token.SignedString", err)
	}

	return token, nil
}
