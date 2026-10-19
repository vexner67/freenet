package jwtauth_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/jwtauth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const issuer = "freenet-test"

func newKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}

	return key
}

type tokenOpts struct {
	key       *rsa.PrivateKey
	method    jwt.SigningMethod
	issuer    string
	typ       jwtauth.TokenType
	subject   string
	sessionID string
	expiresAt time.Time
	// deactivated signs the token of a deleted page, banned of a blocked
	// one.
	deactivated bool
	banned      bool
}

func sign(t *testing.T, o tokenOpts) string {
	t.Helper()

	claims := jwtauth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   o.subject,
			Issuer:    o.issuer,
			ExpiresAt: jwt.NewNumericDate(o.expiresAt),
		},
		TokenType:   o.typ,
		SessionID:   o.sessionID,
		Deactivated: o.deactivated,
		Banned:      o.banned,
	}

	var signKey any = o.key
	if o.method == jwt.SigningMethodHS256 {
		signKey = []byte("secret")
	}

	token, err := jwt.NewWithClaims(o.method, claims).SignedString(signKey)
	if err != nil {
		t.Fatalf("SignedString: %v", err)
	}

	return token
}

func validOpts(key *rsa.PrivateKey, userID, sessionID uuid.UUID) tokenOpts {
	return tokenOpts{
		key:       key,
		method:    jwt.SigningMethodRS256,
		issuer:    issuer,
		typ:       jwtauth.AccessToken,
		subject:   userID.String(),
		sessionID: sessionID.String(),
		expiresAt: time.Now().Add(time.Minute),
	}
}

func TestVerifier_Verify(t *testing.T) {
	t.Parallel()

	key := newKey(t)
	otherKey := newKey(t)
	userID, sessionID := uuid.New(), uuid.New()
	v := jwtauth.NewVerifier(&key.PublicKey, issuer)

	t.Run("valid access token", func(t *testing.T) {
		t.Parallel()

		id, err := v.Verify(sign(t, validOpts(key, userID, sessionID)), jwtauth.AccessToken)
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
		if id.UserID != userID || id.SessionID != sessionID {
			t.Fatalf("got %+v", id)
		}
	})

	tests := []struct {
		name   string
		modify func(*tokenOpts)
		want   jwtauth.TokenType
	}{
		{"refresh token used as access", func(o *tokenOpts) { o.typ = jwtauth.RefreshToken }, jwtauth.AccessToken},
		{"access token used as refresh", func(*tokenOpts) {}, jwtauth.RefreshToken},
		{"expired", func(o *tokenOpts) { o.expiresAt = time.Now().Add(-time.Minute) }, jwtauth.AccessToken},
		{"wrong issuer", func(o *tokenOpts) { o.issuer = "someone-else" }, jwtauth.AccessToken},
		{"signed by another key", func(o *tokenOpts) { o.key = otherKey }, jwtauth.AccessToken},
		{"hmac instead of rsa", func(o *tokenOpts) { o.method = jwt.SigningMethodHS256 }, jwtauth.AccessToken},
		{"subject is not a uuid", func(o *tokenOpts) { o.subject = "admin" }, jwtauth.AccessToken},
		{"session id is not a uuid", func(o *tokenOpts) { o.sessionID = "" }, jwtauth.AccessToken},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			o := validOpts(key, userID, sessionID)
			tt.modify(&o)

			_, err := v.Verify(sign(t, o), tt.want)
			if !errors.Is(err, jwtauth.ErrInvalidToken) {
				t.Fatalf("got %v, want ErrInvalidToken", err)
			}
		})
	}

	t.Run("garbage", func(t *testing.T) {
		t.Parallel()

		if _, err := v.Verify("not.a.jwt", jwtauth.AccessToken); !errors.Is(err, jwtauth.ErrInvalidToken) {
			t.Fatalf("got %v, want ErrInvalidToken", err)
		}
	})
}

