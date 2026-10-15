package postgres_test

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/postgres"
)

func TestDocumentRepository(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool := newPool(t)
	documents := postgres.NewDocumentRepository(pool)
	ann, bob := uuid.New(), uuid.New()

	add := func(owner uuid.UUID, name post.DocumentName) *post.Document {
		t.Helper()

		d, err := post.NewDocument(uuid.Must(uuid.NewV7()), owner, name, "pdf", 4096, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if err = documents.Save(ctx, d); err != nil {
			t.Fatalf("Save: %v", err)
		}

		return d
	}

	first := add(ann, "Отчёт.pdf")
	second := add(ann, "План.pdf")
	theirs := add(bob, "Чужой.pdf")

	got, err := documents.Get(ctx, first.ID())
	if err != nil || !reflect.DeepEqual(got, first) {
		t.Fatalf("Get = %+v, %v; want %+v", got, err, first)
	}
	if _, err = documents.Get(ctx, uuid.New()); !errors.Is(err, post.ErrDocumentNotFound) {
		t.Fatalf("Get a missing one: %v", err)
	}

	list, err := documents.List(ctx, ann, uuid.Nil, 10)
	if err != nil || len(list) != 2 || list[0].ID() != second.ID() || list[1].ID() != first.ID() {
		t.Fatalf("List = %v, %v", list, err)
	}
	if list, err = documents.List(ctx, ann, second.ID(), 10); err != nil || len(list) != 1 || list[0].ID() != first.ID() {
		t.Fatalf("List after the newest = %v, %v", list, err)
	}
	if n, err := documents.Count(ctx, ann); err != nil || n != 2 {
		t.Fatalf("Count = %d, %v", n, err)
	}

	many, err := documents.GetMany(ctx, []uuid.UUID{first.ID(), theirs.ID(), uuid.New()})
	if err != nil || len(many) != 2 {
		t.Fatalf("GetMany = %v, %v", many, err)
	}

	renamed, _ := first.Renamed(ann, "Годовой отчёт")
	if err = documents.Update(ctx, renamed); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got, _ = documents.Get(ctx, first.ID()); got.Name() != "Годовой отчёт" {
		t.Fatalf("renamed to %q", got.Name())
	}

	// A post attaches documents and keeps them through an edit.
	posts := postgres.NewPostRepository(pool)
	content, _ := post.NewDocumented("", nil, post.Documents{first.ID(), second.ID()})
	p, err := post.New(ann, ann, content, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = posts.Save(ctx, p); err != nil {
		t.Fatalf("Save a post of documents: %v", err)
	}
	edited, err := p.Edited(ann, "Вот", nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = posts.Update(ctx, edited); err != nil {
		t.Fatal(err)
	}
	read, err := posts.Get(ctx, p.ID())
	if err != nil || !slices.Equal(read.Content().Documents(), post.Documents{first.ID(), second.ID()}) {
		t.Fatalf("the post attaches %v, %v", read.Content().Documents(), err)
	}

	if err = documents.Delete(ctx, first.ID()); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err = documents.Delete(ctx, first.ID()); !errors.Is(err, post.ErrDocumentNotFound) {
		t.Fatalf("Delete again: %v", err)
	}
	if err = documents.Update(ctx, renamed); !errors.Is(err, post.ErrDocumentNotFound) {
		t.Fatalf("Update a deleted one: %v", err)
	}

	if err = documents.DeleteByUser(ctx, ann); err != nil {
		t.Fatalf("DeleteByUser: %v", err)
	}
	if n, _ := documents.Count(ctx, ann); n != 0 {
		t.Fatalf("%d documents left", n)
	}
	if _, err = documents.Get(ctx, theirs.ID()); err != nil {
		t.Fatalf("another's document went too: %v", err)
	}
}
