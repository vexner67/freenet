package grpc_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	notificationv1 "github.com/vexner67/freenet/notification/api/notification/v1"
	notificationgrpc "github.com/vexner67/freenet/notification/internal/grpc"
	"github.com/vexner67/freenet/notification/internal/inmem"
	"github.com/vexner67/freenet/notification/internal/service"
	"github.com/vexner67/freenet/pkg/jwtauth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const issuer = "freenet-test"

type testServer struct {
	client notificationv1.NotificationServiceClient
	key    *rsa.PrivateKey
}

// newServer wires the handler the same way app.New does, without mTLS.
func newServer(t *testing.T) testServer {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(
		notificationgrpc.ErrorInterceptor(log),
		jwtauth.UnaryServerInterceptor(jwtauth.NewVerifier(&key.PublicKey, issuer), notificationgrpc.PublicMethods()...),
	))
	notificationv1.RegisterNotificationServiceServer(srv, notificationgrpc.NewNotificationHandler(
		service.NewNotificationService(inmem.NewNotificationRepository(), inmem.NewReadMarkRepository()),
		log,
	))

	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	return testServer{client: notificationv1.NewNotificationServiceClient(conn), key: key}
}

// as returns a context of the given user with a signed access token.
func (s testServer) as(t *testing.T, userID uuid.UUID) context.Context {
	t.Helper()

	token, err := jwt.NewWithClaims(jwt.SigningMethodRS256, jwtauth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			Issuer:    issuer,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		},
		TokenType: jwtauth.AccessToken,
		SessionID: uuid.NewString(),
	}).SignedString(s.key)
	if err != nil {
		t.Fatal(err)
	}

	return metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+token)
}

func TestNotifications(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ctx := context.Background()
	ann, bob := uuid.New(), uuid.New()
	post, comment := uuid.NewString(), uuid.NewString()

	events := []*notificationv1.Event{
		{Kind: notificationv1.Kind_KIND_FOLLOW, RecipientId: ann.String(), ActorId: bob.String()},
		{Kind: notificationv1.Kind_KIND_LIKE, RecipientId: ann.String(), ActorId: bob.String(), PostId: post},
		{Kind: notificationv1.Kind_KIND_COMMENT, RecipientId: ann.String(), ActorId: bob.String(), PostId: post, CommentId: comment},
	}

	// Services call Notify without a user token.
	for _, e := range events {
		if _, err := s.client.Notify(ctx, &notificationv1.NotifyRequest{Event: e}); err != nil {
			t.Fatalf("Notify(%v): %v", e, err)
		}
	}

	unread, err := s.client.CountUnread(s.as(t, ann), &notificationv1.CountUnreadRequest{})
	if err != nil || unread.GetCount() != 3 {
		t.Fatalf("CountUnread = %v, %v", unread, err)
	}

	page, err := s.client.ListNotifications(s.as(t, ann), &notificationv1.ListNotificationsRequest{Box: notificationv1.Box_BOX_UNREAD, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.GetNotifications()) != 2 || page.GetNextCursor() == "" {
		t.Fatalf("first page: %v", page)
	}

	newest := page.GetNotifications()[0]
	if newest.GetKind() != notificationv1.Kind_KIND_COMMENT || newest.GetActorId() != bob.String() ||
		newest.GetPostId() != post || newest.GetCommentId() != comment || newest.GetCreatedAt() == nil {
		t.Fatalf("comment notification: %v", newest)
	}
	if follow := page.GetNotifications()[1]; follow.GetKind() != notificationv1.Kind_KIND_LIKE || follow.GetCommentId() != "" {
		t.Fatalf("like notification: %v", follow)
	}

	if _, err = s.client.MarkRead(s.as(t, ann), &notificationv1.MarkReadRequest{NotificationId: newest.GetId()}); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}

	archive, err := s.client.ListNotifications(s.as(t, ann), &notificationv1.ListNotificationsRequest{Box: notificationv1.Box_BOX_ARCHIVE})
	if err != nil || len(archive.GetNotifications()) != 3 {
		t.Fatalf("archive = %v, %v", archive, err)
	}

	if _, err = s.client.Retract(ctx, &notificationv1.RetractRequest{Event: events[0]}); err != nil {
		t.Fatalf("Retract: %v", err)
	}
	if _, err = s.client.ForgetPost(ctx, &notificationv1.ForgetPostRequest{PostId: post}); err != nil {
		t.Fatalf("ForgetPost: %v", err)
	}

	archive, err = s.client.ListNotifications(s.as(t, ann), &notificationv1.ListNotificationsRequest{Box: notificationv1.Box_BOX_ARCHIVE})
	if err != nil || len(archive.GetNotifications()) != 0 {
		t.Fatalf("after Retract and ForgetPost: %v, %v", archive, err)
	}
}

