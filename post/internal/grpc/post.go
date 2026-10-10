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
	Count(ctx context.Context) (int, error)
	Create(ctx context.Context, in service.CreateInput) (*post.Post, error)
	Repost(ctx context.Context, in service.RepostInput) (*post.Post, error)
	Delete(ctx context.Context, requesterID, postID uuid.UUID) error
	Edit(ctx context.Context, requesterID, postID uuid.UUID, edit service.PostEdit) (*post.Post, bool, error)
	Pin(ctx context.Context, requesterID, postID uuid.UUID) error
	Unpin(ctx context.Context, requesterID, postID uuid.UUID) error
	Get(ctx context.Context, viewerID, postID uuid.UUID) (*post.Post, bool, error)
	GetMany(ctx context.Context, ids []uuid.UUID) ([]*post.Post, error)
	Archive(ctx context.Context, requesterID, postID uuid.UUID) error
	Unarchive(ctx context.Context, requesterID, postID uuid.UUID) error
	CloseComments(ctx context.Context, requesterID, postID uuid.UUID) error
	OpenComments(ctx context.Context, requesterID, postID uuid.UUID) error
	ListWall(ctx context.Context, viewerID, wallID uuid.UUID, pick service.WallPick, cursor uuid.UUID, size int) (service.Page, error)
	Feed(ctx context.Context, viewerID uuid.UUID, sources service.FeedSources, cursor uuid.UUID, size int) (service.Page, error)
	Like(ctx context.Context, userID, postID uuid.UUID) (int, error)
	Unlike(ctx context.Context, userID, postID uuid.UUID) (int, error)
	Likers(ctx context.Context, postID uuid.UUID, cursor *service.LikerCursor, size int) (service.LikerPage, error)
}

type CommentService interface {
	Create(ctx context.Context, in service.CommentInput) (*post.Comment, error)
	Like(ctx context.Context, userID, commentID uuid.UUID) (int, error)
	Unlike(ctx context.Context, userID, commentID uuid.UUID) (int, error)
	Likers(ctx context.Context, commentID uuid.UUID, cursor *service.LikerCursor, size int) (service.LikerPage, error)
	Delete(ctx context.Context, requesterID, commentID uuid.UUID) error
	List(ctx context.Context, viewerID, postID, cursor uuid.UUID, size int) (service.CommentPage, error)
	GetMany(ctx context.Context, ids []uuid.UUID) ([]*post.Comment, error)
	Last(ctx context.Context, viewerID uuid.UUID, postIDs []uuid.UUID) (service.CommentPage, error)
}

type SuggestionService interface {
	Suggest(ctx context.Context, wallID, authorID uuid.UUID, content post.Content) (*post.Suggestion, error)
	List(ctx context.Context, viewerID, wallID uuid.UUID, mine bool, cursor uuid.UUID, size int) (service.SuggestionPage, error)
	Publish(ctx context.Context, requesterID, id uuid.UUID, text post.Text, signed bool) (*post.Post, error)
	Withdraw(ctx context.Context, requesterID, id uuid.UUID) error
}

type TopicService interface {
	Start(ctx context.Context, in service.StartTopicInput) (*post.Topic, error)
	Get(ctx context.Context, id uuid.UUID) (*post.Topic, error)
	List(ctx context.Context, clubID uuid.UUID, query string, after *service.TopicCursor, size int) (service.TopicPage, error)
	Change(ctx context.Context, requesterID, id uuid.UUID, changes post.TopicChanges) (*post.Topic, error)
	Delete(ctx context.Context, requesterID, id uuid.UUID) error
	Write(ctx context.Context, in service.TopicMessageInput) (*post.TopicMessage, error)
	Messages(ctx context.Context, viewerID, topicID, after uuid.UUID, size int) (service.TopicMessagePage, error)
	DeleteMessage(ctx context.Context, requesterID, id uuid.UUID) error
	LikeMessage(ctx context.Context, userID, id uuid.UUID) (int, error)
	UnlikeMessage(ctx context.Context, userID, id uuid.UUID) (int, error)
}

