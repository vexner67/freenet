package fediclient

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	federationv1 "github.com/vexner67/freenet/federation/api/federation/v1"
	"github.com/vexner67/freenet/pkg/outbox"
	"github.com/vexner67/freenet/post/internal/post"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type fakeAPI struct {
	federationv1.FederationServiceClient
	changed  *federationv1.PostChangedRequest
	deadline bool
	err      error
}

func (f *fakeAPI) PostChanged(ctx context.Context, req *federationv1.PostChangedRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	_, f.deadline = ctx.Deadline()
	f.changed = req
	return &emptypb.Empty{}, f.err
}

type recorder struct {
	messages []outbox.Message
}

func (r *recorder) Add(_ context.Context, kind string, payload []byte) error {
	r.messages = append(r.messages, outbox.Message{Kind: kind, Payload: payload})
	return nil
}

func newPost(t *testing.T) *post.Post {
	t.Helper()

	author := uuid.New()

	content, err := post.NewContent(post.Text("привет"), nil)
	if err != nil {
		t.Fatal(err)
	}

	p, err := post.New(author, author, content, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	return p
}

func TestRoundTrip(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	p := newPost(t)

	tests := []struct {
		name   string
		record func(o *Outbox) error
		want   federationv1.PostChange
	}{
		{"published", func(o *Outbox) error { return o.Published(ctx, p) }, federationv1.PostChange_POST_CHANGE_PUBLISHED},
		{"edited", func(o *Outbox) error { return o.Edited(ctx, p) }, federationv1.PostChange_POST_CHANGE_EDITED},
		{"deleted", func(o *Outbox) error { return o.Deleted(ctx, p) }, federationv1.PostChange_POST_CHANGE_DELETED},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			r := &recorder{}
			if err := tt.record(NewOutbox(r)); err != nil {
				t.Fatal(err)
			}

			if len(r.messages) != 1 || Key(r.messages[0]) != p.ID().String() {
				t.Fatalf("recorded %+v, keyed by the post", r.messages)
			}

			api := &fakeAPI{}
			if err := NewDeliverer(api, time.Second).Deliver(ctx, r.messages[0]); err != nil {
				t.Fatal(err)
			}

			if api.changed.GetChange() != tt.want || api.changed.GetPostId() != p.ID().String() ||
				api.changed.GetAuthorId() != p.AuthorID().String() || !api.deadline {
				t.Fatalf("delivered %v", api.changed)
			}
		})
	}
}

func TestDeliverErrors(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	r := &recorder{}
	if err := NewOutbox(r).Published(ctx, newPost(t)); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		message   outbox.Message
		err       error
		permanent bool
	}{
		{"unknown kind", outbox.Message{Kind: "x"}, nil, true},
		{"garbage", outbox.Message{Kind: kindPostChanged, Payload: []byte{0xff}}, nil, true},
		{"rejected", r.messages[0], status.Error(codes.InvalidArgument, "x"), true},
		{"down", r.messages[0], status.Error(codes.Unavailable, "x"), false},
		{"broken", r.messages[0], errors.New("x"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := NewDeliverer(&fakeAPI{err: tt.err}, time.Second).Deliver(ctx, tt.message)
			if err == nil || outbox.IsPermanent(err) != tt.permanent {
				t.Fatalf("Deliver = %v, want permanent %v", err, tt.permanent)
			}
		})
	}
}
