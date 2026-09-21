package jwt

import (
	"crypto/rsa"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	jwt.RegisteredClaims
	TokenType string `json:"type"`
	SessionID string `json:"sessionId"`
}

const (
	AccessToken  = "access"
	RefreshToken = "refresh"
)

type JWT struct {
	privateKey *rsa.PrivateKey
	issuer     string
	accessTTL  time.Duration
	refreshTTL time.Duration
}

func New(
	privateKey *rsa.PrivateKey,
	issuer string,
	accessTTL time.Duration,
	refreshTTL time.Duration,
) *JWT {
	return &JWT{
		privateKey: privateKey,
		issuer:     issuer,
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
	}
}

func (j *JWT) CreateAccessToken(userID, sessionID, tokenID string) (string, error) {
	now := time.Now()

	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ID:        tokenID,
			Issuer:    j.issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(j.accessTTL)),
		},
		TokenType: AccessToken,
		SessionID: sessionID,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)

	return token.SignedString(j.privateKey)
}

func (j *JWT) CreateRefreshToken(userID, sessionID, tokenID string) (string, error) {
	now := time.Now()

	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ID:        tokenID,
			Issuer:    j.issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(j.refreshTTL)),
		},
		TokenType: RefreshToken,
		SessionID: sessionID,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)

	return token.SignedString(j.privateKey)
}

func (j *JWT) Parse(tokenString string) (*Claims, error) {
	claims := &Claims{}

	_, err := jwt.ParseWithClaims(
		tokenString,
		claims,
		func(token *jwt.Token) (any, error) {
			return &j.privateKey.PublicKey, nil
		},
		jwt.WithValidMethods([]string{
			jwt.SigningMethodRS256.Alg(),
		}),
		jwt.WithIssuer(j.issuer),
	)
	if err != nil {
		return nil, err
	}

	return claims, nil
}
