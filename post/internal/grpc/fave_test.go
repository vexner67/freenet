package grpc_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func postTexts(posts []*postv1.Post) []string {
	texts := make([]string, 0, len(posts))
	for _, p := range posts {
		texts = append(texts, p.GetText())
	}

	return texts
}

func TestLikedPosts(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, bob := uuid.New(), uuid.New()

	write := func(text string) string {
		t.Helper()

		resp, err := s.client.CreatePost(s.as(t, ann), &postv1.CreatePostRequest{Text: text})
		if err != nil {
			t.Fatal(err)
		}

		return resp.GetPost().GetId()
	}

	first, second, third := write("первая"), write("вторая"), write("третья")

	// Liked in another order than written: the newest like comes first.
	for _, id := range []string{second, third, first} {
		if _, err := s.client.LikePost(s.as(t, bob), &postv1.LikePostRequest{PostId: id}); err != nil {
			t.Fatal(err)
		}
	}

	page, err := s.client.ListLikedPosts(s.as(t, bob), &postv1.ListLikedPostsRequest{Limit: 2, TextLimit: 3})
	if err != nil || page.GetTotal() != 3 || page.GetNextCursor() == "" {
		t.Fatalf("ListLikedPosts = %v, %v", page, err)
	}
	if got := postTexts(page.GetPosts()); !slices.Equal(got, []string{"пер", "тре"}) {
		t.Fatalf("the first page = %v", got)
	}
	if p := page.GetPosts()[0]; !p.GetLiked() || !p.GetTextCut() || p.GetMine() {
		t.Fatalf("a liked post = %v", p)
	}

	page, err = s.client.ListLikedPosts(s.as(t, bob), &postv1.ListLikedPostsRequest{Limit: 2, Cursor: page.GetNextCursor()})
	if err != nil || page.GetNextCursor() != "" || !slices.Equal(postTexts(page.GetPosts()), []string{"вторая"}) {
		t.Fatalf("the second page = %v, %v", page, err)
	}

	// A like taken back, a deleted post and an archived one leave.
	if _, err = s.client.UnlikePost(s.as(t, bob), &postv1.UnlikePostRequest{PostId: first}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.client.DeletePost(s.as(t, ann), &postv1.DeletePostRequest{Id: third}); err != nil {
		t.Fatal(err)
	}

	page, err = s.client.ListLikedPosts(s.as(t, bob), &postv1.ListLikedPostsRequest{})
	if err != nil || page.GetTotal() != 1 || !slices.Equal(postTexts(page.GetPosts()), []string{"вторая"}) {
		t.Fatalf("after the unlike and the delete = %v, %v", page, err)
	}

	if _, err = s.client.ArchivePost(s.as(t, ann), &postv1.ArchivePostRequest{Id: second}); err != nil {
		t.Fatal(err)
	}
	if page, err = s.client.ListLikedPosts(s.as(t, bob), &postv1.ListLikedPostsRequest{}); err != nil || page.GetTotal() != 0 || len(page.GetPosts()) != 0 {
		t.Fatalf("after the archive = %v, %v", page, err)
	}

	// Somebody else's faves are nobody's to read: only one's own.
	if page, err = s.client.ListLikedPosts(s.as(t, ann), &postv1.ListLikedPostsRequest{}); err != nil || page.GetTotal() != 0 {
		t.Fatalf("faves of who likes nothing = %v, %v", page, err)
	}
	if _, err = s.client.ListLikedPosts(context.Background(), &postv1.ListLikedPostsRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("anonymous: %v", err)
	}
	if _, err = s.client.ListLikedPosts(s.as(t, bob), &postv1.ListLikedPostsRequest{Cursor: "x"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("a bad cursor: %v", err)
	}
}

func TestLikedCommentsAndPhotos(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, bob := uuid.New(), uuid.New()

	created, err := s.client.CreatePost(s.as(t, ann), &postv1.CreatePostRequest{Text: "запись"})
	if err != nil {
		t.Fatal(err)
	}
	postID := created.GetPost().GetId()

	comment := func(text string) string {
		t.Helper()

		resp, err := s.client.CreateComment(s.as(t, ann), &postv1.CreateCommentRequest{PostId: postID, Text: text})
		if err != nil {
			t.Fatal(err)
		}

		return resp.GetComment().GetId()
	}

	one, two := comment("первый"), comment(strings.Repeat("я", 10))
	for _, id := range []string{two, one} {
		if _, err = s.client.LikeComment(s.as(t, bob), &postv1.LikeCommentRequest{CommentId: id}); err != nil {
			t.Fatal(err)
		}
	}

	comments, err := s.client.ListLikedComments(s.as(t, bob), &postv1.ListLikedCommentsRequest{Limit: 1, TextLimit: 4})
	if err != nil || comments.GetTotal() != 2 || len(comments.GetComments()) != 1 || comments.GetNextCursor() == "" {
		t.Fatalf("ListLikedComments = %v, %v", comments, err)
	}
	if c := comments.GetComments()[0]; c.GetId() != one || c.GetText() != "перв" || !c.GetTextCut() || !c.GetLiked() || c.GetPostId() != postID {
		t.Fatalf("a liked comment = %v", c)
	}

	comments, err = s.client.ListLikedComments(s.as(t, bob), &postv1.ListLikedCommentsRequest{Limit: 1, Cursor: comments.GetNextCursor()})
	if err != nil || len(comments.GetComments()) != 1 || comments.GetComments()[0].GetId() != two || comments.GetNextCursor() != "" {
		t.Fatalf("the second page = %v, %v", comments, err)
	}

	// The comments of an archived post leave with it.
	if _, err = s.client.ArchivePost(s.as(t, ann), &postv1.ArchivePostRequest{Id: postID}); err != nil {
		t.Fatal(err)
	}
	if comments, err = s.client.ListLikedComments(s.as(t, bob), &postv1.ListLikedCommentsRequest{}); err != nil || comments.GetTotal() != 0 || len(comments.GetComments()) != 0 {
		t.Fatalf("after the archive = %v, %v", comments, err)
	}
	if _, err = s.client.ListLikedComments(context.Background(), &postv1.ListLikedCommentsRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("anonymous: %v", err)
	}

	album, err := s.client.CreateAlbum(s.as(t, ann), &postv1.CreateAlbumRequest{Name: "Лето"})
	if err != nil {
		t.Fatal(err)
	}
	sea, hills := uuid.Must(uuid.NewV7()).String(), uuid.Must(uuid.NewV7()).String()
	if _, err = s.client.AddAlbumPhotos(s.as(t, ann), &postv1.AddAlbumPhotosRequest{
		AlbumId: album.GetAlbum().GetId(), Photos: []*postv1.NewAlbumPhoto{{PhotoId: sea, Description: "Море"}, {PhotoId: hills}},
	}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{sea, hills} {
		if _, err = s.client.LikeAlbumPhoto(s.as(t, bob), &postv1.LikeAlbumPhotoRequest{PhotoId: id}); err != nil {
			t.Fatal(err)
		}
	}

	photos, err := s.client.ListLikedAlbumPhotos(s.as(t, bob), &postv1.ListLikedAlbumPhotosRequest{})
	if err != nil || photos.GetTotal() != 2 || len(photos.GetPhotos()) != 2 {
		t.Fatalf("ListLikedAlbumPhotos = %v, %v", photos, err)
	}
	if p := photos.GetPhotos()[0]; p.GetId() != hills || !p.GetLiked() || p.GetMine() {
		t.Fatalf("the newest liked photo = %v", p)
	}
	if !slices.Equal(photos.GetOwnerIds(), []string{ann.String(), ann.String()}) {
		t.Fatalf("the owners = %v", photos.GetOwnerIds())
	}

	if _, err = s.client.DeleteAlbumPhoto(s.as(t, ann), &postv1.DeleteAlbumPhotoRequest{Id: hills}); err != nil {
		t.Fatal(err)
	}
	photos, err = s.client.ListLikedAlbumPhotos(s.as(t, bob), &postv1.ListLikedAlbumPhotosRequest{})
	if err != nil || photos.GetTotal() != 1 || photos.GetPhotos()[0].GetDescription() != "Море" {
		t.Fatalf("after the delete = %v, %v", photos, err)
	}
	if _, err = s.client.ListLikedAlbumPhotos(context.Background(), &postv1.ListLikedAlbumPhotosRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("anonymous: %v", err)
	}
}

func TestOwnPosts(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, bob := uuid.New(), uuid.New()

	write := func(author, wall uuid.UUID, text string) string {
		t.Helper()

		resp, err := s.client.CreatePost(s.as(t, author), &postv1.CreatePostRequest{Text: text, WallId: wall.String()})
		if err != nil {
			t.Fatal(err)
		}

		return resp.GetPost().GetId()
	}

	own := write(ann, ann, "у себя")
	guest := write(ann, bob, "у Боба")
	write(bob, ann, "Боб у Анны")
	write(ann, ann, "ещё у себя")

	page, err := s.client.ListOwnPosts(s.as(t, ann), &postv1.ListOwnPostsRequest{Limit: 2})
	if err != nil || page.GetTotal() != 3 || page.GetNextCursor() == "" {
		t.Fatalf("ListOwnPosts = %v, %v", page, err)
	}
	if got := postTexts(page.GetPosts()); !slices.Equal(got, []string{"ещё у себя", "у Боба"}) {
		t.Fatalf("the first page = %v", got)
	}
	if !page.GetPosts()[0].GetMine() {
		t.Fatal("an own post is not mine")
	}

	page, err = s.client.ListOwnPosts(s.as(t, ann), &postv1.ListOwnPostsRequest{Limit: 2, Cursor: page.GetNextCursor()})
	if err != nil || page.GetNextCursor() != "" || !slices.Equal(postTexts(page.GetPosts()), []string{"у себя"}) {
		t.Fatalf("the second page = %v, %v", page, err)
	}

	// What she archived on her wall stays hers to read; what Bob archived
	// on his is his alone.
	if _, err = s.client.ArchivePost(s.as(t, ann), &postv1.ArchivePostRequest{Id: own}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.client.ArchivePost(s.as(t, bob), &postv1.ArchivePostRequest{Id: guest}); err != nil {
		t.Fatal(err)
	}

	page, err = s.client.ListOwnPosts(s.as(t, ann), &postv1.ListOwnPostsRequest{})
	if err != nil || page.GetTotal() != 2 || !slices.Equal(postTexts(page.GetPosts()), []string{"ещё у себя", "у себя"}) {
		t.Fatalf("after the archives = %v, %v", page, err)
	}

	if _, err = s.client.ListOwnPosts(context.Background(), &postv1.ListOwnPostsRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("anonymous: %v", err)
	}
}
