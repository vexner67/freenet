package grpc_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// A deleted page is not counted among those who liked; restored, it is
// again, and erased, never.
func TestHoldUserReactions(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, bob := uuid.New(), uuid.New()

	created, err := s.client.CreatePost(s.as(t, ann), &postv1.CreatePostRequest{Text: "запись"})
	if err != nil {
		t.Fatal(err)
	}
	id := created.GetPost().GetId()

	if _, err = s.client.LikePost(s.as(t, bob), &postv1.LikePostRequest{PostId: id}); err != nil {
		t.Fatal(err)
	}

	likes := func() int32 {
		t.Helper()

		got, err := s.client.GetPost(context.Background(), &postv1.GetPostRequest{Id: id})
		if err != nil {
			t.Fatal(err)
		}

		return got.GetPost().GetLikeCount()
	}

	held := &postv1.HoldUserReactionsRequest{UserId: bob.String()}
	if _, err = s.client.HoldUserReactions(context.Background(), held); err != nil || likes() != 0 {
		t.Fatalf("HoldUserReactions: %v, likes %d", err, likes())
	}

	released := &postv1.ReleaseUserReactionsRequest{UserId: bob.String()}
	if _, err = s.client.ReleaseUserReactions(context.Background(), released); err != nil || likes() != 1 {
		t.Fatalf("ReleaseUserReactions: %v, likes %d", err, likes())
	}

	// Erased while deleted: nothing comes back.
	if _, err = s.client.HoldUserReactions(context.Background(), held); err != nil {
		t.Fatal(err)
	}
	if _, err = s.client.PurgeUser(context.Background(), &postv1.PurgeUserRequest{UserId: bob.String()}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.client.ReleaseUserReactions(context.Background(), released); err != nil || likes() != 0 {
		t.Fatalf("release of the erased: %v, likes %d", err, likes())
	}

	if _, err = s.client.HoldUserReactions(context.Background(), &postv1.HoldUserReactionsRequest{UserId: "x"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("HoldUserReactions of no user: %v", err)
	}
	if _, err = s.client.ReleaseUserReactions(context.Background(), &postv1.ReleaseUserReactionsRequest{UserId: "x"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("ReleaseUserReactions of no user: %v", err)
	}
}