type AlbumService interface {
	Create(ctx context.Context, in service.CreateAlbumInput) (*post.Album, error)
	Get(ctx context.Context, id uuid.UUID) (*post.Album, *post.AlbumPhoto, error)
	List(ctx context.Context, ownerID, before uuid.UUID, size int) (service.AlbumPage, error)
	Change(ctx context.Context, requesterID, id uuid.UUID, name post.AlbumName, description post.Description) (*post.Album, error)
	Delete(ctx context.Context, requesterID, id uuid.UUID) error
	AddPhotos(ctx context.Context, requesterID, albumID uuid.UUID, in []service.NewAlbumPhoto) ([]*post.AlbumPhoto, error)
	AddAvatar(ctx context.Context, requesterID, ownerID, photoID uuid.UUID) (*post.AlbumPhoto, error)
	Photos(ctx context.Context, albumID, after uuid.UUID, size int) (service.AlbumPhotoPage, error)
	Photo(ctx context.Context, id uuid.UUID) (*post.AlbumPhoto, *post.Album, error)
	Describe(ctx context.Context, requesterID, id uuid.UUID, description post.Description) (*post.AlbumPhoto, error)
	DeletePhoto(ctx context.Context, requesterID, id uuid.UUID) error
	SetCover(ctx context.Context, requesterID, albumID, photoID uuid.UUID) error
	MovePhoto(ctx context.Context, requesterID, photoID, albumID uuid.UUID) error
	LikePhoto(ctx context.Context, userID, photoID uuid.UUID) (int, error)
	UnlikePhoto(ctx context.Context, userID, photoID uuid.UUID) (int, error)
	LikedPhotos(ctx context.Context, userID uuid.UUID, photoIDs []uuid.UUID) (map[uuid.UUID]bool, error)
	PhotoLikers(ctx context.Context, photoID uuid.UUID, cursor *service.LikerCursor, size int) (service.LikerPage, error)
	CommentPhoto(ctx context.Context, authorID, photoID uuid.UUID, text post.CommentText) (*post.PhotoComment, error)
	DeletePhotoComment(ctx context.Context, requesterID, id uuid.UUID) error
	PhotoComments(ctx context.Context, photoID, after uuid.UUID, size int) (service.PhotoCommentPage, error)
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
	Topics      TopicService
	Albums      AlbumService
	Purge       PurgeService
}

type PostHandler struct {
	postv1.UnimplementedPostServiceServer
	postService       PostService
	commentService    CommentService
	suggestionService SuggestionService
	topicService      TopicService
	albumService      AlbumService
	purgeService      PurgeService
	logger            *slog.Logger
}

func NewPostHandler(s Services, logger *slog.Logger) *PostHandler {
	return &PostHandler{
		postService:       s.Posts,
		commentService:    s.Comments,
		suggestionService: s.Suggestions,
		topicService:      s.Topics,
		albumService:      s.Albums,
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

	mentions, err := parseMentions(req.GetMentionedIds())
	if err != nil {
		return nil, err
	}

	p, err := h.postService.Create(ctx, service.CreateInput{
		WallID:   wallID,
		AuthorID: authorID,
		Content:  content,
		ClubWall: req.GetClubWall(),
		FromWall: req.GetFromWall(),
		Signed:   req.GetSigned(),
		NSFW:     req.GetNsfw(),
		Mentions: mentions,

		AvatarUpdate: req.GetAvatarUpdate(),
	})
	if err != nil {
		return nil, err
	}

	return &postv1.CreatePostResponse{Post: toProtoPost(p, false, viewerID(ctx))}, nil
}

// RepostPost shares a post or, with photo_id, a photo of an album.
func (h *PostHandler) RepostPost(ctx context.Context, req *postv1.RepostPostRequest) (*postv1.RepostPostResponse, error) {
	authorID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	var postID uuid.UUID
	if req.GetPhotoId() == "" {
		if _, postID, err = likeTarget(ctx, req.GetPostId()); err != nil {
			return nil, err
		}
	}

	text, err := post.NewText(req.GetText())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	in := service.RepostInput{AuthorID: authorID, PostID: postID, Text: text, FromWall: req.GetFromWall(), Signed: req.GetSigned()}
	if req.GetPhotoId() != "" {
		if in.PhotoID, err = uuid.Parse(req.GetPhotoId()); err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid photo id")
		}
	}
	if req.GetWallId() != "" {
		if in.WallID, err = uuid.Parse(req.GetWallId()); err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid wall id")
		}
	}

	p, err := h.postService.Repost(ctx, in)
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

	p, liked, err := h.postService.Edit(ctx, requesterID, postID, service.PostEdit{Text: text, Photos: photos, NSFW: req.Nsfw})
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

