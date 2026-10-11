package grpc_test

import (
	"context"
	"slices"
	"testing"

	"github.com/google/uuid"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestIgnoredSources(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, annID := s.member(t, "ann")
	bob, bobID := s.member(t, "bob")
	club := uuid.NewString()

	for _, source := range []string{bobID, club, bobID} {
		if _, err := s.client.IgnoreSource(ann, &userv1.IgnoreSourceRequest{SourceId: source}); err != nil {
			t.Fatalf("IgnoreSource: %v", err)
		}
	}

	list, err := s.client.ListIgnoredSources(ann, &userv1.ListIgnoredSourcesRequest{})
	if err != nil || !slices.Equal(list.GetSourceIds(), []string{club, bobID}) {
		t.Fatalf("ann ignores %v, %v", list.GetSourceIds(), err)
	}
	if list, err = s.client.ListIgnoredSources(bob, &userv1.ListIgnoredSourcesRequest{}); err != nil || len(list.GetSourceIds()) != 0 {
		t.Fatalf("bob ignores %v, %v", list.GetSourceIds(), err)
	}

	if _, err = s.client.UnignoreSources(ann, &userv1.UnignoreSourcesRequest{SourceIds: []string{club}}); err != nil {
		t.Fatalf("UnignoreSources: %v", err)
	}
	if list, _ = s.client.ListIgnoredSources(ann, &userv1.ListIgnoredSourcesRequest{}); !slices.Equal(list.GetSourceIds(), []string{bobID}) {
		t.Fatalf("after UnignoreSources ann ignores %v", list.GetSourceIds())
	}

	tooMany := make([]string, 11)
	for i := range tooMany {
		tooMany[i] = uuid.NewString()
	}

	for name, call := range map[string]func() error{
		"oneself": func() error {
			_, err := s.client.IgnoreSource(ann, &userv1.IgnoreSourceRequest{SourceId: annID})
			return err
		},
		"not an id": func() error {
			_, err := s.client.IgnoreSource(ann, &userv1.IgnoreSourceRequest{SourceId: "bob"})
			return err
		},
		"nothing to unignore": func() error {
			_, err := s.client.UnignoreSources(ann, &userv1.UnignoreSourcesRequest{})
			return err
		},
		"too many to unignore": func() error {
			_, err := s.client.UnignoreSources(ann, &userv1.UnignoreSourcesRequest{SourceIds: tooMany})
			return err
		},
	} {
		if got := status.Code(call()); got != codes.InvalidArgument {
			t.Errorf("%s: code = %v", name, got)
		}
	}

	_, err = s.client.ListIgnoredSources(context.Background(), &userv1.ListIgnoredSourcesRequest{})
	if got := status.Code(err); got != codes.Unauthenticated {
		t.Fatalf("without a token: code = %v", got)
	}
}

func TestIgnoredSources_Limit(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, _ := s.member(t, "ann")

	for range 50 {
		if _, err := s.client.IgnoreSource(ann, &userv1.IgnoreSourceRequest{SourceId: uuid.NewString()}); err != nil {
			t.Fatal(err)
		}
	}

	_, err := s.client.IgnoreSource(ann, &userv1.IgnoreSourceRequest{SourceId: uuid.NewString()})
	if st := status.Convert(err); st.Code() != codes.FailedPrecondition || st.Message() != "too many ignored sources" {
		t.Fatalf("the 51st source = %v", err)
	}
}
