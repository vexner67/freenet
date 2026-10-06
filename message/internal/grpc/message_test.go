package grpc_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	messagev1 "github.com/vexner67/freenet/message/api/message/v1"
	messagegrpc "github.com/vexner67/freenet/message/internal/grpc"
	"github.com/vexner67/freenet/message/internal/inmem"
	"github.com/vexner67/freenet/message/internal/service"
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
	client messagev1.MessageServiceClient
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
		messagegrpc.ErrorInterceptor(log),
		jwtauth.UnaryServerInterceptor(jwtauth.NewVerifier(&key.PublicKey, issuer), messagegrpc.PublicMethods()...),
	))
	store := inmem.NewStore()
	messagev1.RegisterMessageServiceServer(srv, messagegrpc.NewMessageHandler(service.NewMessageService(store.Messages(), store.Dialogs()), log))

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

	return testServer{client: messagev1.NewMessageServiceClient(conn), key: key}
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

func TestDialog(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, bob := uuid.New(), uuid.New()

	var fromBob []string
	for _, text := range []string{"привет", "как дела?"} {
		resp, err := s.client.SendMessage(s.as(t, bob), &messagev1.SendMessageRequest{RecipientId: ann.String(), Text: text})
		if err != nil {
			t.Fatalf("SendMessage: %v", err)
		}
		if m := resp.GetMessage(); m.GetSenderId() != bob.String() || m.GetRecipientId() != ann.String() || m.GetRead() || m.GetCreatedAt() == nil {
			t.Fatalf("sent = %v", m)
		}
		fromBob = append(fromBob, resp.GetMessage().GetId())
	}

	unread, err := s.client.CountUnreadDialogs(s.as(t, ann), &messagev1.CountUnreadDialogsRequest{})
	if err != nil || unread.GetCount() != 1 {
		t.Fatalf("CountUnreadDialogs = %v, %v", unread, err)
	}

	dialogs, err := s.client.ListDialogs(s.as(t, ann), &messagev1.ListDialogsRequest{})
	if err != nil || len(dialogs.GetDialogs()) != 1 {
		t.Fatalf("ListDialogs = %v, %v", dialogs, err)
	}
	if d := dialogs.GetDialogs()[0]; d.GetPeerId() != bob.String() || !d.GetUnread() || d.GetLastMessage().GetText() != "как дела?" {
		t.Fatalf("dialog = %v", d)
	}

	if _, err = s.client.MarkDialogRead(s.as(t, ann), &messagev1.MarkDialogReadRequest{PeerId: bob.String(), UpToId: fromBob[1]}); err != nil {
		t.Fatalf("MarkDialogRead: %v", err)
	}

	// Bob sees his messages read.
	history, err := s.client.ListMessages(s.as(t, bob), &messagev1.ListMessagesRequest{PeerId: ann.String()})
	if err != nil {
		t.Fatal(err)
	}
	if len(history.GetMessages()) != 2 || !history.GetMessages()[1].GetRead() || history.GetPeerReadUpTo() != fromBob[1] || history.GetNextCursor() != "" {
		t.Fatalf("bob's history = %v", history)
	}
	if history.GetMessages()[0].GetText() != "привет" {
		t.Fatalf("oldest first: %v", history.GetMessages())
	}

	reply, err := s.client.SendMessage(s.as(t, ann), &messagev1.SendMessageRequest{RecipientId: bob.String(), Text: "хорошо"})
	if err != nil {
		t.Fatal(err)
	}

	// Bob polls for what came after the last message he had.
	newer, err := s.client.ListMessages(s.as(t, bob), &messagev1.ListMessagesRequest{PeerId: ann.String(), After: fromBob[1]})
	if err != nil || len(newer.GetMessages()) != 1 || newer.GetMessages()[0].GetId() != reply.GetMessage().GetId() {
		t.Fatalf("newer = %v, %v", newer, err)
	}
}

func TestErrors(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann := uuid.New()
	asAnn := s.as(t, ann)
	bob := uuid.NewString()

	tests := []struct {
		name string
		call func() error
		want codes.Code
	}{
		{"send without token", func() error {
			_, err := s.client.SendMessage(context.Background(), &messagev1.SendMessageRequest{RecipientId: bob, Text: "x"})
			return err
		}, codes.Unauthenticated},
		{"dialogs without token", func() error {
			_, err := s.client.ListDialogs(context.Background(), &messagev1.ListDialogsRequest{})
			return err
		}, codes.Unauthenticated},
		{"empty text", func() error {
			_, err := s.client.SendMessage(asAnn, &messagev1.SendMessageRequest{RecipientId: bob, Text: " \n "})
			return err
		}, codes.InvalidArgument},
		{"too long text", func() error {
			_, err := s.client.SendMessage(asAnn, &messagev1.SendMessageRequest{RecipientId: bob, Text: strings.Repeat("я", 4097)})
			return err
		}, codes.InvalidArgument},
		{"to yourself", func() error {
			_, err := s.client.SendMessage(asAnn, &messagev1.SendMessageRequest{RecipientId: ann.String(), Text: "x"})
			return err
		}, codes.InvalidArgument},
		{"bad recipient", func() error {
			_, err := s.client.SendMessage(asAnn, &messagev1.SendMessageRequest{RecipientId: "bob", Text: "x"})
			return err
		}, codes.InvalidArgument},
		{"cursor and after", func() error {
			_, err := s.client.ListMessages(asAnn, &messagev1.ListMessagesRequest{PeerId: bob, Cursor: uuid.NewString(), After: uuid.NewString()})
			return err
		}, codes.InvalidArgument},
		{"bad after", func() error {
			_, err := s.client.ListMessages(asAnn, &messagev1.ListMessagesRequest{PeerId: bob, After: "x"})
			return err
		}, codes.InvalidArgument},
		{"bad dialogs cursor", func() error {
			_, err := s.client.ListDialogs(asAnn, &messagev1.ListDialogsRequest{Cursor: "x"})
			return err
		}, codes.InvalidArgument},
		{"mark an unknown message", func() error {
			_, err := s.client.MarkDialogRead(asAnn, &messagev1.MarkDialogReadRequest{PeerId: bob, UpToId: uuid.NewString()})
			return err
		}, codes.NotFound},
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
