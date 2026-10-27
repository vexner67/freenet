package backend

import (
	"context"

	"github.com/vexner67/freenet/gateway/internal/model"
	"github.com/vexner67/freenet/pkg/errs"
	postv1 "github.com/vexner67/freenet/post/api/post/v1"
)

// Articles is rest.Articles over the post service.
type Articles struct {
	api postv1.PostServiceClient
}

func NewArticles(api postv1.PostServiceClient) *Articles {
	return &Articles{
		api: api,
	}
}

func (a *Articles) Create(ctx context.Context, accessToken string, d model.ArticleDraft) (model.Article, error) {
	resp, err := a.api.CreateArticle(withToken(ctx, accessToken), &postv1.CreateArticleRequest{
		Title: d.Title, CoverId: d.CoverID, Blocks: toProtoArticleBlocks(d.Blocks),
	})
	if err != nil {
		return model.Article{}, errs.Wrap("a.api.CreateArticle", fromStatus(err))
	}

	return toArticle(resp.GetArticle()), nil
}

func (a *Articles) Edit(ctx context.Context, accessToken, id string, d model.ArticleDraft) (model.Article, error) {
	resp, err := a.api.EditArticle(withToken(ctx, accessToken), &postv1.EditArticleRequest{
		Id: id, Title: d.Title, CoverId: d.CoverID, Blocks: toProtoArticleBlocks(d.Blocks),
	})
	if err != nil {
		return model.Article{}, errs.Wrap("a.api.EditArticle", fromStatus(err))
	}

	return toArticle(resp.GetArticle()), nil
}

func (a *Articles) Delete(ctx context.Context, accessToken, id string) error {
	if _, err := a.api.DeleteArticle(withToken(ctx, accessToken), &postv1.DeleteArticleRequest{Id: id}); err != nil {
		return errs.Wrap("a.api.DeleteArticle", fromStatus(err))
	}

	return nil
}

// Get reads an article, without its blocks when brief.
func (a *Articles) Get(ctx context.Context, id string, brief bool) (model.Article, error) {
	resp, err := a.api.GetArticle(ctx, &postv1.GetArticleRequest{Id: id, Brief: brief})
	if err != nil {
		return model.Article{}, errs.Wrap("a.api.GetArticle", fromStatus(err))
	}

	return toArticle(resp.GetArticle()), nil
}

// List pages the articles of authorID, newest first, brief.
func (a *Articles) List(ctx context.Context, authorID, cursor string, limit int) (model.ArticlePage, error) {
	resp, err := a.api.ListArticles(ctx, &postv1.ListArticlesRequest{AuthorId: authorID, Cursor: cursor, Limit: toLimit(limit)})
	if err != nil {
		return model.ArticlePage{}, errs.Wrap("a.api.ListArticles", fromStatus(err))
	}

	page := model.ArticlePage{NextCursor: resp.GetNextCursor(), Total: int(resp.GetTotal())}
	for _, pa := range resp.GetArticles() {
		page.Articles = append(page.Articles, toArticle(pa))
	}

	return page, nil
}

var articleBlockKinds = map[string]postv1.ArticleBlockKind{
	"paragraph":  postv1.ArticleBlockKind_ARTICLE_BLOCK_KIND_PARAGRAPH,
	"heading":    postv1.ArticleBlockKind_ARTICLE_BLOCK_KIND_HEADING,
	"subheading": postv1.ArticleBlockKind_ARTICLE_BLOCK_KIND_SUBHEADING,
	"quote":      postv1.ArticleBlockKind_ARTICLE_BLOCK_KIND_QUOTE,
	"bullet":     postv1.ArticleBlockKind_ARTICLE_BLOCK_KIND_BULLET,
	"number":     postv1.ArticleBlockKind_ARTICLE_BLOCK_KIND_NUMBER,
	"code":       postv1.ArticleBlockKind_ARTICLE_BLOCK_KIND_CODE,
	"photo":      postv1.ArticleBlockKind_ARTICLE_BLOCK_KIND_PHOTO,
	"divider":    postv1.ArticleBlockKind_ARTICLE_BLOCK_KIND_DIVIDER,
}

// toProtoArticleBlocks: an unknown kind goes as unspecified, which the
// post service refuses.
func toProtoArticleBlocks(blocks []model.ArticleBlock) []*postv1.ArticleBlock {
	out := make([]*postv1.ArticleBlock, 0, len(blocks))
	for _, b := range blocks {
		pb := &postv1.ArticleBlock{Kind: articleBlockKinds[b.Kind], PhotoId: b.PhotoID, Caption: b.Caption}
		for _, s := range b.Spans {
			pb.Spans = append(pb.Spans, &postv1.ArticleSpan{Text: s.Text, Bold: s.Bold, Italic: s.Italic, Strike: s.Strike, Link: s.Link})
		}
		out = append(out, pb)
	}

	return out
}

func toArticle(pa *postv1.Article) model.Article {
	a := model.Article{
		ID:        pa.GetId(),
		AuthorID:  pa.GetAuthorId(),
		Excerpt:   pa.GetExcerpt(),
		Minutes:   int(pa.GetMinutes()),
		CreatedAt: pa.GetCreatedAt().AsTime(),
	}
	a.Title, a.CoverID = pa.GetTitle(), pa.GetCoverId()

	if pa.GetEditedAt() != nil {
		at := pa.GetEditedAt().AsTime()
		a.EditedAt = &at
	}

	for _, pb := range pa.GetBlocks() {
		b := model.ArticleBlock{PhotoID: pb.GetPhotoId(), Caption: pb.GetCaption()}
		for kind, k := range articleBlockKinds {
			if k == pb.GetKind() {
				b.Kind = kind
			}
		}
		for _, s := range pb.GetSpans() {
			b.Spans = append(b.Spans, model.ArticleSpan{Text: s.GetText(), Bold: s.GetBold(), Italic: s.GetItalic(), Strike: s.GetStrike(), Link: s.GetLink()})
		}
		a.Blocks = append(a.Blocks, b)
	}

	return a
}
