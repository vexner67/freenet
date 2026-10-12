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
	svc, reads := newFeedService()
	dan := uuid.New()
	reads(cat, []uuid.UUID{ann, bob})
	reads(dan, []uuid.UUID{ann})

	write(t, svc, ann, "анна у себя")
	write(t, svc, bob, "боб у себя")
	if _, err := svc.Create(ctx, service.CreateInput{WallID: ann, AuthorID: bob, Content: textContent(t, "боб у анны")}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, service.CreateInput{WallID: bob, AuthorID: ann, Content: textContent(t, "анна у боба")}); err != nil {
		t.Fatal(err)
	}

	feed := func(reader uuid.UUID, sources service.FeedSources) []string {
		t.Helper()

		page, err := svc.Feed(ctx, reader, sources, uuid.Nil, 10)
		if err != nil {
			t.Fatalf("Feed(%+v): %v", sources, err)
		}

		return texts(page.Posts)
	}

	// cat follows ann and bob, dan follows ann alone.
	tests := []struct {
		name    string
		reader  uuid.UUID
		sources service.FeedSources
		want    []string
	}{
		{"everyone", cat, service.FeedSources{Everyone: true}, []string{"анна у боба", "боб у анны", "боб у себя", "анна у себя"}},
		{"everyone, own only", cat, service.FeedSources{Everyone: true, OwnOnly: true}, []string{"боб у себя", "анна у себя"}},
		{"everyone but bob's wall", cat, service.FeedSources{Everyone: true, Ignored: []uuid.UUID{bob}}, []string{"боб у анны", "анна у себя"}},
		{"the followed", cat, service.FeedSources{}, []string{"анна у боба", "боб у анны", "боб у себя", "анна у себя"}},
		{"the followed, own only", cat, service.FeedSources{OwnOnly: true}, []string{"боб у себя", "анна у себя"}},
		// Ann is followed to the walls of others too.
		{"ann alone", dan, service.FeedSources{}, []string{"анна у боба", "боб у анны", "анна у себя"}},
		// But not to a wall the reader ignores.
		{"ann alone but bob's wall", dan, service.FeedSources{Ignored: []uuid.UUID{bob}}, []string{"боб у анны", "анна у себя"}},
		// Bob ignored is read neither on his wall nor, as an author, on a
		// wall nobody reads; on ann's wall he stays.
		{"the followed but bob", cat, service.FeedSources{Ignored: []uuid.UUID{bob}}, []string{"боб у анны", "анна у себя"}},
		// Ann ignored: her wall goes, and so does what she wrote on bob's
		// wall — it stays only as a post on a wall the reader reads.
		{"the followed but ann", cat, service.FeedSources{Ignored: []uuid.UUID{ann}}, []string{"анна у боба", "боб у себя"}},
	}

	for _, tt := range tests {
		if got := feed(tt.reader, tt.sources); !slices.Equal(got, tt.want) {
			t.Errorf("%s: feed = %v, want %v", tt.name, got, tt.want)
		}
	}

	tooMany := service.FeedSources{Everyone: true, Ignored: make([]uuid.UUID, service.MaxFeedIgnored+1)}
	if _, err := svc.Feed(ctx, cat, tooMany, uuid.Nil, 10); !errors.Is(err, service.ErrTooManyAuthors) {
		t.Fatalf("too many ignored walls: %v", err)
	}
}
