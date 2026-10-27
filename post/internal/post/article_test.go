package post_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vexner67/freenet/post/internal/post"
)

func paragraph(spans ...post.Span) post.Block {
	return post.Block{Kind: post.BlockParagraph, Spans: spans}
}

func plain(text string) post.Span {
	return post.Span{Text: text}
}

func TestNewArticle(t *testing.T) {
	t.Parallel()

	photo, cover := uuid.New(), uuid.New()
	long := strings.Repeat("я", 100_001)

	for _, tt := range []struct {
		name  string
		draft post.ArticleDraft
		err   error
	}{
		{name: "text", draft: post.ArticleDraft{Title: "Заголовок", Blocks: []post.Block{paragraph(plain("Текст"))}}},
		{name: "all kinds", draft: post.ArticleDraft{Title: "Всё", CoverID: cover, Blocks: []post.Block{
			{Kind: post.BlockHeading, Spans: []post.Span{plain("Глава")}},
			{Kind: post.BlockSubheading, Spans: []post.Span{plain("Часть")}},
			paragraph(plain("Это "), post.Span{Text: "жирно", Bold: true}, post.Span{Text: "ссылка", Link: "https://example.com"}),
			{Kind: post.BlockQuote, Spans: []post.Span{plain("Цитата")}},
			{Kind: post.BlockBullet, Spans: []post.Span{plain("пункт")}},
			{Kind: post.BlockNumber, Spans: []post.Span{plain("первый")}},
			{Kind: post.BlockCode, Spans: []post.Span{plain("go run .\n")}},
			{Kind: post.BlockPhoto, PhotoID: photo, Caption: "Подпись"},
			{Kind: post.BlockDivider},
		}}},
		{name: "no title", draft: post.ArticleDraft{Title: "  ", Blocks: []post.Block{paragraph(plain("Текст"))}}, err: post.ErrArticleTitleEmpty},
		{name: "long title", draft: post.ArticleDraft{Title: strings.Repeat("я", 257), Blocks: []post.Block{paragraph(plain("Текст"))}}, err: post.ErrArticleTitleTooLong},
		{name: "no text", draft: post.ArticleDraft{Title: "Т", Blocks: []post.Block{paragraph(plain("  ")), {Kind: post.BlockDivider}}}, err: post.ErrArticleEmpty},
		{name: "photos only", draft: post.ArticleDraft{Title: "Т", Blocks: []post.Block{{Kind: post.BlockPhoto, PhotoID: photo}}}, err: post.ErrArticleEmpty},
		{name: "too long", draft: post.ArticleDraft{Title: "Т", Blocks: []post.Block{paragraph(plain(long))}}, err: post.ErrArticleTooLong},
		{name: "script link", draft: post.ArticleDraft{Title: "Т", Blocks: []post.Block{paragraph(post.Span{Text: "x", Link: "javascript:alert(1)"})}}, err: post.ErrArticleLink},
		{name: "marked code", draft: post.ArticleDraft{Title: "Т", Blocks: []post.Block{{Kind: post.BlockCode, Spans: []post.Span{{Text: "x", Bold: true}}}}}, err: post.ErrArticleBlock},
		{name: "unknown kind", draft: post.ArticleDraft{Title: "Т", Blocks: []post.Block{{Kind: "table", Spans: []post.Span{plain("x")}}}}, err: post.ErrArticleBlock},
		{name: "photo without id", draft: post.ArticleDraft{Title: "Т", Blocks: []post.Block{paragraph(plain("x")), {Kind: post.BlockPhoto}}}, err: post.ErrArticleBlock},
		{name: "text with photo", draft: post.ArticleDraft{Title: "Т", Blocks: []post.Block{{Kind: post.BlockParagraph, Spans: []post.Span{plain("x")}, PhotoID: photo}}}, err: post.ErrArticleBlock},
		{name: "divider with text", draft: post.ArticleDraft{Title: "Т", Blocks: []post.Block{paragraph(plain("x")), {Kind: post.BlockDivider, Spans: []post.Span{plain("x")}}}}, err: post.ErrArticleBlock},
		{name: "photo twice", draft: post.ArticleDraft{Title: "Т", CoverID: photo, Blocks: []post.Block{paragraph(plain("x")), {Kind: post.BlockPhoto, PhotoID: photo}}}, err: post.ErrArticlePhotos},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			a, err := post.NewArticle(uuid.New(), tt.draft, time.Now())
			if !errors.Is(err, tt.err) {
				t.Fatalf("NewArticle error = %v, want %v", err, tt.err)
			}

			if tt.err == nil && a.ID() == uuid.Nil {
				t.Fatal("NewArticle gave no ID")
			}
		})
	}
}

func TestArticleCleansBlocks(t *testing.T) {
	t.Parallel()

	a, err := post.NewArticle(uuid.New(), post.ArticleDraft{Title: "  Мой \n заголовок ", Blocks: []post.Block{
		paragraph(plain("")),
		paragraph(plain("Раз, "), plain("два\r\n"), post.Span{Text: "три", Bold: true}, post.Span{Text: ""}),
		{Kind: post.BlockPhoto, PhotoID: uuid.New(), Caption: " Под\nпись "},
	}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	if a.Title() != "Мой заголовок" {
		t.Fatalf("Title = %q", a.Title())
	}

	want := []post.Span{plain("Раз, два\n"), {Text: "три", Bold: true}}
	if blocks := a.Blocks(); len(blocks) != 2 || !reflect.DeepEqual(blocks[0].Spans, want) || blocks[1].Caption != "Под пись" {
		t.Fatalf("Blocks = %+v", blocks)
	}
}

func TestArticleEdited(t *testing.T) {
	t.Parallel()

	author := uuid.New()
	a, err := post.NewArticle(author, post.ArticleDraft{Title: "Было", Blocks: []post.Block{paragraph(plain("текст"))}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	draft := post.ArticleDraft{Title: "Стало", Blocks: []post.Block{paragraph(plain("новый"))}}
	if _, err = a.Edited(uuid.New(), draft, time.Now()); !errors.Is(err, post.ErrCannotChangeArticle) {
		t.Fatalf("Edited by another = %v", err)
	}

	edited, err := a.Edited(author, draft, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	if edited.ID() != a.ID() || edited.Title() != "Стало" || edited.EditedAt() == nil || a.Title() != "Было" {
		t.Fatalf("Edited = %+v", edited)
	}

	if _, err = a.Edited(author, post.ArticleDraft{Title: "", Blocks: draft.Blocks}, time.Now()); !errors.Is(err, post.ErrArticleTitleEmpty) {
		t.Fatalf("Edited without title = %v", err)
	}
}

func TestArticlePhotosExcerptMinutes(t *testing.T) {
	t.Parallel()

	cover, photo := uuid.New(), uuid.New()
	words := strings.TrimSpace(strings.Repeat("слово ", 450))
	a, err := post.NewArticle(uuid.New(), post.ArticleDraft{Title: "Т", CoverID: cover, Blocks: []post.Block{
		{Kind: post.BlockHeading, Spans: []post.Span{plain("Глава")}},
		paragraph(plain(words)),
		{Kind: post.BlockPhoto, PhotoID: photo},
	}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	if got := a.Photos(); !reflect.DeepEqual(got, []uuid.UUID{cover, photo}) {
		t.Fatalf("Photos = %v", got)
	}

	excerpt, cut := a.Excerpt(20)
	if excerpt != "слово слово слово" || !cut {
		t.Fatalf("Excerpt = %q, %v", excerpt, cut)
	}

	if a.Minutes() != 3 {
		t.Fatalf("Minutes = %d, want 3", a.Minutes())
	}
}
