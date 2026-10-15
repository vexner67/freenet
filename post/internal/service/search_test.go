package service_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
)

// openvk's "Поиск записей": a tag, or a part of the text, on every wall.
func TestPostService_Search(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ann, bob, cat := uuid.New(), uuid.New(), uuid.New()
	svc, _ := newFeedService()

	write(t, svc, ann, "Идём в #Поход")
	write(t, svc, bob, "#походы и горы")
	write(t, svc, bob, "поход без тега")

	search := func(query string, hidden ...uuid.UUID) ([]string, int) {
		t.Helper()

		page, err := svc.Search(ctx, cat, post.SearchQuery(query), hidden, uuid.Nil, 10)
		if err != nil {
			t.Fatalf("Search(%q): %v", query, err)
		}

		return texts(page.Posts), page.Total
	}

	tests := []struct {
		name   string
		query  string
		hidden []uuid.UUID
		want   []string
	}{
		{"tag", "#поход", nil, []string{"Идём в #Поход"}},
		{"text", "ПОХОД", nil, []string{"поход без тега", "#походы и горы", "Идём в #Поход"}},
		{"text without the blacklisted", "поход", []uuid.UUID{bob}, []string{"Идём в #Поход"}},
		{"nothing", "лес", nil, nil},
		{"empty", "", nil, nil},
	}

	for _, tt := range tests {
		got, total := search(tt.query, tt.hidden...)
		if !slices.Equal(got, tt.want) || total != len(tt.want) {
			t.Errorf("%s: found %v (%d), want %v", tt.name, got, total, tt.want)
		}
	}

	page, err := svc.Search(ctx, cat, "поход", nil, uuid.Nil, 2)
	if err != nil || len(page.Posts) != 2 || page.Total != 3 || page.NextCursor != page.Posts[1].ID() {
		t.Fatalf("the first page of two = %+v, %v", page, err)
	}

	tooMany := make([]uuid.UUID, service.MaxFeedHidden+1)
	if _, err = svc.Search(ctx, cat, "поход", tooMany, uuid.Nil, 10); !errors.Is(err, service.ErrTooManyAuthors) {
		t.Fatalf("too many blacklisted: %v", err)
	}
}
