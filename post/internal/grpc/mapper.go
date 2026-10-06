package grpc

import (
	"github.com/google/uuid"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func fromProtoContent(req *postv1.CreatePostRequest) (post.Content, error) {
	text, err := post.NewText(req.GetText())
	if err != nil {
		return post.Content{}, status.Error(codes.InvalidArgument, err.Error())
	}

	ids := make([]uuid.UUID, 0, len(req.GetPhotoIds()))
	for _, s := range req.GetPhotoIds() {
		id, err := uuid.Parse(s)
		if err != nil {
			return post.Content{}, status.Error(codes.InvalidArgument, post.ErrInvalidPhoto.Error())
		}
		ids = append(ids, id)
	}

	photos, err := post.NewPhotos(ids)
	if err != nil {
		return post.Content{}, status.Error(codes.InvalidArgument, err.Error())
	}

	content, err := post.NewContent(text, photos)
	if err != nil {
		return post.Content{}, status.Error(codes.InvalidArgument, err.Error())
	}

	return content, nil
}

func toProtoPost(p *post.Post, liked bool) *postv1.Post {
	counters := p.Counters()
	photos := make([]string, len(p.Content().Photos()))
	for i, id := range p.Content().Photos() {
		photos[i] = id.String()
	}

	return &postv1.Post{
		Id:           p.ID().String(),
		WallId:       p.WallID().String(),
		AuthorId:     p.AuthorID().String(),
		Text:         p.Content().Text().String(),
		PhotoIds:     photos,
		CreatedAt:    timestamppb.New(p.CreatedAt()),
		LikeCount:    int32(counters.Likes),    //nolint:gosec // counters come from int4 columns
		CommentCount: int32(counters.Comments), //nolint:gosec // counters come from int4 columns
		Liked:        liked,
	}
}

func toProtoPosts(page service.Page) []*postv1.Post {
	posts := make([]*postv1.Post, 0, len(page.Posts))
	for _, p := range page.Posts {
		posts = append(posts, toProtoPost(p, page.Liked[p.ID()]))
	}

	return posts
}

func toProtoComment(c *post.Comment) *postv1.Comment {
	return &postv1.Comment{
		Id:        c.ID().String(),
		PostId:    c.PostID().String(),
		AuthorId:  c.AuthorID().String(),
		Text:      c.Text().String(),
		CreatedAt: timestamppb.New(c.CreatedAt()),
	}
}

// cursorString is "" on the last page.
func cursorString(cursor uuid.UUID) string {
	if cursor == uuid.Nil {
		return ""
	}

	return cursor.String()
}
