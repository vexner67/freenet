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
	"google.golang.org/protobuf/types/known/timestamppb"
)

type PostService interface {
	Create(ctx context.Context, authorID uuid.UUID, text post.Text) (*post.Post, error)
	Delete(ctx context.Context, requesterID, postID uuid.UUID) error
	ListByAuthor(ctx context.Context, authorID, cursor uuid.UUID, size int) (service.Page, error)
	Feed(ctx context.Context, authorIDs []uuid.UUID, cursor uuid.UUID, size int) (service.Page, error)
}

type PostHandler struct {
	postv1.UnimplementedPostServiceServer
	postService PostService
	logger      *slog.Logger
}

func NewPostHandler(postService PostService, logger *slog.Logger) *PostHandler {
	return &PostHandler{
		postService: postService,
		logger:      logger,
	}
}

func (h *PostHandler) CreatePost(ctx context.Context, req *postv1.CreatePostRequest) (*postv1.CreatePostResponse, error) {
	authorID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	text, err := post.NewText(req.GetText())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	p, err := h.postService.Create(ctx, authorID, text)
	if err != nil {
		return nil, err
	}

	return &postv1.CreatePostResponse{Post: toProtoPost(p)}, nil
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

func (h *PostHandler) ListPosts(ctx context.Context, req *postv1.ListPostsRequest) (*postv1.ListPostsResponse, error) {
	authorID, err := uuid.Parse(req.GetAuthorId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid author id")
	}

	cursor, err := parseCursor(req.GetCursor())
	if err != nil {
		return nil, err
	}

	page, err := h.postService.ListByAuthor(ctx, authorID, cursor, int(req.GetLimit()))
	if err != nil {
		return nil, err
	}

	resp := &postv1.ListPostsResponse{
		Posts: make([]*postv1.Post, 0, len(page.Posts)),
		Total: int32(page.Total), //nolint:gosec // a wall will not hold 2^31 posts
	}

	for _, p := range page.Posts {
		resp.Posts = append(resp.Posts, toProtoPost(p))
	}

	if page.NextCursor != uuid.Nil {
		resp.NextCursor = page.NextCursor.String()
	}

	return resp, nil
}

func (h *PostHandler) ListFeed(ctx context.Context, req *postv1.ListFeedRequest) (*postv1.ListFeedResponse, error) {
	if len(req.GetAuthorIds()) > service.MaxFeedAuthors {
		return nil, status.Error(codes.InvalidArgument, "too many authors")
	}

	authorIDs := make([]uuid.UUID, 0, len(req.GetAuthorIds()))
	for _, s := range req.GetAuthorIds() {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid author id")
		}
		authorIDs = append(authorIDs, id)
	}

	cursor, err := parseCursor(req.GetCursor())
	if err != nil {
		return nil, err
	}

	page, err := h.postService.Feed(ctx, authorIDs, cursor, int(req.GetLimit()))
	if err != nil {
		return nil, err
	}

	resp := &postv1.ListFeedResponse{Posts: make([]*postv1.Post, 0, len(page.Posts))}
	for _, p := range page.Posts {
		resp.Posts = append(resp.Posts, toProtoPost(p))
	}

	if page.NextCursor != uuid.Nil {
		resp.NextCursor = page.NextCursor.String()
	}

	return resp, nil
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

func toProtoPost(p *post.Post) *postv1.Post {
	return &postv1.Post{
		Id:        p.ID().String(),
		AuthorId:  p.AuthorID().String(),
		Text:      p.Text().String(),
		CreatedAt: timestamppb.New(p.CreatedAt()),
	}
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
