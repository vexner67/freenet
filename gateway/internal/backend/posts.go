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

// Create writes a post and returns it without the profiles of Author and
// Wall: the caller knows who they are.
func (p *Posts) Create(ctx context.Context, accessToken string, np model.NewPost) (model.Post, error) {
	resp, err := p.api.CreatePost(withToken(ctx, accessToken), &postv1.CreatePostRequest{
		WallId:   np.WallID,
		Text:     np.Text,
		PhotoIds: np.PhotoIDs,
		ClubWall: np.ClubWall,
		FromWall: np.FromWall,
		Signed:   np.Signed,

		AvatarUpdate: np.AvatarUpdate,
	})
	if err != nil {
		return model.Post{}, errs.Wrap("p.api.CreatePost", fromStatus(err))
	}

	return toPost(resp.GetPost()), nil
}

// Repost shares the post on the caller's wall and returns the repost.
func (p *Posts) Repost(ctx context.Context, accessToken, postID, text string) (model.Post, error) {
	resp, err := p.api.RepostPost(withToken(ctx, accessToken), &postv1.RepostPostRequest{PostId: postID, Text: text})
	if err != nil {
		return model.Post{}, errs.Wrap("p.api.RepostPost", fromStatus(err))
	}

	return toPost(resp.GetPost()), nil
}

func (p *Posts) Delete(ctx context.Context, accessToken, id string) error {
	if _, err := p.api.DeletePost(withToken(ctx, accessToken), &postv1.DeletePostRequest{Id: id}); err != nil {
		return errs.Wrap("p.api.DeletePost", fromStatus(err))
	}

	return nil
}

// Edit changes the text and photos of the caller's post and returns it.
func (p *Posts) Edit(ctx context.Context, accessToken, id, text string, photoIDs []string) (model.Post, error) {
	resp, err := p.api.EditPost(withToken(ctx, accessToken), &postv1.EditPostRequest{Id: id, Text: text, PhotoIds: photoIDs})
	if err != nil {
		return model.Post{}, errs.Wrap("p.api.EditPost", fromStatus(err))
	}

	return toPost(resp.GetPost()), nil
}

// Pin shows the post first on its wall, which is the caller's.
func (p *Posts) Pin(ctx context.Context, accessToken, id string) error {
	if _, err := p.api.PinPost(withToken(ctx, accessToken), &postv1.PinPostRequest{Id: id}); err != nil {
		return errs.Wrap("p.api.PinPost", fromStatus(err))
	}

	return nil
}

func (p *Posts) Unpin(ctx context.Context, accessToken, id string) error {
	if _, err := p.api.UnpinPost(withToken(ctx, accessToken), &postv1.UnpinPostRequest{Id: id}); err != nil {
		return errs.Wrap("p.api.UnpinPost", fromStatus(err))
	}

	return nil
}

// Get returns the post without the profiles of Author and Wall; the caller
// looks them up. So do List, Feed and GetMany.
func (p *Posts) Get(ctx context.Context, accessToken, id string) (model.Post, error) {
	resp, err := p.api.GetPost(withToken(ctx, accessToken), &postv1.GetPostRequest{Id: id})
	if err != nil {
		return model.Post{}, errs.Wrap("p.api.GetPost", fromStatus(err))
	}

	return toPost(resp.GetPost()), nil
}

