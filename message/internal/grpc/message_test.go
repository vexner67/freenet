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
	"github.com/vexner67/freenet/message/internal/mediaclient"
	"github.com/vexner67/freenet/message/internal/service"
	"github.com/vexner67/freenet/pkg/jwtauth"
	"github.com/vexner67/freenet/pkg/pubsub"
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
	verifier := jwtauth.NewVerifier(&key.PublicKey, issuer)
	srv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			messagegrpc.ErrorInterceptor(log),
			jwtauth.UnaryServerInterceptor(verifier, jwtauth.Access{Public: messagegrpc.PublicMethods()}),
		),
		grpc.ChainStreamInterceptor(jwtauth.StreamServerInterceptor(verifier)),
	)
	store := inmem.NewStore()
	events := pubsub.New[uuid.UUID, service.Event](64)
	messagev1.RegisterMessageServiceServer(srv, messagegrpc.NewMessageHandler(service.NewMessageService(service.Deps{Messages: store.Messages(), Dialogs: store.Dialogs(), Events: events, Tx: inmem.TxManager{}, Photos: mediaclient.Nop{}}), log))

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
		if m := resp.GetMessage(); m.GetSenderId() != bob.String() || m.GetRecipientId() != ann.String() || m.GetRead() || m.GetCreatedAt() == nil ||
			!m.GetDeletableUntil().AsTime().Equal(m.GetCreatedAt().AsTime().Add(24*time.Hour)) {
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

	// Ann shares a post: a message of it alone.
	post := uuid.NewString()
	shared, err := s.client.SendMessage(s.as(t, ann), &messagev1.SendMessageRequest{RecipientId: bob.String(), PostId: post})
	if err != nil || shared.GetMessage().GetPostId() != post || shared.GetMessage().GetText() != "" {
		t.Fatalf("a shared post = %v, %v", shared, err)
	}
	if _, err = s.client.SendMessage(s.as(t, ann), &messagev1.SendMessageRequest{RecipientId: bob.String(), PostId: "x"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("a bad post id: %v", err)
	}
	if _, err = s.client.DeleteMessage(s.as(t, ann), &messagev1.DeleteMessageRequest{Id: shared.GetMessage().GetId()}); err != nil {
		t.Fatal(err)
	}

	// Bob polls for what came after the last message he had.
	newer, err := s.client.ListMessages(s.as(t, bob), &messagev1.ListMessagesRequest{PeerId: ann.String(), After: fromBob[1]})
	if err != nil || len(newer.GetMessages()) != 1 || newer.GetMessages()[0].GetId() != reply.GetMessage().GetId() {
		t.Fatalf("newer = %v, %v", newer, err)
	}

	// Ann takes her answer back: bob can no longer see it.
	if _, err = s.client.DeleteMessage(s.as(t, ann), &messagev1.DeleteMessageRequest{Id: reply.GetMessage().GetId()}); err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}
	if history, err = s.client.ListMessages(s.as(t, bob), &messagev1.ListMessagesRequest{PeerId: ann.String()}); err != nil || len(history.GetMessages()) != 2 {
		t.Fatalf("bob's history after the deletion = %v, %v", history, err)
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
			_, err := s.client.SendMessage(asAnn, &messagev1.SendMessageRequest{RecipientId: bob, Text: strings.Repeat("я", 60001)})
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
		{"delete without token", func() error {
			_, err := s.client.DeleteMessage(context.Background(), &messagev1.DeleteMessageRequest{Id: uuid.NewString()})
			return err
		}, codes.Unauthenticated},
		{"delete a bad id", func() error {
			_, err := s.client.DeleteMessage(asAnn, &messagev1.DeleteMessageRequest{Id: "x"})
			return err
		}, codes.InvalidArgument},
		{"delete an unknown message", func() error {
			_, err := s.client.DeleteMessage(asAnn, &messagev1.DeleteMessageRequest{Id: uuid.NewString()})
			return err
		}, codes.NotFound},
		{"delete a message to you", func() error {
			resp, err := s.client.SendMessage(s.as(t, uuid.MustParse(bob)), &messagev1.SendMessageRequest{RecipientId: ann.String(), Text: "x"})
			if err != nil {
				return err
			}
			_, err = s.client.DeleteMessage(asAnn, &messagev1.DeleteMessageRequest{Id: resp.GetMessage().GetId()})
			return err
		}, codes.PermissionDenied},
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

func TestHasMessagesFrom(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, bob := uuid.New(), uuid.New()

	if _, err := s.client.SendMessage(s.as(t, bob), &messagev1.SendMessageRequest{RecipientId: ann.String(), Text: "привет"}); err != nil {
		t.Fatal(err)
	}

	resp, err := s.client.HasMessagesFrom(s.as(t, ann), &messagev1.HasMessagesFromRequest{PeerId: bob.String()})
	if err != nil || !resp.GetHasMessages() {
		t.Fatalf("ann has messages from bob: %v, %v", resp, err)
	}

	resp, err = s.client.HasMessagesFrom(s.as(t, bob), &messagev1.HasMessagesFromRequest{PeerId: ann.String()})
	if err != nil || resp.GetHasMessages() {
		t.Fatalf("bob has none from ann: %v, %v", resp, err)
	}

	_, err = s.client.HasMessagesFrom(s.as(t, ann), &messagev1.HasMessagesFromRequest{PeerId: "x"})
	if got := status.Code(err); got != codes.InvalidArgument {
		t.Fatalf("bad peer id: code = %v", got)
	}
}

func TestSubscribe(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, bob := uuid.New(), uuid.New()

	ctx, cancel := context.WithCancel(s.as(t, ann))
	defer cancel()

	stream, err := s.client.Subscribe(ctx, &messagev1.SubscribeRequest{})
	if err != nil {
		t.Fatal(err)
	}

	// The stream is open once the server has the subscription; until then
	// a message could be missed, so send until one arrives.
	var got *messagev1.SubscribeResponse
	received := make(chan *messagev1.SubscribeResponse, 1)
	go func() {
		resp, err := stream.Recv()
		if err == nil {
			received <- resp
		}
	}()

	var sent *messagev1.Message
	for got == nil {
		resp, err := s.client.SendMessage(s.as(t, bob), &messagev1.SendMessageRequest{RecipientId: ann.String(), Text: "привет"})
		if err != nil {
			t.Fatal(err)
		}
		sent = resp.GetMessage()

		select {
		case got = <-received:
		case <-time.After(50 * time.Millisecond):
		}
	}

	if m := got.GetMessage(); m == nil || m.GetSenderId() != bob.String() || m.GetText() != "привет" {
		t.Fatalf("event = %v, want bob's message", got)
	}

	if _, err = s.client.MarkDialogRead(s.as(t, ann), &messagev1.MarkDialogReadRequest{PeerId: bob.String(), UpToId: sent.GetId()}); err != nil {
		t.Fatal(err)
	}

	// Earlier sends of the loop may come first; the read mark follows them.
	for {
		resp, err := stream.Recv()
		if err != nil {
			t.Fatalf("Recv: %v", err)
		}
		if r := resp.GetRead(); r != nil {
			if r.GetReaderId() != ann.String() || r.GetPeerId() != bob.String() || r.GetUpToId() != sent.GetId() {
				t.Fatalf("read mark = %v", r)
			}
			break
		}
	}

	if _, err = s.client.DeleteMessage(s.as(t, bob), &messagev1.DeleteMessageRequest{Id: sent.GetId()}); err != nil {
		t.Fatal(err)
	}
	if resp, err := stream.Recv(); err != nil || resp.GetDeleted().GetId() != sent.GetId() ||
		resp.GetDeleted().GetSenderId() != bob.String() || resp.GetDeleted().GetRecipientId() != ann.String() {
		t.Fatalf("event = %v, %v; want the deletion", resp, err)
	}

	noToken, err := s.client.Subscribe(context.Background(), &messagev1.SubscribeRequest{})
	if err == nil {
		_, err = noToken.Recv()
	}
	if got := status.Code(err); got != codes.Unauthenticated {
		t.Fatalf("without a token: code = %v", got)
	}
}

func TestEditHideSearch(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, bob := uuid.New(), uuid.New()

	sent, err := s.client.SendMessage(s.as(t, ann), &messagev1.SendMessageRequest{RecipientId: bob.String(), Text: "Еду на море"})
	if err != nil {
		t.Fatal(err)
	}
	id := sent.GetMessage().GetId()
	if sent.GetMessage().GetEditedAt() != nil || sent.GetMessage().GetEditableUntil() == nil {
		t.Fatalf("a new message = %v", sent.GetMessage())
	}

	// Edit: the sender only.
	edited, err := s.client.EditMessage(s.as(t, ann), &messagev1.EditMessageRequest{Id: id, Text: "Еду на море!"})
	if err != nil || edited.GetMessage().GetText() != "Еду на море!" || edited.GetMessage().GetEditedAt() == nil {
		t.Fatalf("EditMessage = %v, %v", edited, err)
	}
	for name, tt := range map[string]struct {
		ctx  context.Context
		req  *messagev1.EditMessageRequest
		code codes.Code
	}{
		"without a token": {context.Background(), &messagev1.EditMessageRequest{Id: id, Text: "x"}, codes.Unauthenticated},
		"the recipient":   {s.as(t, bob), &messagev1.EditMessageRequest{Id: id, Text: "x"}, codes.PermissionDenied},
		"no text":         {s.as(t, ann), &messagev1.EditMessageRequest{Id: id, Text: " "}, codes.InvalidArgument},
		"a bad id":        {s.as(t, ann), &messagev1.EditMessageRequest{Id: "x", Text: "x"}, codes.InvalidArgument},
		"no message":      {s.as(t, ann), &messagev1.EditMessageRequest{Id: uuid.NewString(), Text: "x"}, codes.NotFound},
	} {
		if _, err = s.client.EditMessage(tt.ctx, tt.req); status.Code(err) != tt.code {
			t.Errorf("EditMessage, %s: %v, want %v", name, err, tt.code)
		}
	}

	// Search: in the dialog and in all of them.
	for _, peer := range []string{"", ann.String()} {
		found, err := s.client.SearchMessages(s.as(t, bob), &messagev1.SearchMessagesRequest{PeerId: peer, Query: "МОРЕ"})
		if err != nil || len(found.GetMessages()) != 1 || found.GetMessages()[0].GetId() != id {
			t.Fatalf("SearchMessages(%q) = %v, %v", peer, found, err)
		}
	}
	if _, err = s.client.SearchMessages(s.as(t, bob), &messagev1.SearchMessagesRequest{Query: " "}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("an empty query: %v", err)
	}

	// Hide: bob deletes it for himself, ann keeps it.
	if _, err = s.client.HideMessage(s.as(t, bob), &messagev1.HideMessageRequest{Id: id}); err != nil {
		t.Fatalf("HideMessage: %v", err)
	}
	if _, err = s.client.HideMessage(s.as(t, bob), &messagev1.HideMessageRequest{Id: id}); status.Code(err) != codes.NotFound {
		t.Fatalf("hidden twice: %v", err)
	}
	if bobs, _ := s.client.ListMessages(s.as(t, bob), &messagev1.ListMessagesRequest{PeerId: ann.String()}); len(bobs.GetMessages()) != 0 {
		t.Fatalf("bob still reads %v", bobs.GetMessages())
	}
	if anns, _ := s.client.ListMessages(s.as(t, ann), &messagev1.ListMessagesRequest{PeerId: bob.String()}); len(anns.GetMessages()) != 1 {
		t.Fatalf("ann reads %v", anns.GetMessages())
	}
	if found, _ := s.client.SearchMessages(s.as(t, bob), &messagev1.SearchMessagesRequest{Query: "море"}); len(found.GetMessages()) != 0 {
		t.Fatalf("bob finds a hidden message: %v", found)
	}
}
