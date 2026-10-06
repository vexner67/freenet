package grpc_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestLikes(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, bob := uuid.New(), uuid.New()

	created, err := s.client.CreatePost(s.as(t, ann), &postv1.CreatePostRequest{Text: "привет"})
	if err != nil {
		t.Fatal(err)
	}
	id := created.GetPost().GetId()

	liked, err := s.client.LikePost(s.as(t, bob), &postv1.LikePostRequest{PostId: id})
	if err != nil || liked.GetLikeCount() != 1 {
		t.Fatalf("LikePost = %v, %v", liked, err)
	}

	// Public methods tell the caller with a token whether they like the post.
	tests := []struct {
		name      string
		ctx       context.Context
		wantLiked bool
	}{
		{"bob", s.as(t, bob), true},
		{"ann", s.as(t, ann), false},
		{"anonymous", context.Background(), false},
	}

	for _, tt := range tests {
		got, err := s.client.GetPost(tt.ctx, &postv1.GetPostRequest{Id: id})
		if err != nil {
			t.Fatalf("%s: GetPost: %v", tt.name, err)
		}
		if got.GetPost().GetLikeCount() != 1 || got.GetPost().GetLiked() != tt.wantLiked {
			t.Fatalf("%s: GetPost = %v", tt.name, got.GetPost())
		}

		wall, err := s.client.ListPosts(tt.ctx, &postv1.ListPostsRequest{WallId: ann.String()})
		if err != nil || wall.GetPosts()[0].GetLiked() != tt.wantLiked {
			t.Fatalf("%s: ListPosts = %v, %v", tt.name, wall, err)
		}

		feed, err := s.client.ListFeed(tt.ctx, &postv1.ListFeedRequest{AuthorIds: []string{ann.String()}})
		if err != nil || feed.GetPosts()[0].GetLiked() != tt.wantLiked {
			t.Fatalf("%s: ListFeed = %v, %v", tt.name, feed, err)
		}
	}

	unliked, err := s.client.UnlikePost(s.as(t, bob), &postv1.UnlikePostRequest{PostId: id})
	if err != nil || unliked.GetLikeCount() != 0 {
		t.Fatalf("UnlikePost = %v, %v", unliked, err)
	}
}

func TestComments(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()

	created, err := s.client.CreatePost(s.as(t, ann), &postv1.CreatePostRequest{Text: "привет"})
	if err != nil {
		t.Fatal(err)
	}
	postID := created.GetPost().GetId()

	var ids []string
	for _, text := range []string{"раз", "два", "три"} {
		c, err := s.client.CreateComment(s.as(t, bob), &postv1.CreateCommentRequest{PostId: postID, Text: text})
		if err != nil {
			t.Fatalf("CreateComment: %v", err)
		}
		if c.GetComment().GetAuthorId() != bob.String() || c.GetComment().GetPostId() != postID || c.GetComment().GetCreatedAt() == nil {
			t.Fatalf("comment fields: %v", c.GetComment())
		}
		ids = append(ids, c.GetComment().GetId())
	}

	page, err := s.client.ListComments(context.Background(), &postv1.ListCommentsRequest{PostId: postID, Limit: 2})
	if err != nil {
		t.Fatalf("ListComments without a token: %v", err)
	}
	if page.GetTotal() != 3 || len(page.GetComments()) != 2 || page.GetComments()[0].GetText() != "раз" || page.GetNextCursor() == "" {
		t.Fatalf("first page: %v", page)
	}

	last, err := s.client.ListComments(context.Background(), &postv1.ListCommentsRequest{PostId: postID, Limit: 2, Cursor: page.GetNextCursor()})
	if err != nil || len(last.GetComments()) != 1 || last.GetComments()[0].GetText() != "три" || last.GetNextCursor() != "" {
		t.Fatalf("last page: %v, %v", last, err)
	}

	got, err := s.client.GetPost(context.Background(), &postv1.GetPostRequest{Id: postID})
	if err != nil || got.GetPost().GetCommentCount() != 3 {
		t.Fatalf("comment count: %v, %v", got, err)
	}

	// Batches by ID without a token, for the notifications page.
	posts, err := s.client.GetPosts(context.Background(), &postv1.GetPostsRequest{Ids: []string{postID, uuid.NewString()}})
	if err != nil || len(posts.GetPosts()) != 1 || posts.GetPosts()[0].GetText() != "привет" {
		t.Fatalf("GetPosts = %v, %v", posts, err)
	}
	comments, err := s.client.GetComments(context.Background(), &postv1.GetCommentsRequest{Ids: ids})
	if err != nil || len(comments.GetComments()) != 3 {
		t.Fatalf("GetComments = %v, %v", comments, err)
	}

	_, err = s.client.DeleteComment(s.as(t, cat), &postv1.DeleteCommentRequest{Id: ids[0]})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("stranger deletes: %v", err)
	}

	if _, err = s.client.DeleteComment(s.as(t, bob), &postv1.DeleteCommentRequest{Id: ids[0]}); err != nil {
		t.Fatalf("author of the comment deletes: %v", err)
	}
	if _, err = s.client.DeleteComment(s.as(t, ann), &postv1.DeleteCommentRequest{Id: ids[1]}); err != nil {
		t.Fatalf("author of the post deletes: %v", err)
	}

	left, err := s.client.ListComments(context.Background(), &postv1.ListCommentsRequest{PostId: postID})
	if err != nil || left.GetTotal() != 1 || len(left.GetComments()) != 1 || left.GetComments()[0].GetId() != ids[2] {
		t.Fatalf("after deleting: %v, %v", left, err)
	}
}
