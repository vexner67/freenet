package grpc

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/pkg/jwtauth"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type PostService interface {
	Create(ctx context.Context, in service.CreateInput) (*post.Post, error)
	Repost(ctx context.Context, authorID, postID uuid.UUID, text post.Text) (*post.Post, error)
	Delete(ctx context.Context, requesterID, postID uuid.UUID) error
	Edit(ctx context.Context, requesterID, postID uuid.UUID, text post.Text, photos post.Photos) (*post.Post, bool, error)
	Pin(ctx context.Context, requesterID, postID uuid.UUID) error
	Unpin(ctx context.Context, requesterID, postID uuid.UUID) error
	Get(ctx context.Context, viewerID, postID uuid.UUID) (*post.Post, bool, error)
	GetMany(ctx context.Context, ids []uuid.UUID) ([]*post.Post, error)
	ListWall(ctx context.Context, viewerID, wallID, cursor uuid.UUID, size int) (service.Page, error)
	Feed(ctx context.Context, viewerID uuid.UUID, sources service.FeedSources, cursor uuid.UUID, size int) (service.Page, error)
	Like(ctx context.Context, userID, postID uuid.UUID) (int, error)
	Unlike(ctx context.Context, userID, postID uuid.UUID) (int, error)
}

type CommentService interface {
	Create(ctx context.Context, in service.CommentInput) (*post.Comment, error)
	Like(ctx context.Context, userID, commentID uuid.UUID) (int, error)
	Unlike(ctx context.Context, userID, commentID uuid.UUID) (int, error)
	Delete(ctx context.Context, requesterID, commentID uuid.UUID) error
	List(ctx context.Context, viewerID, postID, cursor uuid.UUID, size int) (service.CommentPage, error)
	GetMany(ctx context.Context, ids []uuid.UUID) ([]*post.Comment, error)
}

type SuggestionService interface {
	Suggest(ctx context.Context, wallID, authorID uuid.UUID, content post.Content) (*post.Suggestion, error)
	List(ctx context.Context, viewerID, wallID uuid.UUID, mine bool, cursor uuid.UUID, size int) (service.SuggestionPage, error)
	Publish(ctx context.Context, requesterID, id uuid.UUID, text post.Text, signed bool) (*post.Post, error)
	Withdraw(ctx context.Context, requesterID, id uuid.UUID) error
}

// PurgeService erases a user's posts, likes and comments once the page is
// deleted for good.
type PurgeService interface {
	PurgeUser(ctx context.Context, userID uuid.UUID) error
}

// Services are the use cases behind the handler.
type Services struct {
	Posts       PostService
	Comments    CommentService
	Suggestions SuggestionService
	Purge       PurgeService
}

type PostHandler struct {
	postv1.UnimplementedPostServiceServer
	postService       PostService
	commentService    CommentService
	suggestionService SuggestionService
	purgeService      PurgeService
	logger            *slog.Logger
}

func NewPostHandler(s Services, logger *slog.Logger) *PostHandler {
	return &PostHandler{
		postService:       s.Posts,
		commentService:    s.Comments,
		suggestionService: s.Suggestions,
		purgeService:      s.Purge,
		logger:            logger,
	}
}

func (h *PostHandler) PurgeUser(ctx context.Context, req *postv1.PurgeUserRequest) (*emptypb.Empty, error) {
	userID, err := uuid.Parse(req.GetUserId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user id")
	}

	if err = h.purgeService.PurgeUser(ctx, userID); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *PostHandler) CreatePost(ctx context.Context, req *postv1.CreatePostRequest) (*postv1.CreatePostResponse, error) {
	authorID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	content, err := fromProtoContent(req)
	if err != nil {
		return nil, err
	}

	wallID := authorID
	if req.GetWallId() != "" {
		if wallID, err = uuid.Parse(req.GetWallId()); err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid wall id")
		}
	}

	p, err := h.postService.Create(ctx, service.CreateInput{
		WallID:   wallID,
		AuthorID: authorID,
		Content:  content,
		ClubWall: req.GetClubWall(),
		FromWall: req.GetFromWall(),
		Signed:   req.GetSigned(),
	})
	if err != nil {
		return nil, err
	}

	return &postv1.CreatePostResponse{Post: toProtoPost(p, false, viewerID(ctx))}, nil
}