func TestUnaryServerInterceptor(t *testing.T) {
	t.Parallel()

	key := newKey(t)
	userID, sessionID := uuid.New(), uuid.New()
	token := sign(t, validOpts(key, userID, sessionID))

	deactivatedOpts := validOpts(key, userID, sessionID)
	deactivatedOpts.deactivated = true
	deactivated := sign(t, deactivatedOpts)

	bannedOpts := validOpts(key, userID, sessionID)
	bannedOpts.banned = true
	banned := sign(t, bannedOpts)

	interceptor := jwtauth.UnaryServerInterceptor(
		jwtauth.NewVerifier(&key.PublicKey, issuer),
		jwtauth.Access{Public: []string{"/svc/Public"}, Deactivated: []string{"/svc/Restore"}, Banned: []string{"/svc/Me"}},
	)

	call := func(method string, md metadata.MD) (jwtauth.Identity, bool, error) {
		var (
			got   jwtauth.Identity
			found bool
		)

		ctx := metadata.NewIncomingContext(context.Background(), md)
		_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: method},
			func(ctx context.Context, _ any) (any, error) {
				got, found = jwtauth.IdentityFromContext(ctx)
				return nil, nil
			})

		return got, found, err
	}

	t.Run("protected method with a valid token", func(t *testing.T) {
		t.Parallel()

		id, found, err := call("/svc/Private", metadata.Pairs("authorization", "Bearer "+token))
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if !found || id.UserID != userID {
			t.Fatalf("identity not in context: %+v", id)
		}
	})

	t.Run("scheme is case-insensitive", func(t *testing.T) {
		t.Parallel()

		if _, _, err := call("/svc/Private", metadata.Pairs("authorization", "bearer "+token)); err != nil {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("public method with a token knows the viewer", func(t *testing.T) {
		t.Parallel()

		id, found, err := call("/svc/Public", metadata.Pairs("authorization", "Bearer "+token))
		if err != nil || !found || id.UserID != userID {
			t.Fatalf("err = %v, found = %v, id = %+v", err, found, id)
		}
	})

	t.Run("public method with a bad token is rejected", func(t *testing.T) {
		t.Parallel()

		_, _, err := call("/svc/Public", metadata.Pairs("authorization", "Bearer garbage"))
		if status.Code(err) != codes.Unauthenticated {
			t.Fatalf("code = %v, want Unauthenticated", status.Code(err))
		}
	})

	t.Run("public method without a token", func(t *testing.T) {
		t.Parallel()

		_, found, err := call("/svc/Public", metadata.MD{})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if found {
			t.Fatal("identity must not be set for anonymous calls")
		}
	})

	t.Run("deleted page: allowed method knows its owner", func(t *testing.T) {
		t.Parallel()

		id, found, err := call("/svc/Restore", metadata.Pairs("authorization", "Bearer "+deactivated))
		if err != nil || !found || id.UserID != userID || !id.Deactivated {
			t.Fatalf("err = %v, found = %v, id = %+v", err, found, id)
		}
	})

	t.Run("deleted page: public method sees an anonymous viewer", func(t *testing.T) {
		t.Parallel()

		_, found, err := call("/svc/Public", metadata.Pairs("authorization", "Bearer "+deactivated))
		if err != nil || found {
			t.Fatalf("err = %v, found = %v; want an anonymous call", err, found)
		}
	})

	t.Run("deleted page: other methods refuse it", func(t *testing.T) {
		t.Parallel()

		_, _, err := call("/svc/Private", metadata.Pairs("authorization", "Bearer "+deactivated))
		if status.Code(err) != codes.PermissionDenied || status.Convert(err).Message() != jwtauth.ErrPageDeleted.Error() {
			t.Fatalf("err = %v, want PermissionDenied: your page is deleted", err)
		}
	})

	t.Run("blocked page: allowed method knows its owner", func(t *testing.T) {
		t.Parallel()

		id, found, err := call("/svc/Me", metadata.Pairs("authorization", "Bearer "+banned))
		if err != nil || !found || id.UserID != userID || !id.Banned {
			t.Fatalf("err = %v, found = %v, id = %+v", err, found, id)
		}
	})

	t.Run("blocked page: public method sees an anonymous viewer", func(t *testing.T) {
		t.Parallel()

		_, found, err := call("/svc/Public", metadata.Pairs("authorization", "Bearer "+banned))
		if err != nil || found {
			t.Fatalf("err = %v, found = %v; want an anonymous call", err, found)
		}
	})

	t.Run("blocked page: other methods refuse it", func(t *testing.T) {
		t.Parallel()

		for _, method := range []string{"/svc/Private", "/svc/Restore"} {
			_, _, err := call(method, metadata.Pairs("authorization", "Bearer "+banned))
			if status.Code(err) != codes.PermissionDenied || status.Convert(err).Message() != jwtauth.ErrPageBanned.Error() {
				t.Fatalf("%s: err = %v, want PermissionDenied: your page is banned", method, err)
			}
		}
	})

	t.Run("active page on a method for deleted ones", func(t *testing.T) {
		t.Parallel()

		if _, found, err := call("/svc/Restore", metadata.Pairs("authorization", "Bearer "+token)); err != nil || !found {
			t.Fatalf("err = %v, found = %v", err, found)
		}
	})

	t.Run("health check without a token", func(t *testing.T) {
		t.Parallel()

		if _, _, err := call(grpc_health_v1.Health_Check_FullMethodName, metadata.MD{}); err != nil {
			t.Fatalf("err = %v", err)
		}
	})

	rejected := []struct {
		name string
		md   metadata.MD
	}{
		{"no metadata", metadata.MD{}},
		{"no bearer scheme", metadata.Pairs("authorization", token)},
		{"empty token", metadata.Pairs("authorization", "Bearer ")},
		{"invalid token", metadata.Pairs("authorization", "Bearer garbage")},
	}

	for _, tt := range rejected {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := call("/svc/Private", tt.md)
			if status.Code(err) != codes.Unauthenticated {
				t.Fatalf("code = %v, want Unauthenticated", status.Code(err))
			}
		})
	}
}
