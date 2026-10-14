package grpc_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestPostponed(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, bob := uuid.New(), uuid.New()
	at := timestamppb.New(time.Now().Add(time.Hour))

	postponed, err := s.client.PostponePost(s.as(t, ann), &postv1.PostponePostRequest{
		Text: " Завтра в 10 ", PublishAt: at, Nsfw: true, MentionedIds: []string{bob.String()},
	})
	p := postponed.GetPostponed()
	if err != nil || p.GetText() != "Завтра в 10" || p.GetWallId() != ann.String() || p.GetAuthorId() != ann.String() || !p.GetNsfw() ||
		!p.GetPublishAt().AsTime().Equal(at.AsTime().Truncate(time.Microsecond)) || len(p.GetMentionedIds()) != 1 {
		t.Fatalf("PostponePost = %v, %v", postponed, err)
	}

	list, err := s.client.ListPostponed(s.as(t, ann), &postv1.ListPostponedRequest{WallId: ann.String(), TextLimit: 6})
	if err != nil || len(list.GetPostponed()) != 1 || list.GetPostponed()[0].GetText() != "Завтра" || !list.GetPostponed()[0].GetTextCut() {
		t.Fatalf("ListPostponed = %v, %v", list, err)
	}

	edited, err := s.client.EditPostponed(s.as(t, ann), &postv1.EditPostponedRequest{Id: p.GetId(), Text: "Послезавтра", PublishAt: at})
	if err != nil || edited.GetPostponed().GetText() != "Послезавтра" || edited.GetPostponed().GetNsfw() || len(edited.GetPostponed().GetMentionedIds()) != 0 {
		t.Fatalf("EditPostponed = %v, %v", edited, err)
	}

	// The wall shows it only once it is published.
	wall, err := s.client.ListPosts(context.Background(), &postv1.ListPostsRequest{WallId: ann.String()})
	if err != nil || len(wall.GetPosts()) != 0 {
		t.Fatalf("the wall = %v, %v", wall, err)
	}
	published, err := s.client.PublishPostponed(s.as(t, ann), &postv1.PublishPostponedRequest{Id: p.GetId()})
	if err != nil || published.GetPost().GetText() != "Послезавтра" || !published.GetPost().GetMine() {
		t.Fatalf("PublishPostponed = %v, %v", published, err)
	}
	if wall, err = s.client.ListPosts(context.Background(), &postv1.ListPostsRequest{WallId: ann.String()}); err != nil || len(wall.GetPosts()) != 1 {
		t.Fatalf("the wall = %v, %v", wall, err)
	}

	another, err := s.client.PostponePost(s.as(t, ann), &postv1.PostponePostRequest{Text: "ещё", PublishAt: at})
	if err != nil {
		t.Fatal(err)
	}
	id := another.GetPostponed().GetId()

	tests := []struct {
		name string
		call func() error
		want codes.Code
	}{
		{"without a token", func() error {
			_, err := s.client.PostponePost(context.Background(), &postv1.PostponePostRequest{Text: "x", PublishAt: at})
			return err
		}, codes.Unauthenticated},
		{"nothing", func() error {
			_, err := s.client.PostponePost(s.as(t, ann), &postv1.PostponePostRequest{PublishAt: at})
			return err
		}, codes.InvalidArgument},
		{"no time", func() error {
			_, err := s.client.PostponePost(s.as(t, ann), &postv1.PostponePostRequest{Text: "x"})
			return err
		}, codes.InvalidArgument},
		{"in the past", func() error {
			_, err := s.client.PostponePost(s.as(t, ann), &postv1.PostponePostRequest{Text: "x", PublishAt: timestamppb.New(time.Now().Add(-time.Minute))})
			return err
		}, codes.InvalidArgument},
		{"on someone else's wall", func() error {
			_, err := s.client.PostponePost(s.as(t, ann), &postv1.PostponePostRequest{Text: "x", WallId: bob.String(), PublishAt: at})
			return err
		}, codes.InvalidArgument},
		{"someone else's list", func() error {
			_, err := s.client.ListPostponed(s.as(t, bob), &postv1.ListPostponedRequest{WallId: ann.String()})
			return err
		}, codes.PermissionDenied},
		{"someone else's", func() error {
			_, err := s.client.GetPostponed(s.as(t, bob), &postv1.GetPostponedRequest{Id: id})
			return err
		}, codes.NotFound},
		{"edited by someone else", func() error {
			_, err := s.client.EditPostponed(s.as(t, bob), &postv1.EditPostponedRequest{Id: id, Text: "x", PublishAt: at})
			return err
		}, codes.NotFound},
		{"deleted by someone else", func() error {
			_, err := s.client.DeletePostponed(s.as(t, bob), &postv1.DeletePostponedRequest{Id: id})
			return err
		}, codes.NotFound},
		{"published twice", func() error {
			_, err := s.client.PublishPostponed(s.as(t, ann), &postv1.PublishPostponedRequest{Id: p.GetId()})
			return err
		}, codes.NotFound},
		{"a bad id", func() error {
			_, err := s.client.GetPostponed(s.as(t, ann), &postv1.GetPostponedRequest{Id: "x"})
			return err
		}, codes.InvalidArgument},
	}

	for _, tt := range tests {
		if code := status.Code(tt.call()); code != tt.want {
			t.Errorf("%s: code = %v, want %v", tt.name, code, tt.want)
		}
	}

	if got, err := s.client.GetPostponed(s.as(t, ann), &postv1.GetPostponedRequest{Id: id}); err != nil || got.GetPostponed().GetText() != "ещё" {
		t.Fatalf("GetPostponed = %v, %v", got, err)
	}
	if _, err = s.client.DeletePostponed(s.as(t, ann), &postv1.DeletePostponedRequest{Id: id}); err != nil {
		t.Fatal(err)
	}
	if list, err = s.client.ListPostponed(s.as(t, ann), &postv1.ListPostponedRequest{WallId: ann.String()}); err != nil || len(list.GetPostponed()) != 0 {
		t.Fatalf("ListPostponed after delete = %v, %v", list, err)
	}
}