func (h *PostHandler) RepostPost(ctx context.Context, req *postv1.RepostPostRequest) (*postv1.RepostPostResponse, error) {
	authorID, postID, err := likeTarget(ctx, req.GetPostId())
	if err != nil {
		return nil, err
	}

	text, err := post.NewText(req.GetText())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	p, err := h.postService.Repost(ctx, authorID, postID, text)
	if err != nil {
		return nil, err
	}

	return &postv1.RepostPostResponse{Post: toProtoPost(p, false, viewerID(ctx))}, nil
}

func (h *PostHandler) EditPost(ctx context.Context, req *postv1.EditPostRequest) (*postv1.EditPostResponse, error) {
	requesterID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	postID, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid post id")
	}

	text, photos, err := fromProtoTextPhotos(req.GetText(), req.GetPhotoIds())
	if err != nil {
		return nil, err
	}

	p, liked, err := h.postService.Edit(ctx, requesterID, postID, text, photos)
	if err != nil {
		return nil, err
	}

	return &postv1.EditPostResponse{Post: toProtoPost(p, liked, viewerID(ctx))}, nil
}

func (h *PostHandler) PinPost(ctx context.Context, req *postv1.PinPostRequest) (*emptypb.Empty, error) {
	return h.setPinned(ctx, req.GetId(), h.postService.Pin)
}

func (h *PostHandler) UnpinPost(ctx context.Context, req *postv1.UnpinPostRequest) (*emptypb.Empty, error) {
	return h.setPinned(ctx, req.GetId(), h.postService.Unpin)
}

func (h *PostHandler) setPinned(ctx context.Context, id string, set func(ctx context.Context, requesterID, postID uuid.UUID) error) (*emptypb.Empty, error) {
	requesterID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	postID, err := uuid.Parse(id)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid post id")
	}

	if err = set(ctx, requesterID, postID); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *PostHandler) DeletePost(ctx context.Context, req *postv1.DeletePostRequest) (*emptypb.Empty, error) {
	requesterID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	postID, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid post id")
	}

	if err = h.postService.Delete(ctx, requesterID, postID); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (h *PostHandler) GetPost(ctx context.Context, req *postv1.GetPostRequest) (*postv1.GetPostResponse, error) {
	postID, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid post id")
	}

	p, liked, err := h.postService.Get(ctx, viewerID(ctx), postID)
	if err != nil {
		return nil, err
	}

	return &postv1.GetPostResponse{Post: toProtoPost(p, liked, viewerID(ctx))}, nil
}

// GetPosts leaves liked unset: the posts are shown without their likes.
func (h *PostHandler) GetPosts(ctx context.Context, req *postv1.GetPostsRequest) (*postv1.GetPostsResponse, error) {
	ids, err := parseIDs(req.GetIds())
	if err != nil {
		return nil, err
	}

	posts, err := h.postService.GetMany(ctx, ids)
	if err != nil {
		return nil, err
	}

	resp := &postv1.GetPostsResponse{Posts: make([]*postv1.Post, 0, len(posts))}
	for _, p := range posts {
		resp.Posts = append(resp.Posts, toProtoPost(p, false, viewerID(ctx)))
	}

	return resp, nil
}

func (h *PostHandler) ListPosts(ctx context.Context, req *postv1.ListPostsRequest) (*postv1.ListPostsResponse, error) {
	wallID, err := uuid.Parse(req.GetWallId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid wall id")
	}

	cursor, err := parseCursor(req.GetCursor())
	if err != nil {
		return nil, err
	}

	page, err := h.postService.ListWall(ctx, viewerID(ctx), wallID, cursor, int(req.GetLimit()))
	if err != nil {
		return nil, err
	}

	return &postv1.ListPostsResponse{
		Posts:      toProtoPosts(page, viewerID(ctx)),
		NextCursor: cursorString(page.NextCursor),
		Total:      int32(page.Total), //nolint:gosec // a wall will not hold 2^31 posts
	}, nil
}