func TestErrors(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ctx := context.Background()
	ann := s.as(t, uuid.New())
	bob := uuid.NewString()

	tests := []struct {
		name string
		call func() error
		want codes.Code
	}{
		{"list without token", func() error {
			_, err := s.client.ListNotifications(ctx, &notificationv1.ListNotificationsRequest{Box: notificationv1.Box_BOX_UNREAD})
			return err
		}, codes.Unauthenticated},
		{"count without token", func() error {
			_, err := s.client.CountUnread(ctx, &notificationv1.CountUnreadRequest{})
			return err
		}, codes.Unauthenticated},
		{"no box", func() error {
			_, err := s.client.ListNotifications(ann, &notificationv1.ListNotificationsRequest{})
			return err
		}, codes.InvalidArgument},
		{"bad cursor", func() error {
			_, err := s.client.ListNotifications(ann, &notificationv1.ListNotificationsRequest{Box: notificationv1.Box_BOX_UNREAD, Cursor: "x"})
			return err
		}, codes.InvalidArgument},
		{"mark someone else's", func() error {
			_, err := s.client.MarkRead(ann, &notificationv1.MarkReadRequest{NotificationId: uuid.NewString()})
			return err
		}, codes.NotFound},
		{"mark a bad id", func() error {
			_, err := s.client.MarkRead(ann, &notificationv1.MarkReadRequest{NotificationId: "x"})
			return err
		}, codes.InvalidArgument},
		{"unknown kind", func() error {
			_, err := s.client.Notify(ctx, &notificationv1.NotifyRequest{Event: &notificationv1.Event{RecipientId: bob, ActorId: uuid.NewString()}})
			return err
		}, codes.InvalidArgument},
		{"bad id in the event", func() error {
			_, err := s.client.Notify(ctx, &notificationv1.NotifyRequest{Event: &notificationv1.Event{Kind: notificationv1.Kind_KIND_FOLLOW, RecipientId: bob, ActorId: "x"}})
			return err
		}, codes.InvalidArgument},
		{"like without a post", func() error {
			_, err := s.client.Notify(ctx, &notificationv1.NotifyRequest{Event: &notificationv1.Event{Kind: notificationv1.Kind_KIND_LIKE, RecipientId: bob, ActorId: uuid.NewString()}})
			return err
		}, codes.InvalidArgument},
		{"own action", func() error {
			_, err := s.client.Notify(ctx, &notificationv1.NotifyRequest{Event: &notificationv1.Event{Kind: notificationv1.Kind_KIND_FOLLOW, RecipientId: bob, ActorId: bob}})
			return err
		}, codes.InvalidArgument},
		{"forget a bad post id", func() error {
			_, err := s.client.ForgetPost(ctx, &notificationv1.ForgetPostRequest{PostId: "x"})
			return err
		}, codes.InvalidArgument},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := status.Code(tt.call()); got != tt.want {
				t.Fatalf("code = %v, want %v", got, tt.want)
			}
		})
	}
}
