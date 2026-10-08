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
	text, photos, err := fromProtoTextPhotos(req.GetText(), req.GetPhotoIds())
	if err != nil {
		return post.Content{}, err
	}

	content, err := post.NewContent(text, photos)
	if err != nil {
		return post.Content{}, status.Error(codes.InvalidArgument, err.Error())
	}

	return content, nil
}

// fromProtoTextPhotos reads what a post says, for a new post or an edit.
func fromProtoTextPhotos(rawText string, rawPhotos []string) (post.Text, post.Photos, error) {
	text, err := post.NewText(rawText)
	if err != nil {
		return "", nil, status.Error(codes.InvalidArgument, err.Error())
	}

	photos, err := fromProtoPhotos(rawPhotos)
	if err != nil {
		return "", nil, err
	}

	return text, photos, nil
}

// fromProtoCommentContent reads what a new comment says.
func fromProtoCommentContent(req *postv1.CreateCommentRequest) (post.CommentContent, error) {
	text, err := post.NewCommentText(req.GetText())
	if err != nil {
		return post.CommentContent{}, status.Error(codes.InvalidArgument, err.Error())
	}

	photos, err := fromProtoPhotos(req.GetPhotoIds())
	if err != nil {
		return post.CommentContent{}, err
	}

	content, err := post.NewCommentContent(text, photos)
	if err != nil {
		return post.CommentContent{}, status.Error(codes.InvalidArgument, err.Error())
	}

	return content, nil
}

func fromProtoPhotos(raw []string) (post.Photos, error) {
	ids := make([]uuid.UUID, 0, len(raw))
	for _, s := range raw {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, post.ErrInvalidPhoto.Error())
		}
		ids = append(ids, id)
	}

	photos, err := post.NewPhotos(ids)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if len(photos) == 0 {
		return nil, nil // no photos, as storage reads them back
	}

	return photos, nil
}

func photoStrings(photos post.Photos) []string {
	out := make([]string, len(photos))
	for i, id := range photos {
		out[i] = id.String()
	}

	return out
}

// toProtoPost: viewerID is the caller, uuid.Nil if anonymous.
func toProtoPost(p *post.Post, liked bool, viewerID uuid.UUID) *postv1.Post {
	counters := p.Counters()

	pp := &postv1.Post{
		Id:            p.ID().String(),
		WallId:        p.WallID().String(),
		AuthorId:      p.AuthorID().String(),
		Text:          p.Content().Text().String(),
		PhotoIds:      photoStrings(p.Content().Photos()),
		CreatedAt:     timestamppb.New(p.CreatedAt()),
		LikeCount:     int32(counters.Likes),    //nolint:gosec // counters come from int4 columns
		CommentCount:  int32(counters.Comments), //nolint:gosec // counters come from int4 columns
		RepostCount:   int32(counters.Reposts),  //nolint:gosec // counters come from int4 columns
		RepostOf:      idString(p.Content().RepostOf()),
		Liked:         liked,
		Pinned:        p.Pinned(),
		EditableUntil: timestamppb.New(p.EditableUntil()),
		FromWall:      p.FromWall(),
		Mine:          viewerID != uuid.Nil && viewerID == p.AuthorID(),
	}

	if !p.EditedAt().IsZero() {
		pp.EditedAt = timestamppb.New(p.EditedAt())
	}

	return pp
}

func toProtoPosts(page service.Page, viewerID uuid.UUID) []*postv1.Post {
	posts := make([]*postv1.Post, 0, len(page.Posts))
	for _, p := range page.Posts {
		posts = append(posts, toProtoPost(p, page.Liked[p.ID()], viewerID))
	}

	return posts
}

func toProtoComment(c *post.Comment, liked bool) *postv1.Comment {
	return &postv1.Comment{
		Id:        c.ID().String(),
		PostId:    c.PostID().String(),
		AuthorId:  c.AuthorID().String(),
		Text:      c.Text().String(),
		CreatedAt: timestamppb.New(c.CreatedAt()),
		PhotoIds:  photoStrings(c.Photos()),
		ReplyToId: idString(c.ReplyTo()),
		LikeCount: int32(c.Likes()), //nolint:gosec // counters come from int4 columns
		Liked:     liked,
	}
}

// cursorString is "" on the last page.
func cursorString(cursor uuid.UUID) string {
	return idString(cursor)
}

// idString is "" for uuid.Nil, as proto fields say "none".
func idString(id uuid.UUID) string {
	if id == uuid.Nil {
		return ""
	}

	return id.String()
}