func (h *PostHandler) ArchivePost(ctx context.Context, req *postv1.ArchivePostRequest) (*emptypb.Empty, error) {
	return h.setPinned(ctx, req.GetId(), h.postService.Archive)
}

func (h *PostHandler) UnarchivePost(ctx context.Context, req *postv1.UnarchivePostRequest) (*emptypb.Empty, error) {
	return h.setPinned(ctx, req.GetId(), h.postService.Unarchive)
}

func (h *PostHandler) CloseComments(ctx context.Context, req *postv1.CloseCommentsRequest) (*emptypb.Empty, error) {
	return h.setPinned(ctx, req.GetPostId(), h.postService.CloseComments)
}

func (h *PostHandler) OpenComments(ctx context.Context, req *postv1.OpenCommentsRequest) (*emptypb.Empty, error) {
	return h.setPinned(ctx, req.GetPostId(), h.postService.OpenComments)
}

// setPinned makes a switch of the post id by the caller: its pin, its
// archive, its comments.
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
// CountPosts counts the posts of the site, for its page "о сайте".
func (h *PostHandler) CountPosts(ctx context.Context, _ *postv1.CountPostsRequest) (*postv1.CountPostsResponse, error) {
	n, err := h.postService.Count(ctx)
	if err != nil {
		return nil, err
	}

	return &postv1.CountPostsResponse{Count: int32(n)}, nil //nolint:gosec // a count of rows
}

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
		resp.Posts = append(resp.Posts, excerpted(toProtoPost(p, false, viewerID(ctx)), p, req.GetTextLimit()))
	}

	return resp, nil
}

var wallFilters = map[postv1.WallFilter]post.WallFilter{
	postv1.WallFilter_WALL_FILTER_NONE:    post.WallAll,
	postv1.WallFilter_WALL_FILTER_OWNER:   post.WallOwner,
	postv1.WallFilter_WALL_FILTER_OTHERS:  post.WallOthers,
	postv1.WallFilter_WALL_FILTER_ARCHIVE: post.WallArchive,
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

	filter, ok := wallFilters[req.GetFilter()]
	if !ok {
		return nil, status.Error(codes.InvalidArgument, post.ErrInvalidWallFilter.Error())
	}

	query, err := post.NewWallQuery(req.GetQuery())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	page, err := h.postService.ListWall(ctx, viewerID(ctx), wallID, service.WallPick{Filter: filter, Query: query}, cursor, int(req.GetLimit()))
	if err != nil {
		return nil, err
	}

	return &postv1.ListPostsResponse{
		Posts:      toProtoPosts(page, viewerID(ctx), req.GetTextLimit()),
		NextCursor: cursorString(page.NextCursor),
		Total:      int32(page.Total), //nolint:gosec // a wall will not hold 2^31 posts
	}, nil
}

func (h *PostHandler) ListFeed(ctx context.Context, req *postv1.ListFeedRequest) (*postv1.ListFeedResponse, error) {
	if len(req.GetAuthorIds()) > service.MaxFeedAuthors || len(req.GetWallIds()) > service.MaxFeedWalls {
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

	sources := service.FeedSources{Authors: authorIDs, Walls: wallIDs, HideNSFW: req.GetHideNsfw(), Everyone: req.GetEveryone()}

	page, err := h.postService.Feed(ctx, viewerID(ctx), sources, cursor, int(req.GetLimit()))
	if err != nil {
		return nil, err
	}

	return &postv1.ListFeedResponse{
		Posts:      toProtoPosts(page, viewerID(ctx), req.GetTextLimit()),
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

// parseMentions reads the people a text names, whom the caller found.
func parseMentions(ss []string) (post.Mentions, error) {
	ids, err := parseFeedIDs(ss, "invalid mentioned id")
	if err != nil {
		return nil, err
	}

	mentions, err := post.NewMentions(ids)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	return mentions, nil
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