// List is the wall of wallID, whoever wrote the posts.
func (p *Posts) List(ctx context.Context, accessToken, wallID, cursor string, limit int) (model.PostPage, error) {
	resp, err := p.api.ListPosts(withToken(ctx, accessToken), &postv1.ListPostsRequest{
		WallId: wallID,
		Cursor: cursor,
		Limit:  toLimit(limit),
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

// Feed is the posts of several authors on any walls and of several walls.
func (p *Posts) Feed(ctx context.Context, accessToken string, sources model.FeedSources, cursor string, limit int) (model.PostPage, error) {
	resp, err := p.api.ListFeed(withToken(ctx, accessToken), &postv1.ListFeedRequest{
		AuthorIds: sources.Authors,
		WallIds:   sources.Walls,
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

// GetMany returns the posts that exist, in no order.
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
func (p *Posts) CreateComment(ctx context.Context, accessToken, postID string, c model.NewComment) (model.Comment, error) {
	resp, err := p.api.CreateComment(withToken(ctx, accessToken), &postv1.CreateCommentRequest{
		PostId:    postID,
		Text:      c.Text,
		PhotoIds:  c.PhotoIDs,
		ReplyToId: c.ReplyTo,
		FromWall:  c.FromWall,
	})
	if err != nil {
		return model.Comment{}, errs.Wrap("p.api.CreateComment", fromStatus(err))
	}

	return toComment(resp.GetComment()), nil
}

// LikeComment and UnlikeComment return how many likes the comment has now.
func (p *Posts) LikeComment(ctx context.Context, accessToken, id string) (int, error) {
	resp, err := p.api.LikeComment(withToken(ctx, accessToken), &postv1.LikeCommentRequest{CommentId: id})
	if err != nil {
		return 0, errs.Wrap("p.api.LikeComment", fromStatus(err))
	}

	return int(resp.GetLikeCount()), nil
}

func (p *Posts) UnlikeComment(ctx context.Context, accessToken, id string) (int, error) {
	resp, err := p.api.UnlikeComment(withToken(ctx, accessToken), &postv1.UnlikeCommentRequest{CommentId: id})
	if err != nil {
		return 0, errs.Wrap("p.api.UnlikeComment", fromStatus(err))
	}

	return int(resp.GetLikeCount()), nil
}

func (p *Posts) DeleteComment(ctx context.Context, accessToken, id string) error {
	if _, err := p.api.DeleteComment(withToken(ctx, accessToken), &postv1.DeleteCommentRequest{Id: id}); err != nil {
		return errs.Wrap("p.api.DeleteComment", fromStatus(err))
	}

	return nil
}

// Comments returns comments without Author; the caller looks the authors up.
// Comments are a page of a post's comments; with a token, Liked says
// which the caller likes.
func (p *Posts) Comments(ctx context.Context, accessToken, postID, cursor string, limit int) (model.CommentPage, error) {
	resp, err := p.api.ListComments(withToken(ctx, accessToken), &postv1.ListCommentsRequest{
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

// Suggest suggests a post for the club wallID; the caller has checked that
// the user may and that the photos are theirs.
func (p *Posts) Suggest(ctx context.Context, accessToken, wallID, text string, photoIDs []string) (model.Suggestion, error) {
	resp, err := p.api.SuggestPost(withToken(ctx, accessToken), &postv1.SuggestPostRequest{WallId: wallID, Text: text, PhotoIds: photoIDs})
	if err != nil {
		return model.Suggestion{}, errs.Wrap("p.api.SuggestPost", fromStatus(err))
	}

	return toSuggestion(resp.GetSuggestion()), nil
}

// Suggestions pages the suggestions for a club's wall: the caller's own
// with mine, all of them for its admins.
func (p *Posts) Suggestions(ctx context.Context, accessToken, wallID string, mine bool, cursor string, limit int) (model.SuggestionPage, error) {
	resp, err := p.api.ListSuggestions(withToken(ctx, accessToken), &postv1.ListSuggestionsRequest{
		WallId: wallID, Mine: mine, Cursor: cursor, Limit: toLimit(limit),
	})
	if err != nil {
		return model.SuggestionPage{}, errs.Wrap("p.api.ListSuggestions", fromStatus(err))
	}

	page := model.SuggestionPage{
		Suggestions: make([]model.Suggestion, 0, len(resp.GetSuggestions())),
		NextCursor:  resp.GetNextCursor(),
		Total:       int(resp.GetTotal()),
	}
	for _, s := range resp.GetSuggestions() {
		page.Suggestions = append(page.Suggestions, toSuggestion(s))
	}

	return page, nil
}

// PublishSuggestion makes a post for the club of a suggestion, saying text
// and, signed, naming its author.
func (p *Posts) PublishSuggestion(ctx context.Context, accessToken, id, text string, signed bool) (model.Post, error) {
	resp, err := p.api.PublishSuggestion(withToken(ctx, accessToken), &postv1.PublishSuggestionRequest{Id: id, Text: text, Signed: signed})
	if err != nil {
		return model.Post{}, errs.Wrap("p.api.PublishSuggestion", fromStatus(err))
	}

	return toPost(resp.GetPost()), nil
}

// DeleteSuggestion withdraws or declines a suggestion.
func (p *Posts) DeleteSuggestion(ctx context.Context, accessToken, id string) error {
	if _, err := p.api.DeleteSuggestion(withToken(ctx, accessToken), &postv1.DeleteSuggestionRequest{Id: id}); err != nil {
		return errs.Wrap("p.api.DeleteSuggestion", fromStatus(err))
	}

	return nil
}

func toSuggestion(s *postv1.Suggestion) model.Suggestion {
	return model.Suggestion{
		ID:        s.GetId(),
		WallID:    s.GetWallId(),
		AuthorID:  s.GetAuthorId(),
		Text:      s.GetText(),
		PhotoIDs:  s.GetPhotoIds(),
		CreatedAt: s.GetCreatedAt().AsTime(),
	}
}

func toPost(p *postv1.Post) model.Post {
	return model.Post{
		ID:        p.GetId(),
		Author:    model.Author{ID: p.GetAuthorId()},
		Wall:      model.Author{ID: p.GetWallId()},
		Text:      p.GetText(),
		PhotoIDs:  p.GetPhotoIds(),
		CreatedAt: p.GetCreatedAt().AsTime(),
		Likes:     int(p.GetLikeCount()),
		Comments:  int(p.GetCommentCount()),
		Reposts:   int(p.GetRepostCount()),
		Liked:     p.GetLiked(),
		RepostOf:  p.GetRepostOf(),
		// Unset timestamps are zero times, not the Unix epoch.
		EditedAt:      fromTimestamp(p.GetEditedAt()),
		EditableUntil: fromTimestamp(p.GetEditableUntil()),
		Pinned:        p.GetPinned(),
		FromWall:      p.GetFromWall(),
		Signed:        p.GetSigned(),
		AvatarUpdate:  p.GetAvatarUpdate(),
		Mine:          p.GetMine(),
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
		PhotoIDs:  c.GetPhotoIds(),
		CreatedAt: c.GetCreatedAt().AsTime(),
		ReplyTo:   c.GetReplyToId(),
		Likes:     int(c.GetLikeCount()),
		Liked:     c.GetLiked(),
		FromWall:  c.GetFromWall(),
		Mine:      c.GetMine(),
	}
}
