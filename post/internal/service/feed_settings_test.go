package service_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/service"
)

// openvk's "Настройки ленты": the walls a reader ignores and the posts
// people wrote on walls of others.
func TestPostService_FeedSettings(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()
	svc, _ := newWallServices(uuid.New(), uuid.New())

	write(t, svc, ann, "анна у себя")
	write(t, svc, bob, "боб у себя")
	if _, err := svc.Create(ctx, service.CreateInput{WallID: ann, AuthorID: bob, Content: textContent(t, "боб у анны")}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, service.CreateInput{WallID: bob, AuthorID: ann, Content: textContent(t, "анна у боба")}); err != nil {
		t.Fatal(err)
	}

	feed := func(sources service.FeedSources) []string {
		t.Helper()

		page, err := svc.Feed(ctx, cat, sources, uuid.Nil, 10)
		if err != nil {
			t.Fatalf("Feed(%+v): %v", sources, err)
		}

		return texts(page.Posts)
	}

	tests := []struct {
		name    string
		sources service.FeedSources
		want    []string
	}{
		{"everyone", service.FeedSources{Everyone: true}, []string{"анна у боба", "боб у анны", "боб у себя", "анна у себя"}},
		{"everyone, own only", service.FeedSources{Everyone: true, OwnOnly: true}, []string{"боб у себя", "анна у себя"}},
		{"everyone but bob's wall", service.FeedSources{Everyone: true, Ignored: []uuid.UUID{bob}}, []string{"боб у анны", "анна у себя"}},
		{
			"the followed", service.FeedSources{Authors: []uuid.UUID{ann, bob}, Walls: []uuid.UUID{ann, bob}},
			[]string{"анна у боба", "боб у анны", "боб у себя", "анна у себя"},
		},
		{
			"the followed, own only", service.FeedSources{Authors: []uuid.UUID{ann, bob}, Walls: []uuid.UUID{ann, bob}, OwnOnly: true},
			[]string{"боб у себя", "анна у себя"},
		},
		{
			// Ann is followed, so what she wrote comes with her — but not
			// from a wall the reader ignores.
			"the followed but bob's wall", service.FeedSources{Authors: []uuid.UUID{ann}, Walls: []uuid.UUID{ann}, Ignored: []uuid.UUID{bob}},
			[]string{"боб у анны", "анна у себя"},
		},
	}

	for _, tt := range tests {
		if got := feed(tt.sources); !slices.Equal(got, tt.want) {
			t.Errorf("%s: feed = %v, want %v", tt.name, got, tt.want)
		}
	}

	tooMany := service.FeedSources{Everyone: true, Ignored: make([]uuid.UUID, service.MaxFeedIgnored+1)}
	if _, err := svc.Feed(ctx, cat, tooMany, uuid.Nil, 10); !errors.Is(err, service.ErrTooManyAuthors) {
		t.Fatalf("too many ignored walls: %v", err)
	}
}
