package grpc_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/jwtauth"
	roomv1 "github.com/vexner67/freenet/room/api/room/v1"
	roomgrpc "github.com/vexner67/freenet/room/internal/grpc"
	"github.com/vexner67/freenet/room/internal/room"
	"github.com/vexner67/freenet/room/internal/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"
)

type reader struct {
	roomgrpc.Reader
	err error
}

func (r *reader) GetRoom(context.Context, uuid.UUID, uuid.UUID) (service.View, error) {
	return service.View{}, r.err
}

type subscriber struct {
	events chan service.Event
	ready  chan struct{}
}

func (s *subscriber) Subscribe(uuid.UUID) (<-chan service.Event, func()) {
	close(s.ready)
	return s.events, func() {}
}
func TestRoomRPCAndSubscribe(t *testing.T) {
	t.Parallel()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	verifier := jwtauth.NewVerifier(&key.PublicKey, "room-test")
	r := &reader{err: room.ErrNotFound}
	events := &subscriber{events: make(chan service.Event, 1), ready: make(chan struct{})}
	server := grpc.NewServer(grpc.ChainUnaryInterceptor(roomgrpc.ErrorInterceptor(slog.New(slog.NewTextHandler(io.Discard, nil))), roomgrpc.AuthInterceptor(verifier)), grpc.ChainStreamInterceptor(jwtauth.StreamServerInterceptor(verifier)))
	roomv1.RegisterRoomServiceServer(server, roomgrpc.NewHandler(roomgrpc.Deps{Reader: r, Events: events}))
	listener := bufconn.Listen(1 << 20)
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		if err := <-done; err != nil {
			t.Error(err)
		}
	})
	conn, err := grpc.NewClient("passthrough:///room-test", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Error(err)
		}
	})
	client := roomv1.NewRoomServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, test := range []struct {
		id   string
		code codes.Code
	}{{"invalid", codes.InvalidArgument}, {uuid.NewString(), codes.NotFound}} {
		_, err := client.GetRoom(ctx, &roomv1.GetRoomRequest{RoomId: test.id})
		if status.Code(err) != test.code {
			t.Fatal(err)
		}
	}
	for _, e := range []struct {
		err  error
		code codes.Code
	}{{room.ErrForbidden, codes.PermissionDenied}, {room.ErrFull, codes.ResourceExhausted}, {room.ErrStalePlayback, codes.FailedPrecondition}, {io.ErrUnexpectedEOF, codes.Internal}} {
		r.err = e.err
		_, err := client.GetRoom(ctx, &roomv1.GetRoomRequest{RoomId: uuid.NewString()})
		if status.Code(err) != e.code {
			t.Fatal(err)
		}
		if e.code == codes.Internal && status.Convert(err).Message() == e.err.Error() {
			t.Fatal("internal detail leaked")
		}
	}
	if _, err = client.PurgeUser(ctx, &roomv1.PurgeUserRequest{UserId: uuid.NewString()}); status.Code(err) != codes.PermissionDenied {
		t.Fatal(err)
	}
	if _, err = client.JoinRoom(ctx, &roomv1.JoinRoomRequest{RoomId: uuid.NewString()}); status.Code(err) != codes.Unauthenticated {
		t.Fatal(err)
	}
	id := uuid.New()
	claims := jwtauth.Claims{RegisteredClaims: jwt.RegisteredClaims{Issuer: "room-test", Subject: id.String(), ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}, TokenType: jwtauth.AccessToken, SessionID: uuid.NewString()}
	token, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := client.Subscribe(metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token), &roomv1.SubscribeRequest{})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-events.ready:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	rid := uuid.New()
	events.events <- service.Event{Kind: "room_join", RoomID: rid, UserID: id}
	event, err := stream.Recv()
	if err != nil || event.GetKind() != "room_join" || event.GetRoomId() != rid.String() {
		t.Fatal(event, err)
	}
}
