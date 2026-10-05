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

func (j *JWT) CreateAccessToken(userID, sessionID, tokenID string) (string, error) {
	return j.create(jwtauth.AccessToken, j.accessTTL, userID, sessionID, tokenID)
}

func (j *JWT) CreateRefreshToken(userID, sessionID, tokenID string) (string, error) {
	return j.create(jwtauth.RefreshToken, j.refreshTTL, userID, sessionID, tokenID)
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

func (j *JWT) create(
	typ jwtauth.TokenType,
	ttl time.Duration,
	userID, sessionID, tokenID string,
) (string, error) {
	now := time.Now()

	claims := jwtauth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ID:        tokenID,
			Issuer:    j.issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
		TokenType: typ,
		SessionID: sessionID,
	}

	token, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(j.privateKey)
	if err != nil {
		return "", errs.Wrap("token.SignedString", err)
	}

	return token, nil
}
