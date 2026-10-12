package grpc_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// A closed page shows its wall to its owner and to those the owner
// follows; a closed club — to its members.
func TestClosedWalls(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, bob, cat, club := uuid.New(), uuid.New(), uuid.New(), uuid.New()

	created, err := s.client.CreatePost(s.as(t, ann), &postv1.CreatePostRequest{Text: "анна"})
	if err != nil {
		t.Fatal(err)
	}
	id := created.GetPost().GetId()

	if _, err = s.client.LikePost(s.as(t, cat), &postv1.LikePostRequest{PostId: id}); err != nil {
		t.Fatal(err)
	}

	// Ann follows bob, and cat follows ann: bob is let in, cat is not.
	s.reads(t, ann, []uuid.UUID{bob})
	s.reads(t, cat, []uuid.UUID{ann})

	closeWall := &postv1.CloseWallRequest{WallId: ann.String()}
	for range 2 {
		if _, err = s.client.CloseWall(context.Background(), closeWall); err != nil {
			t.Fatalf("CloseWall: %v", err)
		}
	}

	sees := func(ctx context.Context) bool {
		t.Helper()

		_, err := s.client.GetPost(ctx, &postv1.GetPostRequest{Id: id})
		if err != nil && status.Code(err) != codes.NotFound {
			t.Fatalf("GetPost: %v", err)
		}

		return err == nil
	}

	for name, ctx := range map[string]context.Context{"ann": s.as(t, ann), "bob": s.as(t, bob)} {
		if !sees(ctx) {
			t.Errorf("%s does not see the post of the closed page", name)
		}
		if _, err = s.client.ListPosts(ctx, &postv1.ListPostsRequest{WallId: ann.String()}); err != nil {
			t.Errorf("%s reads the closed wall: %v", name, err)
		}
	}

	for name, ctx := range map[string]context.Context{"cat": s.as(t, cat), "nobody": context.Background()} {
		if sees(ctx) {
			t.Errorf("%s sees the post of the closed page", name)
		}
		if _, err = s.client.ListPosts(ctx, &postv1.ListPostsRequest{WallId: ann.String()}); status.Code(err) != codes.PermissionDenied {
			t.Errorf("%s reads the closed wall: %v", name, err)
		}
		if many, err := s.client.GetPosts(ctx, &postv1.GetPostsRequest{Ids: []string{id}}); err != nil || len(many.GetPosts()) != 0 {
			t.Errorf("%s gets the post in a batch: %v", name, err)
		}
		if _, err = s.client.ListComments(ctx, &postv1.ListCommentsRequest{PostId: id}); status.Code(err) != codes.NotFound {
			t.Errorf("%s reads the comments: %v", name, err)
		}
	}

	// Cat follows ann and still reads nothing of hers, liked or not.
	if page, err := s.client.ListFeed(s.as(t, cat), &postv1.ListFeedRequest{}); err != nil || len(page.GetPosts()) != 0 {
		t.Errorf("cat's feed shows %d posts, %v", len(page.GetPosts()), err)
	}
	if page, err := s.client.ListFeed(s.as(t, cat), &postv1.ListFeedRequest{Everyone: true}); err != nil || len(page.GetPosts()) != 0 {
		t.Errorf("everyone's feed shows cat %d posts, %v", len(page.GetPosts()), err)
	}
	if page, err := s.client.ListFeed(s.as(t, bob), &postv1.ListFeedRequest{Everyone: true}); err != nil || len(page.GetPosts()) != 1 {
		t.Errorf("everyone's feed shows bob %d posts, %v", len(page.GetPosts()), err)
	}
	if page, err := s.client.ListLikedPosts(s.as(t, cat), &postv1.ListLikedPostsRequest{}); err != nil || len(page.GetPosts()) != 0 {
		t.Errorf("cat's faves show %d posts, %v", len(page.GetPosts()), err)
	}
	if _, err = s.client.CreateComment(s.as(t, cat), &postv1.CreateCommentRequest{PostId: id, Text: "ау"}); status.Code(err) != codes.NotFound {
		t.Errorf("cat comments the closed page: %v", err)
	}
	if _, err = s.client.RepostPost(s.as(t, cat), &postv1.RepostPostRequest{PostId: id}); status.Code(err) != codes.NotFound {
		t.Errorf("cat shares the post of the closed page: %v", err)
	}

	if _, err = s.client.OpenWall(context.Background(), &postv1.OpenWallRequest{WallId: ann.String()}); err != nil {
		t.Fatalf("OpenWall: %v", err)
	}
	if !sees(context.Background()) {
		t.Error("the opened page still hides its post")
	}

	// A closed club lets its members in.
	onClub, err := s.client.CreatePost(s.as(t, ann), &postv1.CreatePostRequest{Text: "клуб", WallId: club.String(), ClubWall: true})
	if err != nil {
		t.Fatal(err)
	}
	id = onClub.GetPost().GetId()

	if _, err = s.client.CloseWall(context.Background(), &postv1.CloseWallRequest{WallId: club.String(), Club: true}); err != nil {
		t.Fatalf("CloseWall: %v", err)
	}
	if sees(s.as(t, bob)) {
		t.Error("a stranger sees the post of the closed club")
	}
	if !sees(s.reads(t, bob, nil, club)) {
		t.Error("a member does not see the post of the closed club")
	}

	for name, req := range map[string]*postv1.CloseWallRequest{"no wall": {}, "a bad wall": {WallId: "x"}} {
		if _, err = s.client.CloseWall(context.Background(), req); status.Code(err) != codes.InvalidArgument {
			t.Errorf("%s: %v, want InvalidArgument", name, err)
		}
	}
	if _, err = s.client.OpenWall(context.Background(), &postv1.OpenWallRequest{WallId: "x"}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("opening a bad wall: %v", err)
	}
}
