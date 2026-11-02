package grpc_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	federationv1 "github.com/vexner67/freenet/federation/api/federation/v1"
	"github.com/vexner67/freenet/federation/internal/fedi"
	federationgrpc "github.com/vexner67/freenet/federation/internal/grpc"
	"github.com/vexner67/freenet/federation/internal/service"
	"github.com/vexner67/freenet/pkg/jwtauth"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type newsFake struct {
	fake

	query   service.NewsQuery
	posts   []service.NewsPost
	picture string
}

func (f *newsFake) News(_ context.Context, q service.NewsQuery) ([]service.NewsPost, error) {
	f.query = q
	return f.posts, f.err
}

func (f *newsFake) Picture(_ context.Context, _ uuid.UUID, index int) (service.Media, error) {
	f.picture = fmt.Sprintf("picture %d", index)
	return service.Media{Body: []byte("png"), ContentType: "image/png"}, f.err
}

func (f *newsFake) VideoCover(_ context.Context, _ uuid.UUID, index int) (service.Media, error) {
	f.picture = fmt.Sprintf("cover %d", index)
	return service.Media{Body: []byte("jpeg"), ContentType: "image/jpeg"}, f.err
}

func (f *newsFake) Subscribe(context.Context, uuid.UUID, string) (service.Account, error) {
	return service.Account{}, f.err
}

func newsHandler(t *testing.T, f *newsFake) *federationgrpc.FederationHandler {
	t.Helper()

	site, err := fedi.NewSite("https://freenet.example")
	if err != nil {
		t.Fatal(err)
	}

	return federationgrpc.NewFederationHandler(f, f, site)
}

func TestListNews(t *testing.T) {
	t.Parallel()

	author := fedi.RestoreRemote("https://m.example/users/ann", "", "", "", "",
		fedi.Profile{Name: "Ann", Username: "ann", Icon: "https://m.example/a.png"}, time.Now())
	published := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	post := fedi.RestoreRemotePost(uuid.New(), "https://m.example/notes/1", author.ID(), "", "hello", "", []fedi.Image{
		{URL: "https://m.example/1.png", Description: "a cat"},
	}, []fedi.Video{
		{URL: "https://m.example/v.mp4", Blurhash: "LEHV6nWB2yk8pyo0adR*.7kCMdnj", Width: 1280, Height: 720, Duration: 84600 * time.Millisecond, Description: "a dog"},
	}, false, "en", published, time.Time{})

	reader := uuid.New()
	before := uuid.New()
	ctx := jwtauth.ContextWithIdentity(context.Background(), jwtauth.Identity{UserID: reader})

	f := &newsFake{posts: []service.NewsPost{{Post: post, Author: author}}}
	h := newsHandler(t, f)

	resp, err := h.ListNews(ctx, &federationv1.ListNewsRequest{
		Scope: federationv1.NewsScope_NEWS_SCOPE_FEATURED, Before: before.String(), Limit: 20, HideSensitive: true,
	})
	if err != nil {
		t.Fatalf("ListNews: %v", err)
	}

	want := service.NewsQuery{Featured: true, Reader: reader, HideSensitive: true, Before: before, Limit: 20}
	if f.query.Featured != want.Featured || f.query.Reader != want.Reader || f.query.HideSensitive != want.HideSensitive ||
		f.query.Before != want.Before || f.query.Limit != want.Limit {
		t.Errorf("query = %+v, want %+v", f.query, want)
	}

	got := resp.GetPosts()[0]
	if got.GetText() != "hello" || got.GetPageUrl() != "https://m.example/notes/1" || got.GetAuthor().GetHandle() != "@ann@m.example" ||
		!got.GetAuthor().GetHasAvatar() || got.GetImages()[0].GetDescription() != "a cat" || got.GetUpdatedAt() != nil ||
		!got.GetPublishedAt().AsTime().Equal(published) {
		t.Errorf("post = %+v", got)
	}

	if v := got.GetVideos(); len(v) != 1 || v[0].GetBlurhash() != "LEHV6nWB2yk8pyo0adR*.7kCMdnj" || v[0].GetWidth() != 1280 ||
		v[0].GetHeight() != 720 || v[0].GetDurationSeconds() != 85 || v[0].GetDescription() != "a dog" {
		t.Errorf("videos = %+v", v)
	}

	for name, req := range map[string]*federationv1.ListNewsRequest{
		"no scope":  {Limit: 20},
		"no limit":  {Scope: federationv1.NewsScope_NEWS_SCOPE_SUBSCRIBED},
		"big limit": {Scope: federationv1.NewsScope_NEWS_SCOPE_SUBSCRIBED, Limit: 101},
		"cursor":    {Scope: federationv1.NewsScope_NEWS_SCOPE_SUBSCRIBED, Limit: 20, Before: "x"},
	} {
		if _, err := h.ListNews(ctx, req); status.Code(err) != codes.InvalidArgument {
			t.Errorf("%s: code %v, want InvalidArgument", name, status.Code(err))
		}
	}

	if _, err := h.ListNews(context.Background(), &federationv1.ListNewsRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Errorf("without a token: code %v, want Unauthenticated", status.Code(err))
	}
}

func TestNewsErrors(t *testing.T) {
	t.Parallel()

	ctx := jwtauth.ContextWithIdentity(context.Background(), jwtauth.Identity{UserID: uuid.New()})

	tests := []struct {
		name string
		err  error
		code codes.Code
		msg  string
	}{
		{"invalid address", fedi.ErrInvalidAddress, codes.InvalidArgument, "invalid fediverse address"},
		// Not what the other server did: that is no business of the caller.
		{"not found", fmt.Errorf("%w: %w", fedi.ErrAccountNotFound, fedi.ErrGone), codes.NotFound, "fediverse account not found"},
		{"too many lookups", fedi.ErrTooManyLookups, codes.ResourceExhausted, "too many fediverse lookups"},
		{"too many", fedi.ErrTooManySubscriptions, codes.FailedPrecondition, "too many fediverse subscriptions"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newsHandler(t, &newsFake{fake: fake{err: tt.err}})
			err := call(func() (any, error) {
				return h.Subscribe(ctx, &federationv1.SubscribeRequest{Address: "@ann@m.example"})
			})

			st, _ := status.FromError(err)
			if st.Code() != tt.code || st.Message() != tt.msg {
				t.Errorf("status = %v %q, want %v %q", st.Code(), st.Message(), tt.code, tt.msg)
			}
		})
	}
}

func TestGetPicture(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		req  *federationv1.GetPictureRequest
		want string
	}{
		{"image", &federationv1.GetPictureRequest{PostId: uuid.NewString(), Index: 1}, "picture 1"},
		{"cover of a video", &federationv1.GetPictureRequest{PostId: uuid.NewString(), Index: 2, Video: true}, "cover 2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := &newsFake{}
			if _, err := newsHandler(t, f).GetPicture(context.Background(), tt.req); err != nil || f.picture != tt.want {
				t.Fatalf("GetPicture read %q, %v; want %q", f.picture, err, tt.want)
			}
		})
	}
}
