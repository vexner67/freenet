// Package postclient reads the posts of the post service as a guest sees
// them. It is the only place in federation that knows the post proto
// contract.
package postclient

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/federation/internal/fedi"
	"github.com/vexner67/freenet/pkg/errs"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Client is the service.Posts; each call is bounded by timeout.
type Client struct {
	posts   postv1.PostServiceClient
	timeout time.Duration
}

func New(posts postv1.PostServiceClient, timeout time.Duration) *Client {
	return &Client{
		posts:   posts,
		timeout: timeout,
	}
}

func (c *Client) Post(ctx context.Context, id uuid.UUID) (fedi.Post, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := c.posts.GetPost(ctx, &postv1.GetPostRequest{Id: id.String()})
	switch status.Code(err) {
	case codes.OK:
	case codes.NotFound, codes.PermissionDenied:
		return fedi.Post{}, fedi.ErrNotFound
	default:
		return fedi.Post{}, errs.Wrap("c.posts.GetPost", err)
	}

	return fromProto(resp.GetPost())
}

func fromProto(p *postv1.Post) (fedi.Post, error) {
	ids := make([]uuid.UUID, 0, 3+len(p.GetPhotoIds()))

	for _, s := range append([]string{p.GetId(), p.GetAuthorId(), p.GetWallId()}, p.GetPhotoIds()...) {
		id, err := uuid.Parse(s)
		if err != nil {
			return fedi.Post{}, errs.Wrap("uuid.Parse", err)
		}

		ids = append(ids, id)
	}

	var edited time.Time
	if p.GetEditedAt() != nil {
		edited = p.GetEditedAt().AsTime()
	}

	return fedi.Post{
		ID:       ids[0],
		AuthorID: ids[1],
		WallID:   ids[2],
		Text:     p.GetText(),
		PhotoIDs: ids[3:],
		Created:  p.GetCreatedAt().AsTime(),
		Edited:   edited,
		NSFW:     p.GetNsfw(),
		FromWall: p.GetFromWall(),
		Repost:   p.GetRepostOf() != "" || p.GetSharedPhotoId() != "",
		Paid:     p.GetPrice() > 0,
		Archived: p.GetArchived(),
	}, nil
}
