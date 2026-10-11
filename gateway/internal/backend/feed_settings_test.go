package backend

import (
	"context"
	"slices"
	"testing"

	"github.com/vexner67/freenet/gateway/internal/failure"
	"github.com/vexner67/freenet/gateway/internal/model"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	userv1 "github.com/vexner67/freenet/user/api/user/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// ignoreAPI is the user service keeping one reader's ignored sources.
type ignoreAPI struct {
	userv1.UserServiceClient
	md      metadata.MD
	ignored []string
}

func (f *ignoreAPI) IgnoreSource(ctx context.Context, req *userv1.IgnoreSourceRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	if len(f.ignored) == 2 {
		return nil, status.Error(codes.FailedPrecondition, "too many ignored sources")
	}
	f.ignored = append([]string{req.GetSourceId()}, f.ignored...)

	return &emptypb.Empty{}, nil
}

func (f *ignoreAPI) UnignoreSources(ctx context.Context, req *userv1.UnignoreSourcesRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	f.ignored = slices.DeleteFunc(f.ignored, func(id string) bool { return slices.Contains(req.GetSourceIds(), id) })

	return &emptypb.Empty{}, nil
}

func (f *ignoreAPI) ListIgnoredSources(ctx context.Context, _ *userv1.ListIgnoredSourcesRequest, _ ...grpc.CallOption) (*userv1.ListIgnoredSourcesResponse, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)

	return &userv1.ListIgnoredSourcesResponse{SourceIds: f.ignored}, nil
}

func TestIgnoredSources(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	api := &ignoreAPI{}
	users := NewUsers(api)

	for _, id := range []string{"u2", "k1"} {
		if err := users.Ignore(ctx, "tok", id); err != nil {
			t.Fatalf("Ignore(%s): %v", id, err)
		}
	}
	if got := api.md.Get("authorization"); len(got) != 1 || got[0] != "Bearer tok" {
		t.Fatalf("authorization metadata = %v", got)
	}

	err := users.Ignore(ctx, "tok", "u3")
	if f := failure.From(err); f.Kind != failure.Conflict || f.Message != "too many ignored sources" {
		t.Fatalf("over the limit: %v", err)
	}

	if got, err := users.IgnoredIDs(ctx, "tok"); err != nil || !slices.Equal(got, []string{"k1", "u2"}) {
		t.Fatalf("IgnoredIDs = %v, %v", got, err)
	}

	if err = users.Unignore(ctx, "tok", []string{"u2", "k1"}); err != nil {
		t.Fatalf("Unignore: %v", err)
	}
	if got, _ := users.IgnoredIDs(ctx, "tok"); len(got) != 0 {
		t.Fatalf("IgnoredIDs after Unignore = %v", got)
	}
}

type feedAPI struct {
	postv1.PostServiceClient
	req *postv1.ListFeedRequest
}

func (f *feedAPI) ListFeed(_ context.Context, req *postv1.ListFeedRequest, _ ...grpc.CallOption) (*postv1.ListFeedResponse, error) {
	f.req = req
	return &postv1.ListFeedResponse{}, nil
}

// The settings of the feed go to the post service as they are.
func TestFeedSettings(t *testing.T) {
	t.Parallel()

	api := &feedAPI{}

	_, err := NewPosts(api).Feed(context.Background(), "tok", model.FeedSources{
		Everyone: true, OwnOnly: true, Ignored: []string{"k1", "u2"}, Hidden: []string{"u3"},
	}, "p9", 5)
	if err != nil {
		t.Fatal(err)
	}

	got := api.req
	if !got.GetEveryone() || !got.GetOwnOnly() || !slices.Equal(got.GetIgnoredWallIds(), []string{"k1", "u2"}) ||
		!slices.Equal(got.GetHiddenAuthorIds(), []string{"u3"}) || got.GetCursor() != "p9" || got.GetLimit() != 5 {
		t.Fatalf("ListFeed got %v", got)
	}
}
