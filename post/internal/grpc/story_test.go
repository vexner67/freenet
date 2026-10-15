package grpc_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestStories(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, bob := uuid.New(), uuid.New()
	asBob := s.reads(t, bob, []uuid.UUID{ann})

	told, err := s.client.CreateStory(s.as(t, ann), &postv1.CreateStoryRequest{PhotoId: uuid.NewString(), Text: " Утро "})
	story := told.GetStory()
	if err != nil || story.GetText() != "Утро" || story.GetAuthorId() != ann.String() || !story.GetMine() ||
		!story.GetExpiresAt().AsTime().After(story.GetCreatedAt().AsTime()) {
		t.Fatalf("CreateStory = %v, %v", told, err)
	}

	feed, err := s.client.ListFeedStories(asBob, &postv1.ListFeedStoriesRequest{})
	if err != nil || len(feed.GetStories()) != 1 || feed.GetStories()[0].GetMine() || feed.GetStories()[0].GetViewed() {
		t.Fatalf("ListFeedStories = %v, %v", feed, err)
	}

	if _, err = s.client.ViewStory(asBob, &postv1.ViewStoryRequest{Id: story.GetId()}); err != nil {
		t.Fatal(err)
	}
	if feed, _ = s.client.ListFeedStories(asBob, &postv1.ListFeedStoriesRequest{}); !feed.GetStories()[0].GetViewed() {
		t.Fatal("bob viewed it")
	}

	own, err := s.client.ListUserStories(s.as(t, ann), &postv1.ListUserStoriesRequest{UserId: ann.String()})
	if err != nil || len(own.GetStories()) != 1 || own.GetStories()[0].GetViews() != 1 {
		t.Fatalf("ListUserStories = %v, %v", own, err)
	}

	viewers, err := s.client.ListStoryViewers(s.as(t, ann), &postv1.ListStoryViewersRequest{Id: story.GetId()})
	if err != nil || viewers.GetTotal() != 1 || len(viewers.GetViewers()) != 1 || viewers.GetViewers()[0].GetUserId() != bob.String() {
		t.Fatalf("ListStoryViewers = %v, %v", viewers, err)
	}

	tests := []struct {
		name string
		call func() error
		code codes.Code
	}{
		{"no token", func() error {
			_, err := s.client.ListFeedStories(context.Background(), &postv1.ListFeedStoriesRequest{})
			return err
		}, codes.Unauthenticated},
		{"no photo", func() error {
			_, err := s.client.CreateStory(s.as(t, ann), &postv1.CreateStoryRequest{PhotoId: "x"})
			return err
		}, codes.InvalidArgument},
		{"long text", func() error {
			_, err := s.client.CreateStory(s.as(t, ann), &postv1.CreateStoryRequest{PhotoId: uuid.NewString(), Text: strings.Repeat("я", 201)})
			return err
		}, codes.InvalidArgument},
		{"viewers of another's", func() error {
			_, err := s.client.ListStoryViewers(asBob, &postv1.ListStoryViewersRequest{Id: story.GetId()})
			return err
		}, codes.PermissionDenied},
		{"delete another's", func() error {
			_, err := s.client.DeleteStory(asBob, &postv1.DeleteStoryRequest{Id: story.GetId()})
			return err
		}, codes.PermissionDenied},
		{"view of none", func() error {
			_, err := s.client.ViewStory(asBob, &postv1.ViewStoryRequest{Id: uuid.NewString()})
			return err
		}, codes.NotFound},
		{"bad cursor", func() error {
			_, err := s.client.ListStoryViewers(s.as(t, ann), &postv1.ListStoryViewersRequest{Id: story.GetId(), Cursor: "x"})
			return err
		}, codes.InvalidArgument},
	}

	for _, tt := range tests {
		if code := status.Code(tt.call()); code != tt.code {
			t.Errorf("%s: code %v, want %v", tt.name, code, tt.code)
		}
	}

	if _, err = s.client.DeleteStory(s.as(t, ann), &postv1.DeleteStoryRequest{Id: story.GetId()}); err != nil {
		t.Fatal(err)
	}
	if feed, _ = s.client.ListFeedStories(asBob, &postv1.ListFeedStoriesRequest{}); len(feed.GetStories()) != 0 {
		t.Fatal("the deleted story stays")
	}
}
