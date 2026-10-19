package grpc

import (
	"context"

	"github.com/google/uuid"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// StoryService keeps the stories people show for a day.
type StoryService interface {
	Tell(ctx context.Context, authorID, photoID uuid.UUID, text post.StoryText) (*post.Story, error)
	Feed(ctx context.Context, audience service.StoryAudience) (service.StoryPage, error)
	User(ctx context.Context, audience service.StoryAudience, authorID uuid.UUID) (service.StoryPage, error)
	Get(ctx context.Context, viewerID, id uuid.UUID) (service.StoryPage, error)
	View(ctx context.Context, viewerID, id uuid.UUID) error
	Viewers(ctx context.Context, requesterID, id uuid.UUID, after *service.ViewCursor, size int) (service.ViewerPage, error)
	Delete(ctx context.Context, requesterID, id uuid.UUID) error
	Like(ctx context.Context, userID, id uuid.UUID) error
	Unlike(ctx context.Context, userID, id uuid.UUID) error
	Comment(ctx context.Context, authorID, id uuid.UUID, text post.CommentText) (*post.StoryComment, error)
	Comments(ctx context.Context, viewerID, id, after uuid.UUID, size int) (service.StoryCommentPage, error)
	DeleteComment(ctx context.Context, requesterID, id uuid.UUID) error
}

func (h *PostHandler) CreateStory(ctx context.Context, req *postv1.CreateStoryRequest) (*postv1.CreateStoryResponse, error) {
	authorID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	photoID, err := uuid.Parse(req.GetPhotoId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid photo id")
	}

	text, err := post.NewStoryText(req.GetText())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	story, err := h.storyService.Tell(ctx, authorID, photoID, text)
	if err != nil {
		return nil, err
	}

	return &postv1.CreateStoryResponse{Story: toProtoStory(story, service.StoryPage{}, authorID)}, nil
}

func (h *PostHandler) ListFeedStories(ctx context.Context, req *postv1.ListFeedStoriesRequest) (*postv1.ListFeedStoriesResponse, error) {
	viewer, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	if len(req.GetHiddenAuthorIds()) > service.MaxFeedHidden || len(req.GetIgnoredWallIds()) > service.MaxFeedIgnored {
		return nil, status.Error(codes.InvalidArgument, "too many authors")
	}

	hidden, err := parseFeedIDs(req.GetHiddenAuthorIds(), "invalid author id")
	if err != nil {
		return nil, err
	}

	ignored, err := parseFeedIDs(req.GetIgnoredWallIds(), "invalid wall id")
	if err != nil {
		return nil, err
	}

	page, err := h.storyService.Feed(ctx, service.StoryAudience{Viewer: viewer, Hidden: hidden, Ignored: ignored})
	if err != nil {
		return nil, err
	}

	return &postv1.ListFeedStoriesResponse{Stories: toProtoStories(page, viewer)}, nil
}

func (h *PostHandler) ListUserStories(ctx context.Context, req *postv1.ListUserStoriesRequest) (*postv1.ListUserStoriesResponse, error) {
	viewer, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	authorID, err := uuid.Parse(req.GetUserId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user id")
	}

	if len(req.GetHiddenAuthorIds()) > service.MaxFeedHidden {
		return nil, status.Error(codes.InvalidArgument, "too many authors")
	}

	hidden, err := parseFeedIDs(req.GetHiddenAuthorIds(), "invalid author id")
	if err != nil {
		return nil, err
	}

	page, err := h.storyService.User(ctx, service.StoryAudience{Viewer: viewer, Hidden: hidden}, authorID)
	if err != nil {
		return nil, err
	}

	return &postv1.ListUserStoriesResponse{Stories: toProtoStories(page, viewer)}, nil
}

func (h *PostHandler) GetStory(ctx context.Context, req *postv1.GetStoryRequest) (*postv1.GetStoryResponse, error) {
	viewer, id, err := callerAndStory(ctx, req.GetId())
	if err != nil {
		return nil, err
	}

	page, err := h.storyService.Get(ctx, viewer, id)
	if err != nil {
		return nil, err
	}

	return &postv1.GetStoryResponse{Story: toProtoStory(page.Stories[0], page, viewer)}, nil
}

func (h *PostHandler) ViewStory(ctx context.Context, req *postv1.ViewStoryRequest) (*emptypb.Empty, error) {
	viewer, id, err := callerAndStory(ctx, req.GetId())
	if err != nil {
		return nil, err
	}

	if err = h.storyService.View(ctx, viewer, id); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *PostHandler) ListStoryViewers(ctx context.Context, req *postv1.ListStoryViewersRequest) (*postv1.ListStoryViewersResponse, error) {
	requesterID, id, err := callerAndStory(ctx, req.GetId())
	if err != nil {
		return nil, err
	}

	at, viewer, ok, err := parseTimedCursor(req.GetCursor())
	if err != nil {
		return nil, err
	}

	var after *service.ViewCursor
	if ok {
		after = &service.ViewCursor{ViewedAt: at, ViewerID: viewer}
	}

	page, err := h.storyService.Viewers(ctx, requesterID, id, after, int(req.GetLimit()))
	if err != nil {
		return nil, err
	}

	resp := &postv1.ListStoryViewersResponse{
		Viewers: make([]*postv1.StoryViewer, 0, len(page.Views)),
		Total:   int32(page.Total), //nolint:gosec // views of a story are far fewer than 2^31
	}
	for _, v := range page.Views {
		resp.Viewers = append(resp.Viewers, &postv1.StoryViewer{UserId: v.ViewerID.String(), ViewedAt: timestamppb.New(v.ViewedAt)})
	}
	if page.Next != nil {
		resp.NextCursor = timedCursor(page.Next.ViewedAt, page.Next.ViewerID)
	}

	return resp, nil
}

func (h *PostHandler) DeleteStory(ctx context.Context, req *postv1.DeleteStoryRequest) (*emptypb.Empty, error) {
	requesterID, id, err := callerAndStory(ctx, req.GetId())
	if err != nil {
		return nil, err
	}

	if err = h.storyService.Delete(ctx, requesterID, id); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func callerAndStory(ctx context.Context, rawID string) (uuid.UUID, uuid.UUID, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	id, err := uuid.Parse(rawID)
	if err != nil {
		return uuid.Nil, uuid.Nil, status.Error(codes.InvalidArgument, "invalid story id")
	}

	return userID, id, nil
}

func toProtoStories(page service.StoryPage, viewerID uuid.UUID) []*postv1.Story {
	stories := make([]*postv1.Story, 0, len(page.Stories))
	for _, s := range page.Stories {
		stories = append(stories, toProtoStory(s, page, viewerID))
	}

	return stories
}

func toProtoStory(s *post.Story, page service.StoryPage, viewerID uuid.UUID) *postv1.Story {
	return &postv1.Story{
		Id:           s.ID().String(),
		AuthorId:     s.AuthorID().String(),
		PhotoId:      s.PhotoID().String(),
		Text:         s.Text().String(),
		CreatedAt:    timestamppb.New(s.CreatedAt()),
		ExpiresAt:    timestamppb.New(s.ExpiresAt()),
		Viewed:       page.Viewed[s.ID()],
		Mine:         s.Mine(viewerID),
		Views:        int32(page.Views[s.ID()]), //nolint:gosec // views of a story are far fewer than 2^31
		Liked:        page.Liked[s.ID()],
		LikeCount:    int32(page.Likes[s.ID()]),    //nolint:gosec // likes of a story are far fewer than 2^31
		CommentCount: int32(page.Comments[s.ID()]), //nolint:gosec // comments of a story are far fewer than 2^31
	}
}