func (h *PostHandler) ListFeed(ctx context.Context, req *postv1.ListFeedRequest) (*postv1.ListFeedResponse, error) {
	if len(req.GetAuthorIds()) > service.MaxFeedAuthors || len(req.GetWallIds()) > service.MaxFeedAuthors {
		return nil, status.Error(codes.InvalidArgument, "too many authors")
	}

	authorIDs, err := parseFeedIDs(req.GetAuthorIds(), "invalid author id")
	if err != nil {
		return nil, err
	}

	wallIDs, err := parseFeedIDs(req.GetWallIds(), "invalid wall id")
	if err != nil {
		return nil, err
	}

	cursor, err := parseCursor(req.GetCursor())
	if err != nil {
		return nil, err
	}

	sources := service.FeedSources{Authors: authorIDs, Walls: wallIDs}

	page, err := h.postService.Feed(ctx, viewerID(ctx), sources, cursor, int(req.GetLimit()))
	if err != nil {
		return nil, err
	}

	return &postv1.ListFeedResponse{
		Posts:      toProtoPosts(page, viewerID(ctx)),
		NextCursor: cursorString(page.NextCursor),
	}, nil
}

func (h *PostHandler) LikePost(ctx context.Context, req *postv1.LikePostRequest) (*postv1.LikePostResponse, error) {
	userID, postID, err := likeTarget(ctx, req.GetPostId())
	if err != nil {
		return nil, err
	}

	likes, err := h.postService.Like(ctx, userID, postID)
	if err != nil {
		return nil, err
	}

	return &postv1.LikePostResponse{LikeCount: int32(likes)}, nil //nolint:gosec // likes come from an int4 column
}

func (h *PostHandler) UnlikePost(ctx context.Context, req *postv1.UnlikePostRequest) (*postv1.UnlikePostResponse, error) {
	userID, postID, err := likeTarget(ctx, req.GetPostId())
	if err != nil {
		return nil, err
	}

	likes, err := h.postService.Unlike(ctx, userID, postID)
	if err != nil {
		return nil, err
	}

	return &postv1.UnlikePostResponse{LikeCount: int32(likes)}, nil //nolint:gosec // likes come from an int4 column
}

// likeTarget is who likes (or shares) which post.
func likeTarget(ctx context.Context, rawPostID string) (uuid.UUID, uuid.UUID, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	postID, err := uuid.Parse(rawPostID)
	if err != nil {
		return uuid.Nil, uuid.Nil, status.Error(codes.InvalidArgument, "invalid post id")
	}

	return userID, postID, nil
}

// parseIDs reads at most service.MaxPageSize IDs.
func parseIDs(raw []string) ([]uuid.UUID, error) {
	if len(raw) > service.MaxPageSize {
		return nil, status.Error(codes.InvalidArgument, "too many ids")
	}

	ids := make([]uuid.UUID, 0, len(raw))
	for _, s := range raw {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid id")
		}
		ids = append(ids, id)
	}

	return ids, nil
}

// parseCursor reads a page cursor; "" is the first page.
func parseCursor(s string) (uuid.UUID, error) {
	if s == "" {
		return uuid.Nil, nil
	}

	cursor, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, status.Error(codes.InvalidArgument, "invalid cursor")
	}

	return cursor, nil
}

// viewerID is who reads a public method: uuid.Nil without an access token.
func viewerID(ctx context.Context) uuid.UUID {
	identity, ok := jwtauth.IdentityFromContext(ctx)
	if !ok {
		return uuid.Nil
	}

	return identity.UserID
}

// currentUserID is the caller's ID from the access token verified by
// jwtauth.UnaryServerInterceptor; never trust an ID from the request body.
func currentUserID(ctx context.Context) (uuid.UUID, error) {
	identity, ok := jwtauth.IdentityFromContext(ctx)
	if !ok {
		return uuid.Nil, status.Error(codes.Unauthenticated, "missing access token")
	}

	return identity.UserID, nil
}

func parseFeedIDs(ss []string, invalid string) ([]uuid.UUID, error) {
	ids := make([]uuid.UUID, 0, len(ss))
	for _, s := range ss {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, invalid)
		}
		ids = append(ids, id)
	}

	return ids, nil
}
