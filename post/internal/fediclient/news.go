package fediclient

import (
	"context"
	"github.com/google/uuid"
	federationv1 "github.com/vexner67/freenet/federation/api/federation/v1"
	"github.com/vexner67/freenet/pkg/errs"
	"github.com/vexner67/freenet/post/internal/post"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"time"
)

// News checks references using the caller's authenticated session.
type News struct {
	api     federationv1.FederationServiceClient
	timeout time.Duration
}

func NewNews(api federationv1.FederationServiceClient, timeout time.Duration) *News {
	return &News{api: api, timeout: timeout}
}
func (n *News) Check(ctx context.Context, id uuid.UUID) error {
	ctx, cancel := context.WithTimeout(ctx, n.timeout)
	defer cancel()
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		for _, token := range md.Get("authorization") {
			ctx = metadata.AppendToOutgoingContext(ctx, "authorization", token)
		}
	}
	_, err := n.api.GetNewsPost(ctx, &federationv1.GetNewsPostRequest{Id: id.String()})
	if status.Code(err) == codes.NotFound {
		return post.ErrNotFound
	}
	return errs.Wrap("n.api.GetNewsPost", err)
}
