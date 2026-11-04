package fediclient

import (
	"context"
	"errors"
	"github.com/google/uuid"
	federationv1 "github.com/vexner67/freenet/federation/api/federation/v1"
	"github.com/vexner67/freenet/post/internal/post"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"testing"
	"time"
)

type newsAPI struct {
	federationv1.FederationServiceClient
	token    string
	id       string
	deadline bool
	err      error
}

func (f *newsAPI) GetNewsPost(ctx context.Context, req *federationv1.GetNewsPostRequest, _ ...grpc.CallOption) (*federationv1.GetNewsPostResponse, error) {
	md, _ := metadata.FromOutgoingContext(ctx)
	if values := md.Get("authorization"); len(values) > 0 {
		f.token = values[0]
	}
	_, f.deadline = ctx.Deadline()
	f.id = req.GetId()
	return &federationv1.GetNewsPostResponse{}, f.err
}
func TestNewsCheck(t *testing.T) {
	t.Parallel()
	id := uuid.New()
	for _, tc := range []struct {
		name    string
		failure error
		missing bool
	}{
		{name: "exists"}, {name: "deleted", failure: status.Error(codes.NotFound, "gone"), missing: true}, {name: "unavailable", failure: status.Error(codes.Unavailable, "offline")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &newsAPI{err: tc.failure}
			ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer test-session"))
			err := NewNews(f, time.Second).Check(ctx, id)
			if f.token != "Bearer test-session" || f.id != id.String() || !f.deadline {
				t.Fatalf("request: %+v", f)
			}
			if tc.missing {
				if !errors.Is(err, post.ErrNotFound) {
					t.Fatal(err)
				}
			} else if status.Code(err) != status.Code(tc.failure) {
				t.Fatal(err)
			}
		})
	}
}
