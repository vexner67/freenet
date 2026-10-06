package backend

import (
	"context"

	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/pkg/errs"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
)

// Posts talks to the post service. Methods that read take an optional
// access token: with it, posts tell whether the viewer likes them.
type Posts struct {
	api postv1.PostServiceClient
}

func NewPosts(api postv1.PostServiceClient) *Posts {
	return &Posts{
		api: api,
	}
}

// Create returns the post without Author: the caller knows who it is.
func (p *Posts) Create(ctx context.Context, accessToken, text string, photoIDs []string) (model.Post, error) {
	resp, err := p.api.CreatePost(withToken(ctx, accessToken), &postv1.CreatePostRequest{Text: text, PhotoIds: photoIDs})
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

// Get returns the post without Author; the caller looks the author up.
func (p *Posts) Get(ctx context.Context, accessToken, id string) (model.Post, error) {
	resp, err := p.api.GetPost(withToken(ctx, accessToken), &postv1.GetPostRequest{Id: id})
	if err != nil {
		return model.Post{}, errs.Wrap("p.api.GetPost", fromStatus(err))
	}

	return toPost(resp.GetPost()), nil
}

// List returns posts without Author: they all belong to authorID.
func (p *Posts) List(ctx context.Context, accessToken, authorID, cursor string, limit int) (model.PostPage, error) {
	resp, err := p.api.ListPosts(withToken(ctx, accessToken), &postv1.ListPostsRequest{
		AuthorId: authorID,
		Cursor:   cursor,
		Limit:    toLimit(limit),
	})
	if err != nil {
		return model.PostPage{}, errs.Wrap("p.api.ListPosts", fromStatus(err))
	}

	return model.PostPage{
		Posts:      toPosts(resp.GetPosts()),
		NextCursor: resp.GetNextCursor(),
		Total:      int(resp.GetTotal()),
	}, nil
}

// Feed returns posts without Author; the caller looks the authors up.
func (p *Posts) Feed(ctx context.Context, accessToken string, authorIDs []string, cursor string, limit int) (model.PostPage, error) {
	resp, err := p.api.ListFeed(withToken(ctx, accessToken), &postv1.ListFeedRequest{
		AuthorIds: authorIDs,
		Cursor:    cursor,
		Limit:     toLimit(limit),
	})
	if err != nil {
		return model.PostPage{}, errs.Wrap("p.api.ListFeed", fromStatus(err))
	}

	return model.PostPage{
		Posts:      toPosts(resp.GetPosts()),
		NextCursor: resp.GetNextCursor(),
	}, nil
}

// GetMany returns the posts that exist, without Author, in no order.
func (p *Posts) GetMany(ctx context.Context, ids []string) ([]model.Post, error) {
	resp, err := p.api.GetPosts(ctx, &postv1.GetPostsRequest{Ids: ids})
	if err != nil {
		return nil, errs.Wrap("p.api.GetPosts", fromStatus(err))
	}

	return toPosts(resp.GetPosts()), nil
}

// CommentsByID returns the comments that exist, without Author, in no order.
func (p *Posts) CommentsByID(ctx context.Context, ids []string) ([]model.Comment, error) {
	resp, err := p.api.GetComments(ctx, &postv1.GetCommentsRequest{Ids: ids})
	if err != nil {
		return nil, errs.Wrap("p.api.GetComments", fromStatus(err))
	}

	comments := make([]model.Comment, 0, len(resp.GetComments()))
	for _, c := range resp.GetComments() {
		comments = append(comments, toComment(c))
	}

	return comments, nil
}

// Like returns how many likes the post has now.
func (p *Posts) Like(ctx context.Context, accessToken, postID string) (int, error) {
	resp, err := p.api.LikePost(withToken(ctx, accessToken), &postv1.LikePostRequest{PostId: postID})
	if err != nil {
		return 0, errs.Wrap("p.api.LikePost", fromStatus(err))
	}

	return int(resp.GetLikeCount()), nil
}

func (p *Posts) Unlike(ctx context.Context, accessToken, postID string) (int, error) {
	resp, err := p.api.UnlikePost(withToken(ctx, accessToken), &postv1.UnlikePostRequest{PostId: postID})
	if err != nil {
		return 0, errs.Wrap("p.api.UnlikePost", fromStatus(err))
	}

	return int(resp.GetLikeCount()), nil
}

// CreateComment returns the comment without Author: the caller knows who it is.
func (p *Posts) CreateComment(ctx context.Context, accessToken, postID, text string) (model.Comment, error) {
	resp, err := p.api.CreateComment(withToken(ctx, accessToken), &postv1.CreateCommentRequest{PostId: postID, Text: text})
	if err != nil {
		return model.Comment{}, errs.Wrap("p.api.CreateComment", fromStatus(err))
	}

	return toComment(resp.GetComment()), nil
}

func (p *Posts) DeleteComment(ctx context.Context, accessToken, id string) error {
	if _, err := p.api.DeleteComment(withToken(ctx, accessToken), &postv1.DeleteCommentRequest{Id: id}); err != nil {
		return errs.Wrap("p.api.DeleteComment", fromStatus(err))
	}

	return nil
}

// Comments returns comments without Author; the caller looks the authors up.
func (p *Posts) Comments(ctx context.Context, postID, cursor string, limit int) (model.CommentPage, error) {
	resp, err := p.api.ListComments(ctx, &postv1.ListCommentsRequest{
		PostId: postID,
		Cursor: cursor,
		Limit:  toLimit(limit),
	})
	if err != nil {
		return model.CommentPage{}, errs.Wrap("p.api.ListComments", fromStatus(err))
	}

	page := model.CommentPage{
		Comments:   make([]model.Comment, 0, len(resp.GetComments())),
		NextCursor: resp.GetNextCursor(),
		Total:      int(resp.GetTotal()),
	}

	for _, c := range resp.GetComments() {
		page.Comments = append(page.Comments, toComment(c))
	}

	return page, nil
}

func toLimit(limit int) int32 {
	return int32(min(max(limit, 0), 1<<10)) //nolint:gosec // clamped
}

func toPost(p *postv1.Post) model.Post {
	return model.Post{
		ID:        p.GetId(),
		Author:    model.Author{ID: p.GetAuthorId()},
		Text:      p.GetText(),
		PhotoIDs:  p.GetPhotoIds(),
		CreatedAt: p.GetCreatedAt().AsTime(),
		Likes:     int(p.GetLikeCount()),
		Comments:  int(p.GetCommentCount()),
		Liked:     p.GetLiked(),
	}
}

func toPosts(posts []*postv1.Post) []model.Post {
	out := make([]model.Post, 0, len(posts))
	for _, p := range posts {
		out = append(out, toPost(p))
	}

	return out
}

func toComment(c *postv1.Comment) model.Comment {
	return model.Comment{
		ID:        c.GetId(),
		PostID:    c.GetPostId(),
		Author:    model.Author{ID: c.GetAuthorId()},
		Text:      c.GetText(),
		CreatedAt: c.GetCreatedAt().AsTime(),
	}
}
