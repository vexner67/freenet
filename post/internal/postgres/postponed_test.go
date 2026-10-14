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

func TestPostponedRepository(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	repo := postgres.NewPostponedRepository(newPool(t))
	club, ann, bob := uuid.New(), uuid.New(), uuid.New()
	now := time.Now()

	postpone := func(d post.PostponedDraft) *post.Postponed {
		t.Helper()
		p, err := post.NewPostponed(d, now)
		if err != nil {
			t.Fatal(err)
		}
		if err = repo.Save(ctx, p); err != nil {
			t.Fatalf("Save: %v", err)
		}
		return p
	}

	later := postpone(post.PostponedDraft{
		WallID: ann, AuthorID: ann, Content: post.RestoreContent("", post.Photos{uuid.New()}, uuid.Nil), NSFW: true,
		Mentions: post.Mentions{bob}, PublishAt: now.Add(2 * time.Hour),
	})
	sooner := postpone(post.PostponedDraft{
		WallID: ann, AuthorID: ann, Content: post.RestoreContent("слова", nil, uuid.Nil), PublishAt: now.Add(time.Hour),
	})
	forClub := postpone(post.PostponedDraft{
		WallID: club, AuthorID: ann, Content: post.RestoreContent("для группы", nil, uuid.Nil), FromWall: true, Signed: true,
		PublishAt: now.Add(time.Hour),
	})

	for _, want := range []*post.Postponed{later, sooner, forClub} {
		if got, err := repo.Get(ctx, want.ID()); err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("Get = %+v, %v; want %+v", got, err, want)
		}
	}
	if _, err := repo.Get(ctx, uuid.New()); !errors.Is(err, post.ErrPostponedNotFound) {
		t.Fatalf("Get missing: %v", err)
	}

	list, err := repo.List(ctx, ann)
	if err != nil || len(list) != 2 || list[0].ID() != sooner.ID() || list[1].ID() != later.ID() {
		t.Fatalf("List = %v, %v", list, err)
	}

	due, err := repo.Due(ctx, now.Add(90*time.Minute), 1000)
	if err != nil || !slices.ContainsFunc(due, func(p *post.Postponed) bool { return p.ID() == sooner.ID() }) ||
		slices.ContainsFunc(due, func(p *post.Postponed) bool { return p.ID() == later.ID() }) {
		t.Fatalf("Due = %v, %v", due, err)
	}

	edited, err := sooner.Edited(post.PostponedEdit{Text: "другие слова", Mentions: post.Mentions{bob}, NSFW: true, PublishAt: now.Add(3 * time.Hour)}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.Update(ctx, edited); err != nil {
		t.Fatal(err)
	}
	if got, err := repo.Get(ctx, sooner.ID()); err != nil || !reflect.DeepEqual(got, edited) {
		t.Fatalf("after Update = %+v, %v; want %+v", got, err, edited)
	}

	if err = repo.Delete(ctx, later.ID()); err != nil {
		t.Fatal(err)
	}
	if err = repo.Delete(ctx, later.ID()); !errors.Is(err, post.ErrPostponedNotFound) {
		t.Fatalf("Delete twice: %v", err)
	}
	if err = repo.Update(ctx, later); !errors.Is(err, post.ErrPostponedNotFound) {
		t.Fatalf("Update deleted: %v", err)
	}

	if err = repo.DeleteByUser(ctx, ann); err != nil {
		t.Fatal(err)
	}
	for _, id := range []uuid.UUID{sooner.ID(), forClub.ID()} {
		if _, err = repo.Get(ctx, id); !errors.Is(err, post.ErrPostponedNotFound) {
			t.Fatalf("Get after DeleteByUser: %v", err)
		}
	}
}

func TestPostponedRepository_Limit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	repo := postgres.NewPostponedRepository(newPool(t))
	ann := uuid.New()
	now := time.Now()

	for i := range post.MaxPostponed + 1 {
		p, err := post.NewPostponed(post.PostponedDraft{
			WallID: ann, AuthorID: ann, Content: post.RestoreContent("ещё", nil, uuid.Nil), PublishAt: now.Add(time.Duration(i+1) * time.Minute),
		}, now)
		if err != nil {
			t.Fatal(err)
		}

		err = repo.Save(ctx, p)
		if i < post.MaxPostponed && err != nil {
			t.Fatalf("Save %d: %v", i, err)
		}
		if i == post.MaxPostponed && !errors.Is(err, post.ErrTooManyPostponed) {
			t.Fatalf("one too many: %v", err)
		}
	}

	if err := repo.DeleteByUser(ctx, ann); err != nil {
		t.Fatal(err)
	}
}
