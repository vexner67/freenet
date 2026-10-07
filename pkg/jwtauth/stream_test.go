package jwtauth_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/jwtauth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type fakeStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s fakeStream) Context() context.Context { return s.ctx }

func TestStreamServerInterceptor(t *testing.T) {
	t.Parallel()

	key := newKey(t)
	userID := uuid.New()
	interceptor := jwtauth.StreamServerInterceptor(jwtauth.NewVerifier(&key.PublicKey, issuer))

	open := func(md metadata.MD, handler grpc.StreamHandler) error {
		ctx := metadata.NewIncomingContext(context.Background(), md)
		return interceptor(nil, fakeStream{ctx: ctx}, &grpc.StreamServerInfo{FullMethod: "/svc/Subscribe"}, handler)
	}

	t.Run("lives as long as the token", func(t *testing.T) {
		t.Parallel()

		opts := validOpts(key, userID, uuid.New())
		opts.expiresAt = time.Now().Add(time.Hour).Truncate(time.Second)

		err := open(metadata.Pairs("authorization", "Bearer "+sign(t, opts)), func(_ any, ss grpc.ServerStream) error {
			id, ok := jwtauth.IdentityFromContext(ss.Context())
			if !ok || id.UserID != userID {
				t.Fatalf("identity = %+v, %v", id, ok)
			}
			if deadline, ok := ss.Context().Deadline(); !ok || !deadline.Equal(opts.expiresAt) {
				t.Fatalf("deadline = %v, %v; want the token's expiry %v", deadline, ok, opts.expiresAt)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("ends with Unauthenticated when the token expires", func(t *testing.T) {
		t.Parallel()

		opts := validOpts(key, userID, uuid.New())
		opts.expiresAt = time.Now().Add(time.Second)

		err := open(metadata.Pairs("authorization", "Bearer "+sign(t, opts)), func(_ any, ss grpc.ServerStream) error {
			<-ss.Context().Done()
			return jwtauth.StreamEnd(ss.Context())
		})
		if got := status.Code(err); got != codes.Unauthenticated {
			t.Fatalf("code = %v, want Unauthenticated", got)
		}
	})

	t.Run("a client that leaves is Canceled", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if got := status.Code(jwtauth.StreamEnd(ctx)); got != codes.Canceled {
			t.Fatalf("code = %v, want Canceled", got)
		}
	})

	t.Run("needs a valid access token", func(t *testing.T) {
		t.Parallel()

		refresh := validOpts(key, userID, uuid.New())
		refresh.typ = jwtauth.RefreshToken

		for _, md := range []metadata.MD{
			{},
			metadata.Pairs("authorization", "Bearer garbage"),
			metadata.Pairs("authorization", "Bearer "+sign(t, refresh)),
		} {
			err := open(md, func(any, grpc.ServerStream) error {
				t.Fatal("the handler must not run")
				return nil
			})
			if got := status.Code(err); got != codes.Unauthenticated {
				t.Fatalf("metadata %v: code = %v", md, got)
			}
		}
	})
}
