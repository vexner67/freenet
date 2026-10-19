package backend

import (
	"context"

	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/pkg/errs"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
)

// Stories is rest.Stories over the post service.
type Stories struct {
	api postv1.PostServiceClient
}

func NewStories(api postv1.PostServiceClient) *Stories {
	return &Stories{
		api: api,
	}
}

// Tell tells a story of the caller with a photo of theirs.
func (s *Stories) Tell(ctx context.Context, accessToken, photoID, text string) (model.Story, error) {
	resp, err := s.api.CreateStory(withToken(ctx, accessToken), &postv1.CreateStoryRequest{PhotoId: photoID, Text: text})
	if err != nil {
		return model.Story{}, errs.Wrap("s.api.CreateStory", fromStatus(err))
	}

	return toStory(resp.GetStory()), nil
}

// Feed is the stories above the caller's news, grouped by author.
func (s *Stories) Feed(ctx context.Context, accessToken string, hidden, ignored []string) ([]model.Story, error) {
	resp, err := s.api.ListFeedStories(withToken(ctx, accessToken), &postv1.ListFeedStoriesRequest{
		HiddenAuthorIds: hidden, IgnoredWallIds: ignored,
	})
	if err != nil {
		return nil, errs.Wrap("s.api.ListFeedStories", fromStatus(err))
	}

	return toStories(resp.GetStories()), nil
}

// User is the live stories of a person, the oldest first.
func (s *Stories) User(ctx context.Context, accessToken, userID string, hidden []string) ([]model.Story, error) {
	resp, err := s.api.ListUserStories(withToken(ctx, accessToken), &postv1.ListUserStoriesRequest{
		UserId: userID, HiddenAuthorIds: hidden,
	})
	if err != nil {
		return nil, errs.Wrap("s.api.ListUserStories", fromStatus(err))
	}

	return toStories(resp.GetStories()), nil
}

// Get is a live story the caller sees.
func (s *Stories) Get(ctx context.Context, accessToken, id string) (model.Story, error) {
	resp, err := s.api.GetStory(withToken(ctx, accessToken), &postv1.GetStoryRequest{Id: id})
	if err != nil {
		return model.Story{}, errs.Wrap("s.api.GetStory", fromStatus(err))
	}

	return toStory(resp.GetStory()), nil
}

func (s *Stories) Like(ctx context.Context, accessToken, id string) error {
	if _, err := s.api.LikeStory(withToken(ctx, accessToken), &postv1.LikeStoryRequest{Id: id}); err != nil {
		return errs.Wrap("s.api.LikeStory", fromStatus(err))
	}

	return nil
}

func (s *Stories) Unlike(ctx context.Context, accessToken, id string) error {
	if _, err := s.api.UnlikeStory(withToken(ctx, accessToken), &postv1.UnlikeStoryRequest{Id: id}); err != nil {
		return errs.Wrap("s.api.UnlikeStory", fromStatus(err))
	}

	return nil
}

// Comment comments a story the caller sees.
func (s *Stories) Comment(ctx context.Context, accessToken, id, text string) (model.StoryComment, error) {
	resp, err := s.api.CreateStoryComment(withToken(ctx, accessToken), &postv1.CreateStoryCommentRequest{StoryId: id, Text: text})
	if err != nil {
		return model.StoryComment{}, errs.Wrap("s.api.CreateStoryComment", fromStatus(err))
	}

	return toStoryComment(resp.GetComment()), nil
}

// Comments pages the comments of a story the caller sees, the oldest
// first.
func (s *Stories) Comments(ctx context.Context, accessToken, id, cursor string, limit int) (model.StoryCommentPage, error) {
	resp, err := s.api.ListStoryComments(withToken(ctx, accessToken), &postv1.ListStoryCommentsRequest{
		StoryId: id, Cursor: cursor, Limit: toLimit(limit),
	})
	if err != nil {
		return model.StoryCommentPage{}, errs.Wrap("s.api.ListStoryComments", fromStatus(err))
	}

	page := model.StoryCommentPage{NextCursor: resp.GetNextCursor(), Total: int(resp.GetTotal())}
	for _, c := range resp.GetComments() {
		page.Comments = append(page.Comments, toStoryComment(c))
	}

	return page, nil
}

func (s *Stories) DeleteComment(ctx context.Context, accessToken, id string) error {
	if _, err := s.api.DeleteStoryComment(withToken(ctx, accessToken), &postv1.DeleteStoryCommentRequest{Id: id}); err != nil {
		return errs.Wrap("s.api.DeleteStoryComment", fromStatus(err))
	}

	return nil
}

func toStoryComment(p *postv1.StoryComment) model.StoryComment {
	return model.StoryComment{
		ID:        p.GetId(),
		StoryID:   p.GetStoryId(),
		AuthorID:  p.GetAuthorId(),
		Text:      p.GetText(),
		CreatedAt: p.GetCreatedAt().AsTime(),
		Mine:      p.GetMine(),
	}
}

func (s *Stories) View(ctx context.Context, accessToken, id string) error {
	if _, err := s.api.ViewStory(withToken(ctx, accessToken), &postv1.ViewStoryRequest{Id: id}); err != nil {
		return errs.Wrap("s.api.ViewStory", fromStatus(err))
	}

	return nil
}

// Viewers pages who viewed a story of the caller, the latest first.
func (s *Stories) Viewers(ctx context.Context, accessToken, id, cursor string, limit int) (model.LikerPage, error) {
	resp, err := s.api.ListStoryViewers(withToken(ctx, accessToken), &postv1.ListStoryViewersRequest{
		Id: id, Cursor: cursor, Limit: toLimit(limit),
	})
	if err != nil {
		return model.LikerPage{}, errs.Wrap("s.api.ListStoryViewers", fromStatus(err))
	}

	page := model.LikerPage{NextCursor: resp.GetNextCursor(), Total: int(resp.GetTotal())}
	for _, v := range resp.GetViewers() {
		page.UserIDs = append(page.UserIDs, v.GetUserId())
	}

	return page, nil
}

func (s *Stories) Delete(ctx context.Context, accessToken, id string) error {
	if _, err := s.api.DeleteStory(withToken(ctx, accessToken), &postv1.DeleteStoryRequest{Id: id}); err != nil {
		return errs.Wrap("s.api.DeleteStory", fromStatus(err))
	}

	return nil
}

func toStories(ps []*postv1.Story) []model.Story {
	stories := make([]model.Story, 0, len(ps))
	for _, p := range ps {
		stories = append(stories, toStory(p))
	}

	return stories
}

func toStory(p *postv1.Story) model.Story {
	return model.Story{
		ID:        p.GetId(),
		AuthorID:  p.GetAuthorId(),
		PhotoID:   p.GetPhotoId(),
		Text:      p.GetText(),
		CreatedAt: p.GetCreatedAt().AsTime(),
		ExpiresAt: p.GetExpiresAt().AsTime(),
		Viewed:    p.GetViewed(),
		Mine:      p.GetMine(),
		Views:     int(p.GetViews()),
		Liked:     p.GetLiked(),
		Likes:     int(p.GetLikeCount()),
		Comments:  int(p.GetCommentCount()),
	}
}
