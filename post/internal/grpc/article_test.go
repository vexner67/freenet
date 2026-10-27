package grpc_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestArticles(t *testing.T) {
	t.Parallel()

	s := newServer(t)
	ann, bob := uuid.New(), uuid.New()
	photo := uuid.NewString()

	blocks := []*postv1.ArticleBlock{
		{Kind: postv1.ArticleBlockKind_ARTICLE_BLOCK_KIND_HEADING, Spans: []*postv1.ArticleSpan{{Text: "Глава"}}},
		{Kind: postv1.ArticleBlockKind_ARTICLE_BLOCK_KIND_PARAGRAPH, Spans: []*postv1.ArticleSpan{{Text: "Это "}, {Text: "важно", Bold: true}}},
		{Kind: postv1.ArticleBlockKind_ARTICLE_BLOCK_KIND_PHOTO, PhotoId: photo, Caption: "Вид"},
	}

	created, err := s.client.CreateArticle(s.as(t, ann), &postv1.CreateArticleRequest{Title: "Моя статья", Blocks: blocks})
	if err != nil {
		t.Fatalf("CreateArticle: %v", err)
	}
	a := created.GetArticle()
	if a.GetTitle() != "Моя статья" || a.GetAuthorId() != ann.String() || len(a.GetBlocks()) != 3 || a.GetExcerpt() != "Это важно" || a.GetMinutes() != 1 {
		t.Fatalf("CreateArticle = %v", a)
	}

	for name, req := range map[string]*postv1.CreateArticleRequest{
		"no title":       {Blocks: blocks},
		"no kind":        {Title: "Т", Blocks: []*postv1.ArticleBlock{{Spans: []*postv1.ArticleSpan{{Text: "x"}}}}},
		"a bad photo":    {Title: "Т", Blocks: []*postv1.ArticleBlock{{Kind: postv1.ArticleBlockKind_ARTICLE_BLOCK_KIND_PHOTO, PhotoId: "x"}}},
		"a script link":  {Title: "Т", Blocks: []*postv1.ArticleBlock{{Kind: postv1.ArticleBlockKind_ARTICLE_BLOCK_KIND_PARAGRAPH, Spans: []*postv1.ArticleSpan{{Text: "x", Link: "javascript:alert(1)"}}}}},
		"a bad cover id": {Title: "Т", CoverId: "x", Blocks: blocks},
	} {
		if _, err = s.client.CreateArticle(s.as(t, ann), req); status.Code(err) != codes.InvalidArgument {
			t.Errorf("CreateArticle with %s: %v", name, err)
		}
	}
	if _, err = s.client.CreateArticle(context.Background(), &postv1.CreateArticleRequest{Title: "Т", Blocks: blocks}); status.Code(err) != codes.Unauthenticated {
		t.Errorf("CreateArticle without a token: %v", err)
	}

	// Anybody reads it, in full or brief.
	got, err := s.client.GetArticle(context.Background(), &postv1.GetArticleRequest{Id: a.GetId()})
	if err != nil || len(got.GetArticle().GetBlocks()) != 3 || got.GetArticle().GetBlocks()[2].GetPhotoId() != photo {
		t.Fatalf("GetArticle = %v, %v", got, err)
	}
	if got, err = s.client.GetArticle(context.Background(), &postv1.GetArticleRequest{Id: a.GetId(), Brief: true}); err != nil || len(got.GetArticle().GetBlocks()) != 0 || got.GetArticle().GetTitle() != "Моя статья" {
		t.Fatalf("GetArticle brief = %v, %v", got, err)
	}
	if _, err = s.client.GetArticle(context.Background(), &postv1.GetArticleRequest{Id: uuid.NewString()}); status.Code(err) != codes.NotFound {
		t.Errorf("GetArticle of none: %v", err)
	}

	list, err := s.client.ListArticles(context.Background(), &postv1.ListArticlesRequest{AuthorId: ann.String()})
	if err != nil || list.GetTotal() != 1 || len(list.GetArticles()) != 1 || len(list.GetArticles()[0].GetBlocks()) != 0 {
		t.Fatalf("ListArticles = %v, %v", list, err)
	}

	edit := &postv1.EditArticleRequest{Id: a.GetId(), Title: "Новая", Blocks: blocks[:2]}
	if _, err = s.client.EditArticle(s.as(t, bob), edit); status.Code(err) != codes.PermissionDenied {
		t.Errorf("EditArticle by another: %v", err)
	}
	edited, err := s.client.EditArticle(s.as(t, ann), edit)
	if err != nil || edited.GetArticle().GetTitle() != "Новая" || edited.GetArticle().GetEditedAt() == nil {
		t.Fatalf("EditArticle = %v, %v", edited, err)
	}

	if _, err = s.client.DeleteArticle(s.as(t, bob), &postv1.DeleteArticleRequest{Id: a.GetId()}); status.Code(err) != codes.PermissionDenied {
		t.Errorf("DeleteArticle by another: %v", err)
	}
	if _, err = s.client.DeleteArticle(s.as(t, ann), &postv1.DeleteArticleRequest{Id: a.GetId()}); err != nil {
		t.Fatalf("DeleteArticle: %v", err)
	}
	if _, err = s.client.GetArticle(context.Background(), &postv1.GetArticleRequest{Id: a.GetId()}); status.Code(err) != codes.NotFound {
		t.Errorf("GetArticle of a deleted one: %v", err)
	}
}
