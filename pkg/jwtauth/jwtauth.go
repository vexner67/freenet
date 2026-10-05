// Package jwtauth defines the JWT format shared by freenet services and
// verifies tokens issued by the auth service.
package jwtauth

import (
	"crypto/rsa"
	"errors"
	"fmt"
	"os"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type TokenType string

const (
	AccessToken  TokenType = "access"
	RefreshToken TokenType = "refresh"
)

// Claims is the payload of every freenet token. The auth service signs it,
// every other service only verifies it.
type Claims struct {
	jwt.RegisteredClaims
	TokenType TokenType `json:"type"`
	SessionID string    `json:"sessionId"`
}

var ErrInvalidToken = errors.New("invalid token")

// Identity is who a verified token belongs to.
type Identity struct {
	UserID    uuid.UUID
	SessionID uuid.UUID
}

type Verifier struct {
	publicKey *rsa.PublicKey
	issuer    string
}

func NewVerifier(publicKey *rsa.PublicKey, issuer string) *Verifier {
	return &Verifier{
		publicKey: publicKey,
		issuer:    issuer,
	}
}

// Verify checks the signature, issuer, expiry and type of the token.
// Every failure wraps ErrInvalidToken.
func (v *Verifier) Verify(token string, want TokenType) (Identity, error) {
	claims := &Claims{}

	_, err := jwt.ParseWithClaims(
		token,
		claims,
		func(*jwt.Token) (any, error) {
			return v.publicKey, nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
		jwt.WithIssuer(v.issuer),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return Identity{}, fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}

	if claims.TokenType != want {
		return Identity{}, fmt.Errorf("%w: unexpected token type %q", ErrInvalidToken, claims.TokenType)
	}

	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return Identity{}, fmt.Errorf("%w: subject: %w", ErrInvalidToken, err)
	}

	sessionID, err := uuid.Parse(claims.SessionID)
	if err != nil {
		return Identity{}, fmt.Errorf("%w: session id: %w", ErrInvalidToken, err)
	}

	return Identity{
		UserID:    userID,
		SessionID: sessionID,
	}, nil
}

// LoadPublicKey reads a PEM-encoded RSA public key (PKIX or PKCS#1).
func LoadPublicKey(path string) (*rsa.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("os.ReadFile: %w", err)
	}

	key, err := jwt.ParseRSAPublicKeyFromPEM(data)
	if err != nil {
		return nil, fmt.Errorf("jwt.ParseRSAPublicKeyFromPEM: %w", err)
	}

	return key, nil
}
