package service_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/inmem"
	"github.com/vexner67/freenet/post/internal/post"
	"github.com/vexner67/freenet/post/internal/service"
)

func articleDraft(title string, cover uuid.UUID, photos ...uuid.UUID) post.ArticleDraft {
	d := post.ArticleDraft{Title: title, CoverID: cover, Blocks: []post.Block{{Kind: post.BlockParagraph, Spans: []post.Span{{Text: "текст"}}}}}
	for _, id := range photos {
		d.Blocks = append(d.Blocks, post.Block{Kind: post.BlockPhoto, PhotoID: id})
	}

	return d
}

func TestArticleService(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	photos := &fakePhotos{}
	store := inmem.NewStore()
	posts := service.NewPostService(service.PostDeps{
		Posts: store.Posts(), Pins: store.Posts(), Comments: store.Comments(), Likes: store.Likes(), Tx: fakeTx{},
		Notifier: &fakeNotifier{}, Photos: &fakePhotos{}, Moderators: fakeModerators{}, Switches: store.Posts(),
	})
	svc := service.NewArticleService(service.ArticleDeps{
		Articles: inmem.NewArticleRepository(), Tx: fakeTx{}, Photos: photos, Pages: store.Holds(), Posts: store.Posts(), Eraser: posts,
	})
	ann, bob := uuid.New(), uuid.New()
	cover, first, second := uuid.New(), uuid.New(), uuid.New()

	a, err := svc.Create(ctx, ann, articleDraft("Статья", cover, first))
	if err != nil {
		t.Fatal(err)
	}
	if want := []photoCall{{ann, []uuid.UUID{cover, first}}}; !reflect.DeepEqual(photos.published, want) {
		t.Fatalf("published = %v, want %v", photos.published, want)
	}

	if _, err = svc.Create(ctx, ann, articleDraft("", uuid.Nil)); !errors.Is(err, post.ErrArticleTitleEmpty) {
		t.Fatalf("Create without a title: %v", err)
	}

	if got, err := svc.Get(ctx, a.ID()); err != nil || got.Title() != "Статья" {
		t.Fatalf("Get = %v, %v", got, err)
	}

	if _, err = svc.Edit(ctx, bob, a.ID(), articleDraft("Чужая", uuid.Nil)); !errors.Is(err, post.ErrCannotChangeArticle) {
		t.Fatalf("Edit by another: %v", err)
	}

	edited, err := svc.Edit(ctx, ann, a.ID(), articleDraft("Новая", cover, second))
	if err != nil || edited.Title() != "Новая" {
		t.Fatalf("Edit = %v, %v", edited, err)
	}
	if got := photos.published[len(photos.published)-1]; !reflect.DeepEqual(got, photoCall{ann, []uuid.UUID{second}}) {
		t.Fatalf("published on edit = %v", got)
	}
	if want := []photoCall{{ann, []uuid.UUID{first}}}; !reflect.DeepEqual(photos.deleted, want) {
		t.Fatalf("deleted on edit = %v, want %v", photos.deleted, want)
	}

	other, err := svc.Create(ctx, ann, articleDraft("Вторая", uuid.Nil))
	if err != nil {
		t.Fatal(err)
	}

	page, err := svc.List(ctx, ann, uuid.Nil, 1)
	if err != nil || len(page.Articles) != 1 || page.Articles[0].ID() != other.ID() || page.Total != 2 || page.NextCursor != other.ID() {
		t.Fatalf("List = %+v, %v", page, err)
	}

	// A deleted page has no articles to read.
	if err = store.Holds().Hold(ctx, ann); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Get(ctx, a.ID()); !errors.Is(err, post.ErrArticleNotFound) {
		t.Fatalf("Get of a deleted page: %v", err)
	}
	if page, err = svc.List(ctx, ann, uuid.Nil, 10); err != nil || len(page.Articles) != 0 {
		t.Fatalf("List of a deleted page = %+v, %v", page, err)
	}
	if err = store.Holds().Release(ctx, ann); err != nil {
		t.Fatal(err)
	}

	if err = svc.Delete(ctx, bob, a.ID()); !errors.Is(err, post.ErrCannotChangeArticle) {
		t.Fatalf("Delete by another: %v", err)
	}
	// The post "Опубликовать на моей стене" wrote goes with the article;
	// one with words of its own stays.
	link := "https://freenet19.fun/article/" + a.ID().String()
	alone := wallPost(t, store, ann, link)
	worded := wallPost(t, store, ann, "Читайте: "+link)

	if err = svc.Delete(ctx, ann, a.ID()); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Posts().Get(ctx, alone); !errors.Is(err, post.ErrNotFound) {
		t.Fatalf("the post of the link alone: %v", err)
	}
	if _, err = store.Posts().Get(ctx, worded); err != nil {
		t.Fatalf("the post with words: %v", err)
	}
	if got := photos.deleted[len(photos.deleted)-1]; !reflect.DeepEqual(got, photoCall{ann, []uuid.UUID{cover, second}}) {
		t.Fatalf("deleted with the article = %v", got)
	}
	if _, err = svc.Get(ctx, a.ID()); !errors.Is(err, post.ErrArticleNotFound) {
		t.Fatalf("Get a deleted one: %v", err)
	}
	if photos.outsideTx != 0 {
		t.Fatalf("%d calls to media outside a transaction", photos.outsideTx)
	}
}

func wallPost(t *testing.T, store *inmem.Store, author uuid.UUID, text string) uuid.UUID {
	t.Helper()

	p, err := post.New(author, author, post.RestoreContent(post.Text(text), nil, uuid.Nil), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Posts().Save(context.Background(), p); err != nil {
		t.Fatal(err)
	}

	return p.ID()
}
