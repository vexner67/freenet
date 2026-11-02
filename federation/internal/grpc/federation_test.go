package grpc_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	federationv1 "github.com/vexner67/freenet/federation/api/federation/v1"
	"github.com/vexner67/freenet/federation/internal/fedi"
	federationgrpc "github.com/vexner67/freenet/federation/internal/grpc"
	"github.com/vexner67/freenet/federation/internal/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fake struct {
	federationgrpc.FederationService

	err      error
	received service.SignedRequest
	to       uuid.UUID
	changed  service.PostEvent
}

func (f *fake) Receive(_ context.Context, r service.SignedRequest, _ fedi.Incoming, to uuid.UUID) error {
	f.received, f.to = r, to
	return f.err
}

func (f *fake) PostChanged(_ context.Context, e service.PostEvent) error {
	f.changed = e
	return f.err
}

func (f *fake) Actor(context.Context, uuid.UUID) (fedi.Person, error) {
	return fedi.Person{}, f.err
}

func newHandler(t *testing.T, f *fake) *federationgrpc.FederationHandler {
	t.Helper()

	site, err := fedi.NewSite("https://freenet.example")
	if err != nil {
		t.Fatal(err)
	}

	return federationgrpc.NewFederationHandler(f, site)
}

// call runs fn through the error interceptor, as the server does.
func call(fn func() (any, error)) error {
	interceptor := federationgrpc.ErrorInterceptor(slog.New(slog.NewTextHandler(io.Discard, nil)))
	_, err := interceptor(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/x"}, func(context.Context, any) (any, error) {
		return fn()
	})

	return err
}

func TestReceive(t *testing.T) {
	t.Parallel()

	f := &fake{}
	h := newHandler(t, f)
	user := uuid.New()

	_, err := h.Receive(context.Background(), &federationv1.ReceiveRequest{
		Method:  "POST",
		Path:    "/ap/users/" + user.String() + "/inbox",
		Headers: map[string]string{"Signature": "x", "Host": "freenet.example"},
		Body:    []byte(`{"type":"Follow","actor":"https://m.example/u","object":"x"}`),
		UserId:  user.String(),
	})
	if err != nil {
		t.Fatalf("Receive: %v", err)
	}

	if f.to != user || f.received.Headers["signature"] != "x" || f.received.Headers["host"] != "freenet.example" || f.received.Method != "POST" {
		t.Fatalf("the service got %+v for %v", f.received, f.to)
	}

	tests := []struct {
		name string
		req  *federationv1.ReceiveRequest
		err  error
		want codes.Code
	}{
		{"not an activity", &federationv1.ReceiveRequest{Body: []byte(`nonsense`)}, nil, codes.InvalidArgument},
		{"too large", &federationv1.ReceiveRequest{Body: make([]byte, 257<<10)}, nil, codes.InvalidArgument},
		{"bad inbox", &federationv1.ReceiveRequest{Body: []byte(`{"type":"Follow","actor":"https://m.example/u"}`), UserId: "x"}, nil, codes.InvalidArgument},
		{"forged", &federationv1.ReceiveRequest{Body: []byte(`{"type":"Follow","actor":"https://m.example/u"}`)}, fedi.ErrBadSignature, codes.Unauthenticated},
		{"too many", &federationv1.ReceiveRequest{Body: []byte(`{"type":"Follow","actor":"https://m.example/u"}`)}, fedi.ErrTooMany, codes.ResourceExhausted},
		{"no such page", &federationv1.ReceiveRequest{Body: []byte(`{"type":"Follow","actor":"https://m.example/u"}`)}, fedi.ErrNotFederated, codes.NotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler(t, &fake{err: tt.err})

			err := call(func() (any, error) { return h.Receive(context.Background(), tt.req) })
			if status.Code(err) != tt.want {
				t.Fatalf("Receive = %v, want %s", err, tt.want)
			}
		})
	}
}

func TestPostChanged(t *testing.T) {
	t.Parallel()

	f := &fake{}
	h := newHandler(t, f)
	post, author := uuid.New(), uuid.New()

	_, err := h.PostChanged(context.Background(), &federationv1.PostChangedRequest{
		Change: federationv1.PostChange_POST_CHANGE_EDITED, PostId: post.String(), AuthorId: author.String(),
	})
	if err != nil {
		t.Fatalf("PostChanged: %v", err)
	}

	if f.changed != (service.PostEvent{Change: service.PostEdited, PostID: post, AuthorID: author}) {
		t.Fatalf("the service got %+v", f.changed)
	}

	for _, req := range []*federationv1.PostChangedRequest{
		{PostId: post.String(), AuthorId: author.String()},
		{Change: federationv1.PostChange_POST_CHANGE_DELETED, PostId: "x", AuthorId: author.String()},
	} {
		if _, err = h.PostChanged(context.Background(), req); status.Code(err) != codes.InvalidArgument {
			t.Errorf("PostChanged(%v) = %v, want InvalidArgument", req, err)
		}
	}
}

func TestGetActor(t *testing.T) {
	t.Parallel()

	h := newHandler(t, &fake{err: fedi.ErrNotFederated})

	err := call(func() (any, error) {
		return h.GetActor(context.Background(), &federationv1.GetActorRequest{UserId: uuid.NewString()})
	})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("GetActor of a page not open = %v", err)
	}

	if _, err = h.GetActor(context.Background(), &federationv1.GetActorRequest{UserId: "x"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("GetActor of a bad ID = %v", err)
	}

	if _, err = h.GetFederation(context.Background(), &federationv1.GetFederationRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("GetFederation without a token = %v", err)
	}
}
