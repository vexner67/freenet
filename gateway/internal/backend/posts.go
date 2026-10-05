package backend

import (
	"context"

	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/pkg/errs"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
)

type Posts struct {
	api postv1.PostServiceClient
}

func NewPosts(api postv1.PostServiceClient) *Posts {
	return &Posts{
		api: api,
	}
}

// Create returns the post without Author: the caller knows who it is.
func (p *Posts) Create(ctx context.Context, accessToken, text string) (model.Post, error) {
	resp, err := p.api.CreatePost(withToken(ctx, accessToken), &postv1.CreatePostRequest{Text: text})
	if err != nil {
		return model.Post{}, errs.Wrap("p.api.CreatePost", fromStatus(err))
	}

	return toPost(resp.GetPost()), nil
}

func (p *Posts) Delete(ctx context.Context, accessToken, id string) error {
	if _, err := p.api.DeletePost(withToken(ctx, accessToken), &postv1.DeletePostRequest{Id: id}); err != nil {
		return errs.Wrap("p.api.DeletePost", fromStatus(err))
	}

	return nil
}

// List returns posts without Author: they all belong to authorID.
func (p *Posts) List(ctx context.Context, authorID, cursor string, limit int) (model.PostPage, error) {
	resp, err := p.api.ListPosts(ctx, &postv1.ListPostsRequest{
		AuthorId: authorID,
		Cursor:   cursor,
		Limit:    int32(min(max(limit, 0), 1<<10)), //nolint:gosec // clamped above
	})
	if err != nil {
		return model.PostPage{}, errs.Wrap("p.api.ListPosts", fromStatus(err))
	}

	page := model.PostPage{
		Posts:      make([]model.Post, 0, len(resp.GetPosts())),
		NextCursor: resp.GetNextCursor(),
		Total:      int(resp.GetTotal()),
	}

	for _, post := range resp.GetPosts() {
		page.Posts = append(page.Posts, toPost(post))
	}

	return page, nil
}

// Feed returns posts without Author; the caller looks the authors up.
func (p *Posts) Feed(ctx context.Context, authorIDs []string, cursor string, limit int) (model.PostPage, error) {
	resp, err := p.api.ListFeed(ctx, &postv1.ListFeedRequest{
		AuthorIds: authorIDs,
		Cursor:    cursor,
		Limit:     int32(min(max(limit, 0), 1<<10)), //nolint:gosec // clamped above
	})
	if err != nil {
		return model.PostPage{}, errs.Wrap("p.api.ListFeed", fromStatus(err))
	}

	page := model.PostPage{
		Posts:      make([]model.Post, 0, len(resp.GetPosts())),
		NextCursor: resp.GetNextCursor(),
	}

	for _, post := range resp.GetPosts() {
		page.Posts = append(page.Posts, toPost(post))
	}

	return page, nil
}

func toPost(p *postv1.Post) model.Post {
	return model.Post{
		ID:        p.GetId(),
		Author:    model.Author{ID: p.GetAuthorId()},
		Text:      p.GetText(),
		CreatedAt: p.GetCreatedAt().AsTime(),
	}
}
